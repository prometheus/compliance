// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sender_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"

	"github.com/prometheus/compliance/remotewrite/sender"
)

const (
	// TODO(bwplotka): Switch back to prometheusDownloadURL once a Prometheus release includes https://github.com/prometheus/prometheus/pull/20098.
	prometheusDownloadURL = ""
	prometheusGitRef      = "6f22682586f217ba0e1172f8111a61db47179798" // https://github.com/prometheus/prometheus/pull/20098
	scrapeConfigTemplate  = `
global:
  scrape_interval: 1s

remote_write:
  - url: "{{.RemoteWriteEndpointURL}}"
    protobuf_message: "{{.RemoteWriteMessage}}"
{{- if .RemoteWriteMode}}
    mode: "{{.RemoteWriteMode}}"
{{- end}}
    send_exemplars: true
    send_native_histograms: true
    queue_config:
      retry_on_http_429: true
    metadata_config:
      send: true

scrape_configs:
  - job_name: "{{.ScrapeTargetJobName}}"
    scrape_interval: 1s
    scrape_native_histograms: true
{{- if .RemoteWriteMode}}
    convert_classic_histograms_to_nhcb: true
{{- end}}
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ["{{.ScrapeTargetHostPort}}"]
`
)

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

type prometheus struct{}

func (p prometheus) Name() string { return "prometheus" }

// Run runs a Prometheus process for a test target options, until ctx is done.
//
// It auto-downloads or builds the Prometheus binary (see prometheusDownloadURL and prometheusGitRef).
// TODO(bwplotka): Process based runners are prone to leaking processes; add docker runner and/or figure out cleanup.
// Manually this could be done with 'killall -m "prometheus-3." -kill'.
func (p prometheus) Run(ctx context.Context, opts sender.Options) error {
	binary, err := getPrometheusBinary()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := scrapeConfigTmpl.Execute(&buf, opts); err != nil {
		return fmt.Errorf("failed to execute config template: %w", err)
	}

	dir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, buf.Bytes(), 0o600); err != nil {
		return err
	}

	return sender.RunCommand(ctx, ".", nil,
		binary,
		`--web.listen-address=0.0.0.0:0`,
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
		"--enable-feature=st-storage,exemplar-storage,metadata-wal-records",
	)
}

var _ sender.Sender = prometheus{}

var downloadMtx sync.Mutex

func getPrometheusBinary() (string, error) {
	if prometheusDownloadURL != "" {
		return downloadBinary(prometheusDownloadURL, "prometheus")
	}
	return buildPrometheusBinary(prometheusGitRef)
}

func buildPrometheusBinary(gitRef string) (string, error) {
	downloadMtx.Lock()
	defer downloadMtx.Unlock()

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	binDir := filepath.Join(cwd, "bin")
	shortRef := gitRef
	if len(shortRef) > 12 {
		shortRef = shortRef[:12]
	}
	filename := filepath.Join(binDir, fmt.Sprintf("prometheus-%s", shortRef))
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		return filename, nil
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}

	tmpBinDir, err := os.MkdirTemp(binDir, "build-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpBinDir)

	pkg := fmt.Sprintf("github.com/prometheus/prometheus/cmd/prometheus@%s", gitRef)
	fmt.Println("Building", pkg)
	cmd := exec.Command("go", "install", pkg)
	cmd.Env = append(os.Environ(), "GOBIN="+tmpBinDir, "GOTOOLCHAIN=auto")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to build %s: %w\n%s", pkg, err, string(out))
	}

	builtBin := filepath.Join(tmpBinDir, "prometheus")
	if runtime.GOOS == "windows" {
		builtBin += ".exe"
	}
	if err := os.Rename(builtBin, filename); err != nil {
		return "", err
	}
	if err := os.Chmod(filename, 0o744); err != nil {
		return "", err
	}
	return filename, nil
}

func downloadBinary(urlPattern string, filenameInArchivePattern string) (string, error) {
	downloadMtx.Lock()
	defer downloadMtx.Unlock()
	return downloadBinaryUnlocked(urlPattern, filenameInArchivePattern)
}

func downloadBinaryUnlocked(urlPattern string, filenameInArchivePattern string) (string, error) {
	urlToDownload, err := instantiateTemplate(urlPattern)
	if err != nil {
		return "", nil
	}

	filenameInArchive, err := instantiateTemplate(filenameInArchivePattern)
	if err != nil {
		return "", nil
	}

	parsed, err := url.Parse(urlToDownload)
	if err != nil {
		return "", nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	filename := path.Join(cwd, "bin", path.Base(parsed.Path))
	decompressTgz := strings.HasSuffix(filename, ".tar.gz")
	if decompressTgz {
		filename = strings.TrimSuffix(filename, ".tar.gz")
	}

	decompressZip := strings.HasSuffix(filename, ".zip")
	if decompressZip {
		filename = strings.TrimSuffix(filename, ".zip")
	}

	// If we've already downloaded it, then skip.
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		return filename, nil
	}

	tempfile, err := downloadURL(urlToDownload)
	if err != nil {
		return "", nil
	}

	if err := os.Mkdir(path.Dir(filename), 0o755); err != nil && !os.IsExist(err) {
		return "", nil
	}

	if decompressTgz {
		if err := extractTarGz(tempfile, filenameInArchive, filename); err != nil {
			return "", err
		}
	} else if decompressZip {
		if err := extractZip(tempfile, filenameInArchive, filename); err != nil {
			return "", err
		}
	} else {
		if err := os.Rename(tempfile, filename); err != nil {
			return "", err
		}
	}

	if err := os.Chmod(filename, 0o744); err != nil {
		return "", err
	}

	return filename, nil
}

func instantiateTemplate(pattern string) (string, error) {
	t := template.Must(template.New("url").Parse(pattern))
	var buf bytes.Buffer
	err := t.Execute(&buf, map[string]interface{}{
		"OS":   runtime.GOOS,
		"Arch": runtime.GOARCH,
	})
	return buf.String(), err
}

func downloadURL(url string) (filename string, err error) {
	fmt.Println("Downloading", url)

	tempfile, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	defer tempfile.Close()

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("error downloading: %d", resp.StatusCode)
	}

	if _, err := io.Copy(tempfile, resp.Body); err != nil {
		return "", err
	}

	return tempfile.Name(), nil
}

func extractZip(srcFile, filename, destFile string) error {
	fmt.Println("Decompressing", srcFile)

	r, err := zip.OpenReader(srcFile)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if path.Base(f.Name) != filename {
			continue
		}

		fmt.Println("Extracting", f.Name)

		src, err := f.Open()
		if err != nil {
			return err
		}
		defer src.Close()

		dest, err := os.Create(destFile)
		if err != nil {
			return err
		}
		defer dest.Close()

		_, err = io.Copy(dest, src)
		return err
	}

	return fmt.Errorf("did not find binary in .zip: %s", filename)
}

func extractTarGz(srcFile, filename, destFile string) error {
	fmt.Println("Decompressing", srcFile)

	f, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gzf, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzf.Close()

	tarReader := tar.NewReader(gzf)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if path.Base(header.Name) != filename {
			continue
		}

		fmt.Println("Extracting", header.Name)

		dest, err := os.Create(destFile)
		if err != nil {
			return err
		}
		defer dest.Close()

		if _, err := io.Copy(dest, tarReader); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("did not find binary in .tar.gz: %s", filename)
}
