package targets

import (
	"context"
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// RunProcess runs arbitrary process for compliance for a test target options, until ctx is canceled or error.
// This function depends on PROMETHEUS_RW2_COMPLIANCE_PROCESS_BINARY variable that should return path to a binary to execute.
// It fails if it's not set.
//
// The executed binary process is expected to:
// * Be blocking, until SIGINT.
// * Accept the test target options settings (what to scrape, where to send RW requests, etc) via the temporary file. The
// file path is passed as the first argument. The file is a YAML encoded remotewrite/sender/targets/common.go TargetOptions structure.
// * Scrape the provided endpoint via the provided message type, to a provided remote write endpoint.
func RunProcess(ctx context.Context, opts TargetOptions) error {
	binary := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_PROCESS_BINARY")
	if binary == "" {
		return errors.New("RunProcess: PROMETHEUS_RW2_COMPLIANCE_PROCESS_BINARY is not set; provide path to the binary to run")
	}
	out, err := yaml.Marshal(opts)
	if err != nil {
		return err
	}
	fName, err := writeTempFile(string(out), "target-options-*.yaml")
	if err != nil {
		return err
	}
	return runCommand(ctx, binary, fName)
}
