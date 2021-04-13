package targets

import (
	"fmt"
	"os"
)

const vectorDownloadURL = "https://github.com/timberio/vector/releases/download/v0.12.2/vector-0.12.2-x86_64-apple-darwin.tar.gz"

func RunVector(opts TargetOptions) error {
	binary, err := downloadBinary(vectorDownloadURL, "vector")
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
