#!/usr/bin/env bash
set -euo pipefail

GCM_CREDENTIALS_FILE="${GCM_CREDENTIALS_FILE:-$HOME/gmp-test-sa-key.json}"
GCM_URL="${GCM_URL:-https://staging-monitoring.sandbox.googleapis.com/v1/prometheus/api/v1/write}"

if [[ ! -f "$GCM_CREDENTIALS_FILE" ]]; then
  echo "Error: Credentials file not found at $GCM_CREDENTIALS_FILE" >&2
  exit 1
fi

CONFIG_FILE="$(mktemp /tmp/rw2_gcm_config.XXXXXX.yml)"
trap 'rm -f "$CONFIG_FILE"' EXIT

cat << EOF > "$CONFIG_FILE"
remote_write:
- name: "google_cloud"
  url: "$GCM_URL"
  protobuf_message: "io.prometheus.write.v2.Request"
  send_exemplars: true
  queue_config:
    retry_on_http_429: true
  google_iam:
    credentials_file: "$GCM_CREDENTIALS_FILE"
  write_relabel_configs:
  - regex: '(__type__|__unit__)'
    action: labeldrop
EOF

echo "Running receiver compliance test against GCM endpoint: $GCM_URL"
echo "Using credentials file: $GCM_CREDENTIALS_FILE"

cd "$(dirname "$0")/remotewrite"
export PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE="$CONFIG_FILE"
export PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="google_cloud"

go test -v ./receiver "$@"
