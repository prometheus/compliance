package targets

import (
	"context"
	"errors"
	"os"
)

// RunProcess runs arbitrary process for compliance for a test target options, until ctx is done.
// This function depends on PROMETHEUS_COMPLIANCE_RW_PROCESS_CMD variable that should return path to a binary to execute.
// It fails if it's not set.
//
// The executed binary process is expected to:
// * Be blocking, until SIGINT.
// * Accept the test target options settings (what to scrape, where to send RW requests, etc) via the environment variables (see the body of this function).
// * Scrape the provided endpoint via the provided message type, to a provided remote write endpoint.
// TODO(bwplotka): Process based runners are prone to leaking processes; add docker runner and/or figure out cleanup.
func RunProcess(ctx context.Context, opts TargetOptions) error {
	cmd := os.Getenv("PROMETHEUS_COMPLIANCE_RW_PROCESS_CMD")
	if cmd == "" {
		return errors.New("RunProcess: PROMETHEUS_COMPLIANCE_RW_PROCESS_CMD is not set; provide path to the binary to run")
	}

	extraEnvVars := []string{
		"PROMETHEUS_COMPLIANCE_RW_TARGET_SCRAPE_JOB_NAME", opts.ScrapeTargetJobName,
		"PROMETHEUS_COMPLIANCE_RW_TARGET_SCRAPE_HOST_PORT", opts.ScrapeTargetHostPort,
		"PROMETHEUS_COMPLIANCE_RW_TARGET_REMOTE_WRITE_ENDPOINT", opts.RemoteWriteEndpointURL,
		"PROMETHEUS_COMPLIANCE_RW_TARGET_REMOTE_WRITE_MESSAGE", string(opts.RemoteWriteMessage),
	}
	return runCommand(ctx, extraEnvVars, cmd)
}
