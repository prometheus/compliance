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

package promql_test

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
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"

	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance/promql"
)

const (
	prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v3.13.5/prometheus-3.13.5.{{.OS}}-{{.Arch}}.tar.gz"
	scrapeConfigTemplate  = `
global:
  scrape_interval: {{.ScrapeInterval}}

scrape_configs:
  - job_name: "{{.ScrapeTargetJobName}}"
    scrape_interval: {{.ScrapeInterval}}
    honor_labels: true
    honor_timestamps: true
    scrape_protocols:
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
      - targets: ["{{.ScrapeTargetHostPort}}"]
`
)

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

type prometheus struct {
	name string
}

func (p prometheus) Name() string {
	if p.name != "" {
		return p.name
	}
	return "prometheus"
}

// Run runs a Prometheus process for the given test target options until ctx is done.
//
// It auto-downloads the Prometheus binary from the official release URL unless
// PROMETHEUS_COMPLIANCE_PROMQL_BINARY is set.
func (p prometheus) Run(ctx context.Context, opts promql.Options) error {
	binary := os.Getenv("PROMETHEUS_COMPLIANCE_PROMQL_BINARY")
	if binary == "" {
		var err error
		binary, err = downloadBinary(prometheusDownloadURL, "prometheus")
		if err != nil {
			return err
		}
	}

	var buf bytes.Buffer
	if err := scrapeConfigTmpl.Execute(&buf, struct {
		ScrapeTargetJobName  string
		ScrapeTargetHostPort string
		ScrapeInterval       string
	}{
		ScrapeTargetJobName:  opts.ScrapeTargetJobName,
		ScrapeTargetHostPort: opts.ScrapeTargetHostPort,
		ScrapeInterval:       model.Duration(opts.ScrapeInterval).String(),
	}); err != nil {
		return fmt.Errorf("failed to execute config template: %w", err)
	}

	dir, err := os.MkdirTemp("", "promql-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, buf.Bytes(), 0o600); err != nil {
		return err
	}

	return promql.RunCommand(ctx, ".", nil,
		binary,
		fmt.Sprintf("--web.listen-address=%s", opts.QueryHostPort),
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
	)
}

var _ promql.Target = prometheus{}

var downloadMtx sync.Mutex

func downloadBinary(urlPattern, filenameInArchivePattern string) (string, error) {
	downloadMtx.Lock()
	defer downloadMtx.Unlock()
	return downloadBinaryUnlocked(urlPattern, filenameInArchivePattern)
}

func downloadBinaryUnlocked(urlPattern, filenameInArchivePattern string) (string, error) {
	urlToDownload, err := instantiateTemplate(urlPattern)
	if err != nil {
		return "", err
	}

	filenameInArchive, err := instantiateTemplate(filenameInArchivePattern)
	if err != nil {
		return "", err
	}

	parsed, err := url.Parse(urlToDownload)
	if err != nil {
		return "", err
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

	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		return filename, nil
	}

	tempfile, err := downloadURL(urlToDownload)
	if err != nil {
		return "", err
	}
	defer os.Remove(tempfile)

	if err := os.Mkdir(path.Dir(filename), 0o755); err != nil && !os.IsExist(err) {
		return "", err
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

	if resp.StatusCode != http.StatusOK {
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
