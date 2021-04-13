package targets

import (
	"fmt"
	"os"
	"runtime"
)

const vmagentURL = "https://github.com/VictoriaMetrics/VictoriaMetrics/releases/download/v1.58.0/vmutils-{{.Arch}}-v1.58.0.tar.gz"

func RunVMAgent(opts TargetOptions) error {
	// need mac builds https://github.com/VictoriaMetrics/VictoriaMetrics/issues/1042!
	if runtime.GOOS != "linux" {
		return fmt.Errorf("cannot run vmagent tests on non-linux: no builds are published")
	}

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
