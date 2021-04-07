package targets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"syscall"
	"text/template"
	"time"
)

type Target func(TargetOptions) error

type TargetOptions struct {
	ScrapeTarget    string
	ReceiveEndpoint string
}

func downloadBinary(pattern string, optFilename string) (string, error) {
	t := template.Must(template.New("url").Parse(pattern))
	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]interface{}{
		"OS":   runtime.GOOS,
		"Arch": runtime.GOARCH,
	}); err != nil {
		return "", err
	}

	parsed, err := url.Parse(buf.String())
	if err != nil {
		return "", nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	filename := path.Join(cwd, "bin", path.Base(parsed.Path))
	decompress := strings.HasSuffix(filename, ".tar.gz")
	if decompress {
		filename = strings.TrimSuffix(filename, ".tar.gz")
	}

	// If we've already downloaded it, then skip.
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		return filename, nil
	}

	tempfile, err := downloadURL(buf.String())
	if err != nil {
		return "", nil
	}

	if err := os.Mkdir(path.Dir(filename), 0o755); err != nil && !os.IsExist(err) {
		return "", nil
	}

	if decompress {
		if err := uncompress(tempfile, optFilename, filename); err != nil {
			return "", err
		}
	} else {
		os.Rename(tempfile, filename)
	}

	if err := os.Chmod(filename, 0o744); err != nil {
		return "", err
	}

	return filename, nil
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

func uncompress(srcFile, filename, destFile string) error {
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
		defer f.Close()

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

func runCommand(prog string, args ...string) error {
	cwd, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cwd)

	cmd := exec.Command(prog, args...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return err
	}

	go func() {
		time.Sleep(15 * time.Second)
		cmd.Process.Signal(syscall.SIGINT)
	}()

	return cmd.Wait()
}
