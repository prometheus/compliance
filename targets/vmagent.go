package targets

import (
	"fmt"
	"os"
	"path"
)

// need mac builds https://github.com/VictoriaMetrics/VictoriaMetrics/issues/1042!
func RunVMAgent(opts TargetOptions) error {
	cwd, _ := os.Getwd()
	binary := path.Join(cwd, "bin", "vmagent-darwin-amd64")

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
		fmt.Sprintf("-promscrape.config=%s", configFileName),
		fmt.Sprintf("-remoteWrite.url=%s", opts.ReceiveEndpoint))
}
