package targets

import (
	"fmt"
	"os"
	"runtime"
)

func getVectorDownloadURL() string {
	var version string = "0.16.1"
	switch runtime.GOOS {
	case "darwin":
		return "https://github.com/timberio/vector/releases/download/v" + version + "/vector-" + version + "-x86_64-apple-darwin.tar.gz"
	case "linux":
		return "https://github.com/timberio/vector/releases/download/v" + version + "/vector-" + version + "-x86_64-unknown-linux-gnu.tar.gz"
	case "windows":
		return "https://github.com/timberio/vector/releases/download/v" + version + "/vector-" + version + "-x86_64-pc-windows-msvc.zip"
	default:
		panic("unsupported OS")
	}
}

func RunVector(opts TargetOptions) error {
	binary, err := downloadBinary(getVectorDownloadURL(), "vector")
	if err != nil {
		return err
	}

	cfg := fmt.Sprintf(`
[sources.prometheus_scrape]
  type = "prometheus_scrape"
  endpoints = ["http://%s/metrics"]
  scrape_interval_secs = 1

[sinks.prometheus_remote_write]
  type = "prometheus_remote_write"
  inputs = ["prometheus_scrape"]
  endpoint = "%s"
`, opts.ScrapeTarget, opts.ReceiveEndpoint)
	configFileName, err := writeTempFile(cfg, "config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(binary, opts.Timeout, fmt.Sprintf("--config-toml=%s", configFileName))
}
