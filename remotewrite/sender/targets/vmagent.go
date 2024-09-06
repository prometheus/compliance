package targets

import (
	"fmt"
	"os"
)

const vmagentURL = "https://github.com/VictoriaMetrics/VictoriaMetrics/releases/download/v1.67.0/vmutils-{{.Arch}}-v1.67.0.tar.gz"

func RunVMAgent(opts TargetOptions) error {
	// NB this won't work on a Mac - need mac builds https://github.com/VictoriaMetrics/VictoriaMetrics/issues/1042!
	// If you build it yourself and stick it in the bin/ directory, the tests will work.
	binary, err := downloadBinary(vmagentURL, "vmagent-prod")
	if err != nil {
		return err
	}

	cfg := fmt.Sprintf(`
global:
  scrape_interval: 1s

scrape_configs:
  - job_name: 'test'
    static_configs:
    - targets: ['%s']
`, opts.ScrapeTarget)
	configFileName, err := writeTempFile(cfg, "config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(binary, opts.Timeout,
		`-httpListenAddr=:0`, `-influxListenAddr=:0`,
		fmt.Sprintf("-promscrape.config=%s", configFileName),
		fmt.Sprintf("-remoteWrite.url=%s", opts.ReceiveEndpoint))
}
