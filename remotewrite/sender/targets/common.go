package targets

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
)

type Target func(TargetOptions) error

type TargetOptions struct {
	ScrapeTarget    string
	ReceiveEndpoint string
	Timeout         time.Duration
}

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

func writeTempFile(contents, name string) (filename string, err error) {
	f, err := os.CreateTemp("", name)
	if err != nil {
		return "", err
	}

	_, err = f.Write([]byte(contents))
	if err != nil {
		return "", err
	}

	return f.Name(), f.Close()
}

// runCommand runs the given command with the given args in a temporary working
// directory and connecting that processes stdin/stdout to stdin/stdout.
// After timeout seconds, it send SIGINT to the process to shut it down and
// returns an error if the process exits with non-zero status code.
func runCommand(prog string, timeout time.Duration, args ...string) error {
	cwd, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cwd)

	var output *os.File
	if true {
		output, err = os.CreateTemp("", "")
		if err != nil {
			return err
		}
		defer output.Close()
		defer os.Remove(output.Name())
	} else {
		output = os.Stdout
	}

	cmd := exec.Command(prog, args...)
	cmd.Dir = cwd
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Start()
	if err != nil {
		return err
	}

	go func() {
		time.Sleep(timeout)
		if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
			log.Fatalf("failed to send signal: %v", err)
		}
	}()

	return cmd.Wait()
}
