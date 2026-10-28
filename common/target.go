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

package common

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
	"syscall"
	"text/template"
)

// RunCommand runs the given command with the given args until context is done.
//
// This is useful when starting process-based compliance targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	var output io.Writer = os.Stdout
	if os.Getenv("DEBUG") == "" {
		output = io.Discard
	}

	cmd := exec.Command(prog, args...)
	// Required for group process signalling on close.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, extraEnvVars...)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return err
	}

	cmdStopped := make(chan error, 1)
	go func() {
		cmdStopped <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Use group process ID to pass signal to all child processes (e.g. under go run).
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			return fmt.Errorf("failed to get pgid: %w", err)
		}
		if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to send signal: %w", err)
		}
		return <-cmdStopped
	case err := <-cmdStopped:
		return err
	}
}

var downloadMtx sync.Mutex

const (
	// TODO(bwplotka): Switch back to DefaultPrometheusDownloadURL once a Prometheus release includes https://github.com/prometheus/prometheus/pull/20098.
	DefaultPrometheusDownloadURL = ""
	DefaultPrometheusGitRef      = "6f22682586f217ba0e1172f8111a61db47179798" // https://github.com/prometheus/prometheus/pull/20098
)

// GetPrometheusBinary downloads or builds the default Prometheus binary (see
// DefaultPrometheusDownloadURL and DefaultPrometheusGitRef) into ./bin/.
func GetPrometheusBinary() (string, error) {
	if DefaultPrometheusDownloadURL != "" {
		return DownloadBinary(DefaultPrometheusDownloadURL, "prometheus")
	}
	return BuildPrometheusBinary(DefaultPrometheusGitRef)
}

// BuildPrometheusBinary builds github.com/prometheus/prometheus/cmd/prometheus at gitRef
// into ./bin/prometheus-<shortRef>, returning the path to the executable binary.
// If the binary already exists in ./bin/, building is skipped.
func BuildPrometheusBinary(gitRef string) (string, error) {
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

// DownloadBinary downloads and extracts a binary archive from urlPattern (templated with {{.OS}} and {{.Arch}})
// into ./bin/, returning the path to the executable binary. If the binary already exists in ./bin/, download is skipped.
func DownloadBinary(urlPattern, filenameInArchivePattern string) (string, error) {
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
