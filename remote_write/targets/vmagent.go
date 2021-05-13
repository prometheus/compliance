package targets

import (
	"fmt"
	"os"

        "github.com/prometheus/compliance/remote_write/latest"
)

func RunVMAgent(opts TargetOptions) error {
	// NB this won't work on a Mac - need mac builds https://github.com/VictoriaMetrics/VictoriaMetrics/issues/1042!
	// If you build it yourself and stick it in the bin/ directory, the tests will work.
	binary, err := downloadBinary(latest.GetDownloadURL("https://github.com/VictoriaMetrics/VictoriaMetrics/releases/download/vVERSION/vmutils-{{.Arch}}-vVERSION.tar.gz"), "vmagent-prod")
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
