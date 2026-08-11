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

package receiver_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/prometheus/compliance/remotewrite/receiver/next"
)

const prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v3.11.0-rc.0/prometheus-3.11.0-rc.0.{{.OS}}-{{.Arch}}.tar.gz"

type prometheus struct{}

func (prometheus) Name() string { return "prometheus" }

// Run downloads (and caches) a Prometheus release binary and runs it as the
// receiver under test, with remote write receiving enabled, until ctx is done.
func (prometheus) Run(ctx context.Context, ready func(remoteWriteURL string)) error {
	binary, err := downloadBinary(prometheusDownloadURL, "prometheus")
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "receiver-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "prometheus.yml")
	if err := os.WriteFile(configFile, []byte("global:\n  scrape_interval: 15s\n"), 0o600); err != nil {
		return err
	}

	port, err := freePort()
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	done := make(chan error, 1)
	go func() {
		done <- receiver.RunCommand(ctx, dir, nil, binary,
			fmt.Sprintf("--web.listen-address=%s", addr),
			fmt.Sprintf("--storage.tsdb.path=%s", dir),
			fmt.Sprintf("--config.file=%s", configFile),
			"--web.enable-remote-write-receiver",
		)
	}()

	readyURL := fmt.Sprintf("http://%s/-/ready", addr)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("prometheus exited before becoming ready: %w", err)
			}
			return nil
		default:
		}

		resp, err := http.Get(readyURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready(fmt.Sprintf("http://%s/api/v1/write", addr))
				return <-done
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("prometheus did not become ready in time")
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

var _ receiver.Receiver = prometheus{}

// TestReceiverCompliance runs the receiver compliance test suite against a
// downloaded Prometheus release binary.
func TestReceiverCompliance(t *testing.T) {
	receiver.RunTests(t, prometheus{}, receiver.ComplianceTests())
}

// The functions below are copied as-is from ../../sender/prometheus_test.go to
// avoid introducing a shared internal package for this draft; if the design is
// accepted, this download helper should be deduplicated between sender and
// receiver (e.g. into an internal/targets helper package).

var downloadMtx sync.Mutex

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

	parsedURL, err := url.Parse(urlToDownload)
	if err != nil {
		return "", nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	filename := path.Join(cwd, "bin", path.Base(parsedURL.Path))
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
	var buf strings.Builder
	err := t.Execute(&buf, map[string]interface{}{
		"OS":   runtime.GOOS,
		"Arch": runtime.GOARCH,
	})
	return buf.String(), err
}

func downloadURL(rawURL string) (filename string, err error) {
	fmt.Println("Downloading", rawURL)

	tempfile, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	defer tempfile.Close()

	resp, err := http.Get(rawURL)
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
