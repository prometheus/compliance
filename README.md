# Prometheus Compliance Tests

This repo contains code to test compliance with various Prometheus standards. Anyone taking part in CNCF's [Prometheus Conformance](https://github.com/cncf/prometheus-conformance) Program will need to run the tests in here against their own implementations.

If you are reading this as someone testing their own implementation or considering to do so: There is a _LOT_ of work that's planned but not executed yet. If you have time or headcount to invest in uplifting everyone's compliance, [please talk to us](https://prometheus.io/community/).

There are several [software categories](https://docs.google.com/document/d/1VGMme9RgpclqF4CF2woNmgFqq0J7nqHn-l72uNmAxhA/) something can be tested in. If something does not seem to fit existing categories, please also talk to us.

## Alert Generator

The [alert_generator](alert_generator/README.md) directory contains a shim at the moment. It will test correct generation and emitting of alerts towards Alertmanager.

## OpenMetrics

The [openmetrics](openmetrics/README.md) directory contains a reference to the [OpenMetrics](https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md) test suite.

## PromQL

The [promql](promql/README.md) directory contains code to test compliance with the [native Prometheus PromQL implementation](https://github.com/prometheus/prometheus/tree/main/promql).

## PromQL E2E

The [promqle2e](promqle2e/README.md) directory contains a Go Module for performing
compliance tests for PromQL correctness. It's designed for smaller in-place quick unit tests, e.g. on per-PR basis, using docker based test framework. Useful as an acceptance tests
for vendors or those who wish to maintain high Prometheus compatibility over time.

## Remote Write: Sender

The [remotewrite/sender](remotewrite/sender/proposals.md) directory contains code to test compliance with the [Prometheus Remote Write specification](https://prometheus.io/docs/specs/remote_write_spec/) as a sender.


📐 COMPREHENSIVE PROPOSAL: Remote Write 2.0 Sender Compliance Test Suite

Executive Summary

This proposal outlines a complete sender compliance test suite mirroring the modern architecture of the existing receiver tests (remotewrite/receiver/), but flipped to validate sender implementations. The suite will test all MUST/SHOULD/MAY requirements
from the Remote Write 2.0 specification.

  ---
Architecture Overview

Core Architectural Inversion

| Aspect       | Receiver Tests (Current)            | Sender Tests (New)                      |
  |--------------|-------------------------------------|-----------------------------------------|
| Role         | HTTP Client                         | HTTP Server (Mock Receiver)             |
| Request Flow | Generate → Send → Validate Response | Capture → Parse → Validate Request      |
| Instance     | External receivers                  | Fork/launch sender processes            |
| Focus        | Response correctness                | Request correctness                     |
| Validation   | Status codes, headers               | Protobuf structure, protocol compliance |

Directory Structure

/remotewrite/sender/
├── main_test.go              # Test framework, sender launcher, config management
├── helpers_test.go           # Mock server, request capture, validation utilities
│
├── protocol_test.go          # HTTP protocol compliance (headers, encoding)
├── symbols_test.go           # Symbol table structure and optimization
├── samples_test.go           # Float sample encoding and ordering
├── histograms_test.go        # Native histogram encoding
├── exemplars_test.go         # Exemplar attachment and encoding
├── metadata_test.go          # Metadata structure and types
├── labels_test.go            # Label validation and special cases
├── timestamps_test.go        # Timestamp ordering and created_timestamp
├── combined_test.go          # Multi-feature integration tests
│
├── retry_test.go             # Retry behavior on errors
├── backoff_test.go           # Exponential backoff validation
├── batching_test.go          # Request batching behavior
├── error_handling_test.go    # 4xx/5xx response handling
│
├── rw1_compat_test.go        # Backward compatibility with RW 1.x
├── fallback_test.go          # Content-type fallback on 415
│
├── config_example.yml        # Sender configurations
├── testdata/                 # Test fixtures, scrape data
│   ├── scrape_samples.txt
│   ├── scrape_histograms.txt
│   └── ...
└── go.mod

  ---
Phase 1: Foundation & Protocol Compliance

Goal: Establish test framework, mock server infrastructure, and validate basic HTTP protocol requirements.

1.1 Core Test Framework (main_test.go)

Components:
// Sender configuration structure
type SenderConfig struct {
Name           string            // e.g., "prometheus", "grafana-agent"
BinaryPath     string            // Path to sender binary
ConfigTemplate string            // Go template with placeholders
StartArgs      []string          // CLI arguments
ReadyCheck     ReadyCheckFunc    // Function to check if sender started
StopSignal     os.Signal         // SIGTERM, SIGINT, etc.
GracePeriod    time.Duration     // Time to wait for graceful shutdown
}

// Global state
var senderConfigs []SenderConfig

// Core functions
func TestMain(m *testing.M) {
// Load sender configs from PROMETHEUS_RW2_COMPLIANCE_SENDER_CONFIG_FILE
// Filter by PROMETHEUS_RW2_COMPLIANCE_SENDERS env var
// Setup temporary directories
// Run tests
// Cleanup
}

func loadSenderConfigs(path string) ([]SenderConfig, error)
func filterSenders(configs []SenderConfig, filter string) []SenderConfig
func forEachSender(t *testing.T, fn func(*testing.T, SenderConfig))

Config Example:
# config_example.yml
senders:
- name: prometheus
binary: ./bin/prometheus
config_template: |
global:
scrape_interval: 1s
scrape_configs:
- job_name: 'test'
static_configs:
- targets: ['{{.ScrapeTarget}}']
remote_write:
- url: {{.RemoteWriteURL}}
queue_config:
max_shards: 1
capacity: 10
ready_check: http://localhost:9090/-/ready

    - name: grafana-agent
      binary: ./bin/agent
      config_template: |
        prometheus:
          configs:
            - name: test
              scrape_configs:
                - job_name: test
                  static_configs:
                    - targets: ['{{.ScrapeTarget}}']
              remote_write:
                - url: {{.RemoteWriteURL}}

1.2 Mock Receiver & Request Capture (helpers_test.go)

Mock Server Framework:
type MockReceiver struct {
server        *httptest.Server
requests      chan *CapturedRequest
mu            sync.Mutex
responseCode  int
responseDelay time.Duration
responseBody  []byte
headers       http.Header  // Custom response headers

      // Statistics
      totalRequests int
      totalSamples  int
      totalHistograms int
}

type CapturedRequest struct {
Timestamp    time.Time
Headers      http.Header
Body         []byte
ContentType  string
Encoding     string

      // Parsed data
      Proto        *writev2.Request  // Parsed protobuf
      ParseError   error
}

func newMockReceiver(opts MockReceiverOpts) *MockReceiver
func (m *MockReceiver) Start(t *testing.T) string  // Returns URL
func (m *MockReceiver) Stop()
func (m *MockReceiver) WaitForRequest(timeout time.Duration) (*CapturedRequest, error)
func (m *MockReceiver) GetAllRequests() []*CapturedRequest
func (m *MockReceiver) SetResponse(code int, body []byte, headers http.Header)
func (m *MockReceiver) SetDelay(delay time.Duration)
func (m *MockReceiver) GetStats() ReceiverStats

Sender Process Management:
type SenderInstance struct {
config      SenderConfig
cmd         *exec.Cmd
configFile  string
scrapeURL   string
remoteURL   string
logs        *bytes.Buffer
started     time.Time
}

func launchSender(t *testing.T, cfg SenderConfig, remoteURL, scrapeURL string) *SenderInstance
func (s *SenderInstance) Stop()
func (s *SenderInstance) Kill()
func (s *SenderInstance) Logs() string
func (s *SenderInstance) WaitForReady(timeout time.Duration) error
func (s *SenderInstance) IsRunning() bool

Scrape Target Mock:
type MockScrapeTarget struct {
server    *httptest.Server
responses []string  // Metric responses to cycle through
index     int
}

func newMockScrapeTarget(metrics ...string) *MockScrapeTarget
func (m *MockScrapeTarget) Start() string  // Returns URL
func (m *MockScrapeTarget) Stop()
func (m *MockScrapeTarget) SetMetrics(metrics string)

Test Orchestration:
// Complete test scenario runner
func runSenderTest(t *testing.T, scenario SenderTestScenario) {
// 1. Start mock receiver
// 2. Start mock scrape target
// 3. Launch sender with config
// 4. Wait for sender to scrape and send
// 5. Capture requests
// 6. Stop sender
// 7. Validate requests
}

type SenderTestScenario struct {
ScrapeData       string
ReceiverResponse ReceiverResponseOpts
Timeout          time.Duration
Validator        RequestValidator
}

type RequestValidator func(*testing.T, *CapturedRequest)

1.3 Protocol Compliance Tests (protocol_test.go)

Test Cases:

func TestProtocolCompliance(t *testing.T) {
testCases := []struct {
name        string
description string
rfcLevel    string  // "MUST", "SHOULD", "MAY"
validator   RequestValidator
}{
{
name: "content_type_protobuf",
description: "Sender MUST use Content-Type: application/x-protobuf",
rfcLevel: "MUST",
validator: func(t *testing.T, req *CapturedRequest) {
ct := req.Headers.Get("Content-Type")
must(t).True(
strings.HasPrefix(ct, "application/x-protobuf"),
"Content-Type must be application/x-protobuf, got: %s", ct,
)
},
},
{
name: "content_type_with_proto_param",
description: "Sender SHOULD use Content-Type with proto parameter for RW 2.0",
rfcLevel: "SHOULD",
validator: func(t *testing.T, req *CapturedRequest) {
ct := req.Headers.Get("Content-Type")
should(t).Contains(
ct, "proto=io.prometheus.write.v2.Request",
"Content-Type should include proto parameter",
)
},
},
{
name: "content_encoding_snappy",
description: "Sender MUST use Content-Encoding: snappy",
rfcLevel: "MUST",
validator: func(t *testing.T, req *CapturedRequest) {
must(t).Equal(
"snappy", req.Headers.Get("Content-Encoding"),
"Content-Encoding must be snappy",
)
},
},
{
name: "version_header_present",
description: "Sender MUST include X-Prometheus-Remote-Write-Version header",
rfcLevel: "MUST",
validator: func(t *testing.T, req *CapturedRequest) {
version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
must(t).NotEmpty(version, "Version header must be present")
},
},
{
name: "version_header_value",
description: "Sender SHOULD use version 2.0.0 for RW 2.0 receivers",
rfcLevel: "SHOULD",
validator: func(t *testing.T, req *CapturedRequest) {
version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
should(t).True(
strings.HasPrefix(version, "2.0"),
"Version should be 2.0.x, got: %s", version,
)
},
},
{
name: "user_agent_present",
description: "Sender MUST include User-Agent header (RFC 9110)",
rfcLevel: "MUST",
validator: func(t *testing.T, req *CapturedRequest) {
ua := req.Headers.Get("User-Agent")
must(t).NotEmpty(ua, "User-Agent header must be present")
},
},
{
name: "snappy_block_format",
description: "Sender MUST use Snappy block format (not framed)",
rfcLevel: "MUST",
validator: func(t *testing.T, req *CapturedRequest) {
// Snappy framed format starts with: sNaPpY
must(t).NotEqual(
[]byte("sNaPpY"), req.Body[:6],
"Must use block format, not framed format",
)
// Validate decompression works
_, err := snappy.Decode(nil, req.Body)
must(t).NoError(err, "Snappy decompression must succeed")
},
},
{
name: "protobuf_parseable",
description: "Sender MUST send valid protobuf message",
rfcLevel: "MUST",
validator: func(t *testing.T, req *CapturedRequest) {
must(t).NoError(req.ParseError, "Protobuf must parse successfully")
must(t).NotNil(req.Proto, "Parsed proto must not be nil")
},
},
}

      for _, tc := range testCases {
          t.Run(tc.name, func(t *testing.T) {
              t.Attr("rfcLevel", tc.rfcLevel)
              t.Attr("description", tc.description)

              forEachSender(t, func(t *testing.T, sender SenderConfig) {
                  runSenderTest(t, SenderTestScenario{
                      ScrapeData: simpleCounter(),
                      Validator: tc.validator,
                  })
              })
          })
      }
}

1.4 Symbol Table Tests (symbols_test.go)

Spec Requirements:
- MUST populate symbols array with deduplicated strings
- MUST start with empty string at index 0
- MUST use references to symbols in labels_refs

Test Cases:
func TestSymbolTable(t *testing.T) {
tests := []struct {
name        string
description string
scrapeData  string
validator   func(*testing.T, *writev2.Request)
}{
{
name: "empty_string_at_index_zero",
description: "Symbol table MUST start with empty string at index 0",
scrapeData: simpleCounter(),
validator: func(t *testing.T, req *writev2.Request) {
must(t).True(len(req.Symbols) > 0, "Symbols must not be empty")
must(t).Equal("", req.Symbols[0], "First symbol must be empty string")
},
},
{
name: "string_deduplication",
description: "Symbol table MUST deduplicate repeated strings",
scrapeData: `
# Repeated label names and values across metrics
test_metric{foo="bar",baz="bar"} 1
test_metric{foo="qux",baz="qux"} 2
another_metric{foo="bar"} 3
`,
validator: func(t *testing.T, req *writev2.Request) {
// "bar" appears 3 times in raw data, should be once in symbols
barCount := 0
for _, sym := range req.Symbols {
if sym == "bar" {
barCount++
}
}
must(t).Equal(1, barCount, "String 'bar' must appear exactly once")

                  // Verify __name__ appears once
                  nameCount := 0
                  for _, sym := range req.Symbols {
                      if sym == "__name__" {
                          nameCount++
                      }
                  }
                  must(t).Equal(1, nameCount, "__name__ must appear exactly once")
              },
          },
          {
              name: "labels_refs_valid_indices",
              description: "labels_refs MUST reference valid symbol indices",
              scrapeData: simpleCounter(),
              validator: func(t *testing.T, req *writev2.Request) {
                  symbolCount := uint32(len(req.Symbols))
                  for _, ts := range req.Timeseries {
                      for _, ref := range ts.LabelsRefs {
                          must(t).True(
                              ref < symbolCount,
                              "Label ref %d exceeds symbol table size %d",
                              ref, symbolCount,
                          )
                      }
                  }
              },
          },
          {
              name: "labels_refs_even_length",
              description: "labels_refs length MUST be multiple of two (key-value pairs)",
              scrapeData: simpleCounter(),
              validator: func(t *testing.T, req *writev2.Request) {
                  for i, ts := range req.Timeseries {
                      must(t).Equal(
                          0, len(ts.LabelsRefs)%2,
                          "Timeseries[%d] labels_refs length must be even, got %d",
                          i, len(ts.LabelsRefs),
                      )
                  }
              },
          },
      }

      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              runSenderTestWithValidator(t, tt.scrapeData, tt.validator)
          })
      }
}

1.5 Validation Helpers (helpers_test.go continued)

// RFC compliance markers (reused from receiver tests)
func must(t *testing.T) *assertions { /* ... */ }
func should(t *testing.T) *assertions { /* ... */ }
func may(t *testing.T) *assertions { /* ... */ }

// Request validation utilities
func validateProtobufStructure(t *testing.T, req *writev2.Request) {
// Symbol table checks
require.NotEmpty(t, req.Symbols)
require.Equal(t, "", req.Symbols[0])

      // Timeseries checks
      for _, ts := range req.Timeseries {
          require.True(t, len(ts.LabelsRefs)%2 == 0)
          require.True(t, len(ts.Samples) > 0 || len(ts.Histograms) > 0)
          require.False(t, len(ts.Samples) > 0 && len(ts.Histograms) > 0)
      }
}

func extractLabels(req *writev2.Request, ts *writev2.TimeSeries) map[string]string {
labels := make(map[string]string)
for i := 0; i < len(ts.LabelsRefs); i += 2 {
key := req.Symbols[ts.LabelsRefs[i]]
val := req.Symbols[ts.LabelsRefs[i+1]]
labels[key] = val
}
return labels
}

func findTimeseriesByName(req *writev2.Request, name string) *writev2.TimeSeries {
for _, ts := range req.Timeseries {
labels := extractLabels(req, ts)
if labels["__name__"] == name {
return ts
}
}
return nil
}

// Test data generators (similar to receiver tests)
func simpleCounter() string {
return "test_counter_total 42\n"
}

func simpleGauge() string {
return "test_gauge 3.14\n"
}

func histogramMetric() string {
return `# HELP test_histogram A test histogram
# TYPE test_histogram histogram
test_histogram_bucket{le="0.1"} 10
test_histogram_bucket{le="0.5"} 20
test_histogram_bucket{le="1.0"} 30
test_histogram_bucket{le="+Inf"} 40
test_histogram_sum 25.5
test_histogram_count 40
`
}

func counterWithExemplar() string {
return `# TYPE test_counter counter
  test_counter_total 100 # {trace_id="abc123"} 5 1234567890
  `
}

Phase 1 Deliverables:
- ✅ Test framework with sender config loading
- ✅ Mock HTTP receiver with request capture
- ✅ Sender process launcher and lifecycle management
- ✅ Mock scrape target for feeding data
- ✅ HTTP protocol compliance tests (8 test cases)
- ✅ Symbol table structure tests (4 test cases)
- ✅ Validation helper functions
- ✅ Configuration example and documentation

  ---
Phase 2: Data Correctness & Encoding

Goal: Validate that senders correctly encode samples, histograms, exemplars, metadata, labels, and timestamps according to the specification.

2.1 Sample Encoding Tests (samples_test.go)

Spec Requirements:
- MUST send samples in timestamp order (older first) per series
- MUST use float64 for values
- MUST use int64 milliseconds since Unix epoch
- MUST use special NaN 0x7ff0000000000002 for stale markers
- SHOULD include complete label set
- SHOULD include metric name as __name__ label

Test Cases (15+ tests):

func TestSampleEncoding(t *testing.T) {
tests := []struct {
name        string
description string
rfcLevel    string
scrapeData  string
validator   func(*testing.T, *writev2.Request)
}{
{
name: "basic_float_sample",
description: "Sender MUST encode basic float samples correctly",
rfcLevel: "MUST",
scrapeData: "test_metric 42.5\n",
validator: func(t *testing.T, req *writev2.Request) {
ts := findTimeseriesByName(req, "test_metric")
must(t).NotNil(ts, "Timeseries must exist")
must(t).Equal(1, len(ts.Samples), "Must have 1 sample")
must(t).Equal(42.5, ts.Samples[0].Value)
},
},
{
name: "special_float_values",
description: "Sender MUST handle special float values (NaN, Inf, -Inf)",
rfcLevel: "MUST",
scrapeData: `
  test_nan NaN
  test_inf +Inf
  test_neg_inf -Inf
  `,
validator: func(t *testing.T, req *writev2.Request) {
tsNan := findTimeseriesByName(req, "test_nan")
must(t).True(math.IsNaN(tsNan.Samples[0].Value))

                  tsInf := findTimeseriesByName(req, "test_inf")
                  must(t).True(math.IsInf(tsInf.Samples[0].Value, 1))

                  tsNegInf := findTimeseriesByName(req, "test_neg_inf")
                  must(t).True(math.IsInf(tsNegInf.Samples[0].Value, -1))
              },
          },
          {
              name: "stale_marker",
              description: "Sender SHOULD send stale markers with special NaN value when series disappear",
              rfcLevel: "SHOULD",
              // This test needs multiple scrapes: metric appears, then disappears
              scrapeData: "", // Custom test logic needed
              validator: func(t *testing.T, req *writev2.Request) {
                  // Validate stale marker NaN value: 0x7ff0000000000002
                  // This requires multiple scrape cycles
                  t.Skip("Requires multi-scrape test infrastructure")
              },
          },
          {
              name: "timestamp_milliseconds",
              description: "Sender MUST use int64 milliseconds since Unix epoch",
              rfcLevel: "MUST",
              scrapeData: "test_metric 42\n",
              validator: func(t *testing.T, req *writev2.Request) {
                  ts := findTimeseriesByName(req, "test_metric")
                  timestamp := ts.Samples[0].Timestamp

                  // Timestamp should be reasonable (within last 10 minutes)
                  now := time.Now().Unix() * 1000
                  must(t).True(
                      timestamp > now-600000 && timestamp <= now,
                      "Timestamp %d not in reasonable range", timestamp,
                  )
              },
          },
          {
              name: "timestamp_ordering",
              description: "Sender MUST send samples in timestamp order (older first)",
              rfcLevel: "MUST",
              scrapeData: "", // Requires multiple samples per series
              validator: func(t *testing.T, req *writev2.Request) {
                  for _, ts := range req.Timeseries {
                      for i := 1; i < len(ts.Samples); i++ {
                          must(t).True(
                              ts.Samples[i].Timestamp >= ts.Samples[i-1].Timestamp,
                              "Samples must be in timestamp order",
                          )
                      }
                  }
              },
          },
          {
              name: "complete_labelset",
              description: "Sender SHOULD include complete label set with each sample",
              rfcLevel: "SHOULD",
              scrapeData: `test_metric{foo="bar",baz="qux"} 42`,
              validator: func(t *testing.T, req *writev2.Request) {
                  ts := findTimeseriesByName(req, "test_metric")
                  labels := extractLabels(req, ts)

                  should(t).Contains(labels, "__name__")
                  should(t).Contains(labels, "foo")
                  should(t).Contains(labels, "baz")
                  should(t).Contains(labels, "job")
                  should(t).Contains(labels, "instance")
              },
          },
          {
              name: "counter_created_timestamp",
              description: "Sender SHOULD provide created_timestamp for counter semantics",
              rfcLevel: "SHOULD",
              scrapeData: "test_counter_total 42\n",
              validator: func(t *testing.T, req *writev2.Request) {
                  ts := findTimeseriesByName(req, "test_counter_total")
                  for _, sample := range ts.Samples {
                      should(t).NotZero(
                          sample.CreatedTimestamp,
                          "Counter should have created_timestamp",
                      )
                  }
              },
          },
          // Additional test cases for edge cases...
      }

      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              t.Attr("rfcLevel", tt.rfcLevel)
              t.Attr("description", tt.description)
              runSenderTestWithValidator(t, tt.scrapeData, tt.validator)
          })
      }
}

2.2 Histogram Tests (histograms_test.go)

Spec Requirements:
- MUST send histograms in timestamp order per series
- MUST follow native histogram specification
- MUST NOT mix samples and histograms in same TimeSeries

Test Cases (12+ tests):

func TestHistogramEncoding(t *testing.T) {
tests := []struct {
name        string
description string
rfcLevel    string
scrapeData  string
validator   func(*testing.T, *writev2.Request)
}{
{
name: "basic_native_histogram",
description: "Sender MUST encode native histograms correctly",
rfcLevel: "MUST",
scrapeData: nativeHistogramScrape(),
validator: func(t *testing.T, req *writev2.Request) {
ts := findTimeseriesByName(req, "test_histogram")
must(t).NotNil(ts, "Histogram timeseries must exist")
must(t).Greater(len(ts.Histograms), 0, "Must have histograms")

                  hist := ts.Histograms[0]
                  must(t).NotNil(hist, "Histogram must not be nil")
                  // Validate histogram structure
              },
          },
          {
              name: "no_mixed_samples_histograms",
              description: "Sender MUST NOT include both samples and histograms in same TimeSeries",
              rfcLevel: "MUST",
              scrapeData: mixedMetrics(),
              validator: func(t *testing.T, req *writev2.Request) {
                  for i, ts := range req.Timeseries {
                      hasSamples := len(ts.Samples) > 0
                      hasHistograms := len(ts.Histograms) > 0
                      must(t).False(
                          hasSamples && hasHistograms,
                          "Timeseries[%d] cannot have both samples and histograms", i,
                      )
                  }
              },
          },
          {
              name: "histogram_timestamp_ordering",
              description: "Sender MUST send histograms in timestamp order",
              rfcLevel: "MUST",
              scrapeData: "", // Multi-scrape scenario
              validator: func(t *testing.T, req *writev2.Request) {
                  for _, ts := range req.Timeseries {
                      for i := 1; i < len(ts.Histograms); i++ {
                          must(t).True(
                              ts.Histograms[i].Timestamp >= ts.Histograms[i-1].Timestamp,
                              "Histograms must be in timestamp order",
                          )
                      }
                  }
              },
          },
          {
              name: "histogram_bucket_structure",
              description: "Sender MUST encode histogram buckets according to spec",
              rfcLevel: "MUST",
              scrapeData: nativeHistogramScrape(),
              validator: func(t *testing.T, req *writev2.Request) {
                  ts := findTimeseriesByName(req, "test_histogram")
                  hist := ts.Histograms[0]

                  // Validate positive/negative spans and deltas
                  must(t).NotNil(hist.PositiveSpans)
                  // Additional bucket validation...
              },
          },
          {
              name: "histogram_created_timestamp",
              description: "Sender SHOULD provide created_timestamp for histograms",
              rfcLevel: "SHOULD",
              scrapeData: nativeHistogramScrape(),
              validator: func(t *testing.T, req *writev2.Request) {
                  ts := findTimeseriesByName(req, "test_histogram")
                  for _, hist := range ts.Histograms {
                      should(t).NotZero(
                          hist.CreatedTimestamp,
                          "Histogram should have created_timestamp",
                      )
                  }
              },
          },
          // Additional histogram test cases...
      }

      // Test execution similar to samples
}

2.3 Exemplar Tests (exemplars_test.go)

Spec Requirements:
- SHOULD provide exemplars if they exist
- MUST include value (double), timestamp (int64 ms)
- MAY include labels (typically trace_id)
- SHOULD use trace_id as label name (best practice)

Test Cases (8+ tests):

func TestExemplarEncoding(t *testing.T) {
// Tests for exemplar attachment, trace_id labels, timestamps, etc.
// Similar structure to receiver exemplar_test.go but validating sender behavior
}

2.4 Metadata Tests (metadata_test.go)

Spec Requirements:
- SHOULD provide metadata sub-fields for each TimeSeries
- MUST follow Prometheus Type and Help guidelines
- SHOULD follow OpenMetrics Unit guidelines

Test Cases (10+ tests):

func TestMetadataEncoding(t *testing.T) {
tests := []struct {
name        string
description string
rfcLevel    string
scrapeData  string
validator   func(*testing.T, *writev2.Request)
}{
{
name: "metadata_present",
description: "Sender SHOULD include metadata for metrics",
rfcLevel: "SHOULD",
scrapeData: `# HELP test_metric A test metric
# TYPE test_metric counter
test_metric_total 42
`,
validator: func(t *testing.T, req *writev2.Request) {
should(t).Greater(len(req.Metadata), 0, "Should include metadata")
},
},
{
name: "metadata_type_values",
description: "Sender MUST use valid MetricType enum values",
rfcLevel: "MUST",
scrapeData: metricsWithAllTypes(),
validator: func(t *testing.T, req *writev2.Request) {
validTypes := []writev2.Metadata_MetricType{
writev2.Metadata_METRIC_TYPE_COUNTER,
writev2.Metadata_METRIC_TYPE_GAUGE,
writev2.Metadata_METRIC_TYPE_HISTOGRAM,
writev2.Metadata_METRIC_TYPE_GAUGEHISTOGRAM,
writev2.Metadata_METRIC_TYPE_SUMMARY,
writev2.Metadata_METRIC_TYPE_INFO,
writev2.Metadata_METRIC_TYPE_STATESET,
}

                  for _, md := range req.Metadata {
                      must(t).Contains(validTypes, md.Type, "Invalid metric type")
                  }
              },
          },
          {
              name: "metadata_help_text",
              description: "Sender SHOULD include help text from scrape",
              rfcLevel: "SHOULD",
              scrapeData: `# HELP test_metric This is help text
# TYPE test_metric gauge
test_metric 42
`,
            validator: func(t *testing.T, req *writev2.Request) {
                md := findMetadataByName(req, "test_metric")
                should(t).Equal("This is help text", md.Help)
            },
        },
        {
            name: "metadata_unit",
            description: "Sender SHOULD include unit following OpenMetrics guidelines",
            rfcLevel: "SHOULD",
            scrapeData: `# UNIT test_metric_seconds seconds
# TYPE test_metric_seconds gauge
test_metric_seconds 42
`,
validator: func(t *testing.T, req *writev2.Request) {
md := findMetadataByName(req, "test_metric_seconds")
should(t).Equal("seconds", md.Unit)
},
},
// Additional metadata tests...
}
}

2.5 Label Validation Tests (labels_test.go)

Spec Requirements:
- MUST sort label names lexicographically
- MUST include __name__ label (SHOULD)
- SHOULD follow metric name regex [a-zA-Z_:]([a-zA-Z0-9_:])*
- SHOULD follow label name regex [a-zA-Z_]([a-zA-Z0-9_])*
- MUST NOT have repeated label names
- MUST NOT have empty label names or values
- MUST NOT use label names beginning with "__" (user-provided)

Test Cases (12+ tests):

func TestLabelValidation(t *testing.T) {
tests := []struct {
name        string
description string
rfcLevel    string
scrapeData  string
validator   func(*testing.T, *writev2.Request)
}{
{
name: "lexicographic_ordering",
description: "Sender MUST sort label names lexicographically",
rfcLevel: "MUST",
scrapeData: `test_metric{z="1",a="2",m="3"} 42`,
validator: func(t *testing.T, req *writev2.Request) {
ts := findTimeseriesByName(req, "test_metric")
labels := extractLabelsOrdered(req, ts) // Returns ordered pairs

                  for i := 1; i < len(labels); i++ {
                      must(t).True(
                          labels[i].Name > labels[i-1].Name,
                          "Labels must be lexicographically sorted",
                      )
                  }
              },
          },
          {
              name: "name_label_present",
              description: "Sender SHOULD include __name__ label",
              rfcLevel: "SHOULD",
              scrapeData: "test_metric 42\n",
              validator: func(t *testing.T, req *writev2.Request) {
                  for _, ts := range req.Timeseries {
                      labels := extractLabels(req, ts)
                      should(t).Contains(labels, "__name__", "Should have __name__ label")
                  }
              },
          },
          {
              name: "valid_metric_name_format",
              description: "Sender SHOULD follow metric name regex",
              rfcLevel: "SHOULD",
              scrapeData: "valid_metric_name:total 42\n",
              validator: func(t *testing.T, req *writev2.Request) {
                  metricNameRegex := regexp.MustCompile(`^[a-zA-Z_:]([a-zA-Z0-9_:])*$`)

                  for _, ts := range req.Timeseries {
                      labels := extractLabels(req, ts)
                      name := labels["__name__"]
                      should(t).True(
                          metricNameRegex.MatchString(name),
                          "Metric name '%s' should match regex", name,
                      )
                  }
              },
          },
          {
              name: "valid_label_name_format",
              description: "Sender SHOULD follow label name regex",
              rfcLevel: "SHOULD",
              scrapeData: `test{valid_label="value"} 42`,
              validator: func(t *testing.T, req *writev2.Request) {
                  labelNameRegex := regexp.MustCompile(`^[a-zA-Z_]([a-zA-Z0-9_])*$`)

                  for _, ts := range req.Timeseries {
                      labels := extractLabels(req, ts)
                      for name := range labels {
                          if !strings.HasPrefix(name, "__") { // System labels excluded
                              should(t).True(
                                  labelNameRegex.MatchString(name),
                                  "Label name '%s' should match regex", name,
                              )
                          }
                      }
                  }
              },
          },
          {
              name: "no_duplicate_labels",
              description: "Sender MUST NOT include repeated label names",
              rfcLevel: "MUST",
              scrapeData: "test_metric 42\n",
              validator: func(t *testing.T, req *writev2.Request) {
                  for _, ts := range req.Timeseries {
                      labels := extractLabels(req, ts)
                      // If extraction succeeded, no duplicates (map would overwrite)
                      // But also check raw refs
                      seen := make(map[string]bool)
                      for i := 0; i < len(ts.LabelsRefs); i += 2 {
                          key := req.Symbols[ts.LabelsRefs[i]]
                          must(t).False(seen[key], "Duplicate label: %s", key)
                          seen[key] = true
                      }
                  }
              },
          },
          {
              name: "no_empty_label_names",
              description: "Sender MUST NOT include empty label names",
              rfcLevel: "MUST",
              scrapeData: "test_metric 42\n",
              validator: func(t *testing.T, req *writev2.Request) {
                  for _, ts := range req.Timeseries {
                      for i := 0; i < len(ts.LabelsRefs); i += 2 {
                          key := req.Symbols[ts.LabelsRefs[i]]
                          must(t).NotEmpty(key, "Label name must not be empty")
                      }
                  }
              },
          },
          {
              name: "no_empty_label_values",
              description: "Sender MUST NOT include empty label values",
              rfcLevel: "MUST",
              scrapeData: "test_metric 42\n",
              validator: func(t *testing.T, req *writev2.Request) {
                  for _, ts := range req.Timeseries {
                      for i := 1; i < len(ts.LabelsRefs); i += 2 {
                          val := req.Symbols[ts.LabelsRefs[i]]
                          must(t).NotEmpty(val, "Label value must not be empty")
                      }
                  }
              },
          },
          // Tests for reserved __ prefix, special characters, etc.
      }
}

2.6 Timestamp Tests (timestamps_test.go)

Test Cases:
- Timestamp format (int64 milliseconds)
- Timestamp ordering within series
- Created timestamp for counters
- Created timestamp zero value handling

2.7 Integration Tests (combined_test.go)

Test Cases:
- Samples + metadata + exemplars together
- Histograms + metadata + exemplars together
- Multiple metric families (counter, gauge, histogram, summary)
- Large requests with many timeseries
- Real-world scrape scenarios

Phase 2 Deliverables:
- ✅ Sample encoding tests (15+ test cases)
- ✅ Histogram encoding tests (12+ test cases)
- ✅ Exemplar tests (8+ test cases)
- ✅ Metadata tests (10+ test cases)
- ✅ Label validation tests (12+ test cases)
- ✅ Timestamp tests (6+ test cases)
- ✅ Integration tests (8+ test cases)
- ✅ Test data generators for all metric types

  ---
Phase 3: Behavior & Reliability Testing

Goal: Validate sender retry logic, backoff behavior, batching, error handling, and response processing.

3.1 Retry Behavior Tests (retry_test.go)

Spec Requirements:
- MUST NOT retry on 4xx (except 429)
- MUST retry on 5xx
- MAY retry on 429
- MUST use backoff algorithm

Test Cases (10+ tests):

func TestRetryBehavior(t *testing.T) {
tests := []struct {
name         string
description  string
rfcLevel     string
responseCode int
shouldRetry  bool
validator    func(*testing.T, *MockReceiver)
}{
{
name: "no_retry_on_4xx",
description: "Sender MUST NOT retry on 4xx errors",
rfcLevel: "MUST",
responseCode: 400,
shouldRetry: false,
validator: func(t *testing.T, mock *MockReceiver) {
// Wait for retry window
time.Sleep(5 * time.Second)
requestCount := len(mock.GetAllRequests())
must(t).Equal(1, requestCount, "Must not retry on 4xx")
},
},
{
name: "retry_on_500",
description: "Sender MUST retry on 500 Internal Server Error",
rfcLevel: "MUST",
responseCode: 500,
shouldRetry: true,
validator: func(t *testing.T, mock *MockReceiver) {
// Wait for retries
time.Sleep(10 * time.Second)
requestCount := len(mock.GetAllRequests())
must(t).Greater(requestCount, 1, "Must retry on 500")
},
},
{
name: "retry_on_503",
description: "Sender MUST retry on 503 Service Unavailable",
rfcLevel: "MUST",
responseCode: 503,
shouldRetry: true,
validator: func(t *testing.T, mock *MockReceiver) {
time.Sleep(10 * time.Second)
requestCount := len(mock.GetAllRequests())
must(t).Greater(requestCount, 1, "Must retry on 503")
},
},
{
name: "may_retry_on_429",
description: "Sender MAY retry on 429 Too Many Requests",
rfcLevel: "MAY",
responseCode: 429,
shouldRetry: true,
validator: func(t *testing.T, mock *MockReceiver) {
time.Sleep(10 * time.Second)
requestCount := len(mock.GetAllRequests())
may(t).Greater(requestCount, 1, "May retry on 429")
},
},
{
name: "retry_after_header",
description: "Sender MAY honor Retry-After response header",
rfcLevel: "MAY",
responseCode: 503,
validator: func(t *testing.T, mock *MockReceiver) {
// Set Retry-After: 5 seconds
mock.SetResponse(503, nil, http.Header{
"Retry-After": []string{"5"},
})

                  req1Time := time.Now()
                  <-mock.requests // First request

                  req2Time := time.Now()
                  select {
                  case <-mock.requests:
                      delay := req2Time.Sub(req1Time)
                      may(t).GreaterOrEqual(
                          delay, 5*time.Second,
                          "Should wait at least Retry-After duration",
                      )
                  case <-time.After(15 * time.Second):
                      t.Fatal("Expected retry within 15 seconds")
                  }
              },
          },
          {
              name: "eventual_success_after_retries",
              description: "Sender should succeed after transient failures",
              rfcLevel: "SHOULD",
              validator: func(t *testing.T, mock *MockReceiver) {
                  // Start with 503, switch to 204 after 2 requests
                  requestCount := 0
                  mock.SetHandler(func(w http.ResponseWriter, r *http.Request) {
                      requestCount++
                      if requestCount <= 2 {
                          w.WriteHeader(503)
                      } else {
                          w.WriteHeader(204)
                      }
                  })

                  // Eventually sender should succeed
                  time.Sleep(30 * time.Second)
                  should(t).GreaterOrEqual(requestCount, 3, "Should eventually succeed")
              },
          },
      }

      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              t.Attr("rfcLevel", tt.rfcLevel)
              t.Attr("description", tt.description)

              mock := newMockReceiver(MockReceiverOpts{
                  ResponseCode: tt.responseCode,
              })
              mock.Start(t)
              defer mock.Stop()

              scrape := newMockScrapeTarget(simpleCounter())
              scrapeURL := scrape.Start()
              defer scrape.Stop()

              sender := launchSender(t, getSenderConfig(), mock.URL, scrapeURL)
              defer sender.Stop()

              tt.validator(t, mock)
          })
      }
}

3.2 Backoff Tests (backoff_test.go)

Spec Requirements:
- MUST use backoff algorithm to prevent server overload

Test Cases:

func TestBackoffBehavior(t *testing.T) {
tests := []struct {
name        string
description string
validator   func(*testing.T, []time.Time)
}{
{
name: "exponential_backoff",
description: "Sender MUST use backoff algorithm (typically exponential)",
validator: func(t *testing.T, requestTimes []time.Time) {
// Validate delays between requests increase
for i := 2; i < len(requestTimes); i++ {
delay1 := requestTimes[i-1].Sub(requestTimes[i-2])
delay2 := requestTimes[i].Sub(requestTimes[i-1])

                      // Each delay should be >= previous (with some tolerance)
                      must(t).True(
                          delay2 >= delay1*0.9, // 10% tolerance for jitter
                          "Backoff should increase: delay1=%v, delay2=%v",
                          delay1, delay2,
                      )
                  }
              },
          },
          {
              name: "backoff_max_delay",
              description: "Sender should have reasonable max backoff delay",
              validator: func(t *testing.T, requestTimes []time.Time) {
                  // No single delay should exceed e.g. 1 minute
                  for i := 1; i < len(requestTimes); i++ {
                      delay := requestTimes[i].Sub(requestTimes[i-1])
                      should(t).LessOrEqual(
                          delay, 60*time.Second,
                          "Backoff delay should have reasonable max",
                      )
                  }
              },
          },
      }

      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              mock := newMockReceiver(MockReceiverOpts{ResponseCode: 503})
              mock.Start(t)
              defer mock.Stop()

              // Collect request timestamps
              var requestTimes []time.Time
              var mu sync.Mutex

              mock.SetHandler(func(w http.ResponseWriter, r *http.Request) {
                  mu.Lock()
                  requestTimes = append(requestTimes, time.Now())
                  mu.Unlock()
                  w.WriteHeader(503)
              })

              // Run sender for 60 seconds
              sender := launchSenderWithData(t, mock.URL, simpleCounter())
              time.Sleep(60 * time.Second)
              sender.Stop()

              mu.Lock()
              times := make([]time.Time, len(requestTimes))
              copy(times, requestTimes)
              mu.Unlock()

              tt.validator(t, times)
          })
      }
}

3.3 Batching Tests (batching_test.go)

Spec Requirements:
- SHOULD send samples for multiple series in single request
- CAN send multiple requests in parallel for different series

Test Cases:

func TestBatchingBehavior(t *testing.T) {
tests := []struct {
name        string
description string
scrapeData  string
validator   func(*testing.T, []*CapturedRequest)
}{
{
name: "multiple_series_per_request",
description: "Sender SHOULD batch multiple series in single request",
scrapeData: `
  metric1 1
  metric2 2
  metric3 3
  metric4 4
  metric5 5
  `,
validator: func(t *testing.T, reqs []*CapturedRequest) {
// Should send all metrics in one (or few) requests
should(t).LessOrEqual(len(reqs), 2, "Should batch metrics")

                  // First request should contain multiple timeseries
                  if len(reqs) > 0 {
                      should(t).Greater(
                          len(reqs[0].Proto.Timeseries), 1,
                          "Request should contain multiple series",
                      )
                  }
              },
          },
          {
              name: "parallel_requests_supported",
              description: "Sender CAN send parallel requests for different series",
              scrapeData: largeMultiSeriesScrape(), // 100+ series
              validator: func(t *testing.T, reqs []*CapturedRequest) {
                  // Check if any requests overlap in time (parallel)
                  // This is optional (CAN), so may or may not happen
              },
          },
          {
              name: "queue_full_behavior",
              description: "Sender should handle queue full scenarios gracefully",
              scrapeData: massiveDataLoad(), // Huge amount of data
              validator: func(t *testing.T, reqs []*CapturedRequest) {
                  // Sender should either:
                  // 1. Drop old data
                  // 2. Apply backpressure
                  // 3. Buffer appropriately
                  // At minimum, shouldn't crash
                  should(t).Greater(len(reqs), 0, "Should send some data")
              },
          },
      }
}

3.4 Error Handling Tests (error_handling_test.go)

Test Cases:

func TestErrorHandling(t *testing.T) {
tests := []struct {
name        string
description string
scenario    ErrorScenario
validator   func(*testing.T, *SenderInstance, *MockReceiver)
}{
{
name: "network_timeout",
description: "Sender should handle network timeouts gracefully",
scenario: NetworkTimeoutScenario{},
validator: func(t *testing.T, sender *SenderInstance, mock *MockReceiver) {
// Sender should retry after timeout
time.Sleep(15 * time.Second)
should(t).True(sender.IsRunning(), "Sender should still be running")
},
},
{
name: "connection_refused",
description: "Sender should handle connection refused",
scenario: ConnectionRefusedScenario{},
validator: func(t *testing.T, sender *SenderInstance, mock *MockReceiver) {
// Sender should keep trying
should(t).True(sender.IsRunning())
},
},
{
name: "partial_write",
description: "Sender should handle partial HTTP writes",
scenario: PartialWriteScenario{},
validator: func(t *testing.T, sender *SenderInstance, mock *MockReceiver) {
// Implementation-specific behavior
},
},
{
name: "malformed_response",
description: "Sender should handle malformed HTTP responses",
scenario: MalformedResponseScenario{},
validator: func(t *testing.T, sender *SenderInstance, mock *MockReceiver) {
should(t).True(sender.IsRunning())
},
},
}
}

3.5 Response Processing Tests (response_test.go)

Spec Requirements:
- SHOULD ignore response body on 2xx
- MAY use X-Prometheus-Remote-Write-*-Written headers to confirm
- SHOULD assume missing headers mean zero written (for 2.0)
- SHOULD assume 415 if 2xx but no headers (for 2.0 message to 1.x receiver)
- MUST log error messages as-is

Test Cases:

func TestResponseProcessing(t *testing.T) {
tests := []struct {
name        string
description string
response    ResponseConfig
validator   func(*testing.T, *SenderInstance)
}{
{
name: "ignore_response_body_on_success",
description: "Sender SHOULD ignore response body on 2xx",
response: ResponseConfig{
Code: 204,
Body: []byte("some random body"),
},
validator: func(t *testing.T, sender *SenderInstance) {
// Should continue normally despite response body
should(t).True(sender.IsRunning())
// Check logs don't show errors
},
},
{
name: "use_written_count_headers",
description: "Sender MAY use X-Prometheus-Remote-Write-*-Written headers",
response: ResponseConfig{
Code: 204,
Headers: http.Header{
"X-Prometheus-Remote-Write-Samples-Written": []string{"10"},
},
},
validator: func(t *testing.T, sender *SenderInstance) {
// If sender supports verification, it should use these headers
// This is optional (MAY)
},
},
{
name: "missing_headers_on_2xx",
description: "Sender SHOULD assume 415 if 2xx with no headers for RW 2.0",
response: ResponseConfig{
Code: 200,
Headers: http.Header{}, // No written count headers
},
validator: func(t *testing.T, sender *SenderInstance) {
// Sender might attempt fallback to RW 1.0
// Check logs or next request format
},
},
{
name: "log_error_messages",
description: "Sender MUST log error messages as-is without interpretation",
response: ResponseConfig{
Code: 400,
Body: []byte("Custom error: invalid metric name"),
},
validator: func(t *testing.T, sender *SenderInstance) {
logs := sender.Logs()
must(t).Contains(
logs, "Custom error: invalid metric name",
"Error message must be logged as-is",
)
},
},
}
}

Phase 3 Deliverables:
- ✅ Retry behavior tests (10+ test cases)
- ✅ Backoff validation tests (5+ test cases)
- ✅ Batching behavior tests (6+ test cases)
- ✅ Error handling tests (8+ test cases)
- ✅ Response processing tests (6+ test cases)
- ✅ Advanced mock server with timing capabilities
- ✅ Network failure simulation utilities

  ---
Phase 4: Backward Compatibility & Edge Cases

Goal: Validate RW 1.0 backward compatibility, content-type fallback, and edge case handling.

4.1 RW 1.0 Compatibility Tests (rw1_compat_test.go)

Spec Requirements:
- MUST support 1.x receivers via configurable content type
- SHOULD use X-Prometheus-Remote-Write-Version: 0.1.0 for 1.x
- SHOULD allow automatic fallback on 415

Test Cases:

func TestRW1Compatibility(t *testing.T) {
tests := []struct {
name        string
description string
rfcLevel    string
validator   func(*testing.T, *CapturedRequest)
}{
{
name: "send_rw1_format",
description: "Sender MUST support sending RW 1.0 format when configured",
rfcLevel: "MUST",
validator: func(t *testing.T, req *CapturedRequest) {
// Check for prometheus.WriteRequest format
var rw1Req prompb.WriteRequest
decompressed, _ := snappy.Decode(nil, req.Body)
err := proto.Unmarshal(decompressed, &rw1Req)
must(t).NoError(err, "Must parse as RW 1.0 format")
},
},
{
name: "rw1_version_header",
description: "Sender SHOULD use version 0.1.0 header for RW 1.0",
rfcLevel: "SHOULD",
validator: func(t *testing.T, req *CapturedRequest) {
version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
should(t).Equal("0.1.0", version)
},
},
{
name: "rw1_content_type",
description: "Sender SHOULD use basic content-type for RW 1.0",
rfcLevel: "SHOULD",
validator: func(t *testing.T, req *CapturedRequest) {
ct := req.Headers.Get("Content-Type")
should(t).Equal("application/x-protobuf", ct)
should(t).NotContains(ct, "proto=io.prometheus.write.v2")
},
},
}
}

4.2 Fallback Tests (fallback_test.go)

Spec Requirements:
- SHOULD allow automatic fallback on 415
- MAY retry with different content-type/encoding

Test Cases:

func TestContentTypeFallback(t *testing.T) {
tests := []struct {
name        string
description string
scenario    FallbackScenario
validator   func(*testing.T, []*CapturedRequest)
}{
{
name: "fallback_on_415",
description: "Sender SHOULD fallback to RW 1.0 on 415 Unsupported Media Type",
scenario: FallbackScenario{
FirstResponse: 415,
SecondResponse: 204,
},
validator: func(t *testing.T, reqs []*CapturedRequest) {
should(t).GreaterOrEqual(len(reqs), 2, "Should retry with fallback")

                  // First request: RW 2.0
                  firstCT := reqs[0].Headers.Get("Content-Type")
                  should(t).Contains(firstCT, "write.v2")

                  // Second request: RW 1.0
                  secondCT := reqs[1].Headers.Get("Content-Type")
                  should(t).NotContains(secondCT, "write.v2")
              },
          },
          {
              name: "remember_fallback_choice",
              description: "Sender SHOULD remember successful fallback for future requests",
              validator: func(t *testing.T, reqs []*CapturedRequest) {
                  // After fallback, subsequent requests should use RW 1.0
                  for i := 2; i < len(reqs); i++ {
                      ct := reqs[i].Headers.Get("Content-Type")
                      should(t).NotContains(ct, "write.v2")
                  }
              },
          },
      }
}

4.3 Edge Case Tests (edge_cases_test.go)

Test Cases:

func TestEdgeCases(t *testing.T) {
tests := []struct {
name        string
description string
scrapeData  string
validator   func(*testing.T, *writev2.Request)
}{
{
name: "empty_scrape",
description: "Sender should handle empty scrape results",
scrapeData: "",
validator: func(t *testing.T, req *writev2.Request) {
// Sender might not send anything, or send empty request
// Both behaviors acceptable
},
},
{
name: "huge_label_values",
description: "Sender should handle very large label values",
scrapeData: fmt.Sprintf(`test{big="%s"} 1`, strings.Repeat("a", 10000)),
validator: func(t *testing.T, req *writev2.Request) {
// Should handle without crashing
should(t).Greater(len(req.Timeseries), 0)
},
},
{
name: "unicode_in_labels",
description: "Sender should handle unicode in label values",
scrapeData: `test{emoji="🚀",japanese="日本語"} 1`,
validator: func(t *testing.T, req *writev2.Request) {
ts := findTimeseriesByName(req, "test")
labels := extractLabels(req, ts)
should(t).Equal("🚀", labels["emoji"])
should(t).Equal("日本語", labels["japanese"])
},
},
{
name: "many_timeseries",
description: "Sender should handle large number of timeseries",
scrapeData: generateNMetrics(10000),
validator: func(t *testing.T, req *writev2.Request) {
// Might be split across multiple requests
// Should handle without memory issues
},
},
{
name: "high_cardinality",
description: "Sender should handle high cardinality metrics",
scrapeData: generateHighCardinalityMetrics(1000),
validator: func(t *testing.T, req *writev2.Request) {
// Symbol table should efficiently deduplicate
symbolCount := len(req.Symbols)
seriesCount := len(req.Timeseries)

                  // Symbols should be significantly less than theoretical max
                  // (each series could have ~5 unique values)
                  should(t).Less(symbolCount, seriesCount*5)
              },
          },
      }
}

4.4 Performance Tests (performance_test.go)

Optional - for reference implementations:

func TestPerformanceCharacteristics(t *testing.T) {
if testing.Short() {
t.Skip("Skipping performance tests in short mode")
}

      tests := []struct {
          name        string
          description string
          dataSize    int
          validator   func(*testing.T, PerformanceMetrics)
      }{
          {
              name: "throughput_1000_series",
              description: "Measure throughput with 1000 series",
              dataSize: 1000,
              validator: func(t *testing.T, metrics PerformanceMetrics) {
                  // Informational only - log metrics
                  t.Logf("Samples/sec: %f", metrics.SamplesPerSecond)
                  t.Logf("Requests/sec: %f", metrics.RequestsPerSecond)
              },
          },
          {
              name: "memory_usage",
              description: "Monitor memory usage under load",
              dataSize: 10000,
              validator: func(t *testing.T, metrics PerformanceMetrics) {
                  t.Logf("Peak memory: %d MB", metrics.PeakMemoryMB)
              },
          },
      }
}

Phase 4 Deliverables:
- ✅ RW 1.0 compatibility tests (8+ test cases)
- ✅ Content-type fallback tests (5+ test cases)
- ✅ Edge case tests (10+ test cases)
- ✅ Performance reference tests (optional, 5+ test cases)
- ✅ Comprehensive documentation and README
- ✅ CI/CD integration examples

  ---
Implementation Notes

Reused Components from Receiver Tests

Directly Reusable:
- RFC compliance markers (must(), should(), may())
- Protobuf structures (writev2.Request, writev2.TimeSeries, etc.)
- Symbol table parsing logic
- Label extraction utilities
- Test attribute system (t.Attr())
- Configuration loading patterns
- Table-driven test patterns

Adapted/Flipped:
- Request generation → Request validation
- HTTP client → HTTP server
- Response checking → Request checking
- External endpoint config → Sender binary config

New Components:
- Mock HTTP server framework
- Sender process launcher
- Mock scrape target
- Request capture and buffering
- Timing and retry validation
- Multi-scrape test orchestration

Test Data Management

Approach:
testdata/
├── scrapes/
│   ├── simple_counter.txt
│   ├── simple_gauge.txt
│   ├── native_histogram.txt
│   ├── with_exemplars.txt
│   ├── all_types.txt
│   └── high_cardinality.txt
├── configs/
│   ├── prometheus_template.yml
│   ├── grafana_agent_template.yml
│   └── otel_template.yml
└── expected/
├── counter_request.pb
└── histogram_request.pb

Configuration Management

Sender Configuration Format:
senders:
- name: prometheus
binary: ./bin/prometheus
config_template: |
# Go template with placeholders
remote_write:
- url: {{.RemoteWriteURL}}
ready_check:
type: http
url: http://localhost:9090/-/ready
timeout: 30s

    - name: custom-sender
      binary: ./bin/custom-sender
      config_template: |
        remote_write_endpoint: {{.RemoteWriteURL}}
      ready_check:
        type: log
        pattern: "Remote write client started"
        timeout: 10s

Environment Variables

- PROMETHEUS_RW2_COMPLIANCE_SENDER_CONFIG_FILE: Path to sender config file
- PROMETHEUS_RW2_COMPLIANCE_SENDERS: Comma-separated sender names to test
- PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT: Default test timeout
- PROMETHEUS_RW2_COMPLIANCE_SCRAPE_INTERVAL: Scrape interval for tests
- PROMETHEUS_RW2_COMPLIANCE_VERBOSE: Enable verbose logging

Multi-Scrape Test Infrastructure

Challenge: Some tests require multiple scrape cycles (e.g., stale markers, timestamp ordering).

Solution:
type MultiScrapeScenario struct {
Scrapes []ScrapeStep
}

type ScrapeStep struct {
Data     string
Wait     time.Duration
Validate func(*testing.T, *CapturedRequest)
}

func runMultiScrapeTest(t *testing.T, scenario MultiScrapeScenario) {
// Orchestrate multiple scrapes with validation at each step
}

  ---
Summary Statistics

Total Test Coverage

| Phase | Category                   | Test Files | Test Cases (Approx) |
  |-------|----------------------------|------------|---------------------|
| 1     | Foundation & Protocol      | 2 files    | 12+ tests           |
| 2     | Data Correctness           | 7 files    | 71+ tests           |
| 3     | Behavior & Reliability     | 5 files    | 35+ tests           |
| 4     | Compatibility & Edge Cases | 4 files    | 28+ tests           |
| Total | All                        | 18 files   | 146+ tests          |

Spec Coverage

- ✅ All MUST requirements covered
- ✅ All SHOULD requirements covered
- ✅ Most MAY requirements covered
- ✅ Edge cases and error scenarios covered
- ✅ Backward compatibility covered

Code Structure

~2500-3000 lines total (estimated)
├── Framework (main_test.go, helpers_test.go): ~800 lines
├── Protocol & Structure (Phase 1): ~400 lines
├── Data Validation (Phase 2): ~1000 lines
├── Behavior Tests (Phase 3): ~600 lines
└── Compatibility (Phase 4): ~300 lines

  ---
Next Steps

1. Review & Discuss: Refine proposal based on feedback
2. Phase 1 Implementation: Build foundation and protocol tests
3. Validate Framework: Ensure mock server and sender launcher work
4. Phase 2 Implementation: Add data validation tests
5. Phase 3 Implementation: Add behavior tests
6. Phase 4 Implementation: Add compatibility tests
7. Documentation: Write comprehensive README and usage guide
8. CI Integration: Add to continuous integration pipeline

  ---
Questions for Finalization

1. Sender Binary Management: Should we include example sender binaries or expect users to provide them?
2. Docker Support: Should senders run in containers instead of as processes?
3. Test Parallelization: Should tests run in parallel or sequentially per sender?
4. Performance Tests: Include performance benchmarks or keep them optional?
5. Test Duration: Some behavior tests (retry, backoff) may take 30-60 seconds. Acceptable?

⏺ ---
I've created a comprehensive, detailed proposal for the Remote Write 2.0 Sender Compliance Test Suite. Here's what I've delivered:

📊 Proposal Overview

Total Coverage: 146+ test cases across 18 test files, organized into 4 implementation phases

Phase Breakdown

1. Phase 1: Foundation & Protocol (12+ tests)
   - Test framework with sender launcher
   - Mock HTTP receiver with request capture
   - HTTP protocol compliance validation
   - Symbol table structure tests
2. Phase 2: Data Correctness (71+ tests)
   - Sample encoding (15+ tests)
   - Histogram encoding (12+ tests)
   - Exemplar tests (8+ tests)
   - Metadata tests (10+ tests)
   - Label validation (12+ tests)
   - Timestamp tests (6+ tests)
   - Integration tests (8+ tests)
3. Phase 3: Behavior & Reliability (35+ tests)
   - Retry behavior (10+ tests)
   - Backoff validation (5+ tests)
   - Batching behavior (6+ tests)
   - Error handling (8+ tests)
   - Response processing (6+ tests)
4. Phase 4: Compatibility & Edge Cases (28+ tests)
   - RW 1.0 compatibility (8+ tests)
   - Content-type fallback (5+ tests)
   - Edge cases (10+ tests)
   - Performance tests (5+ tests, optional)

🎯 Key Architectural Decisions

- ✅ Mock HTTP Server for receiving requests (vs client sending)
- ✅ Process Launcher for forking sender binaries
- ✅ Mock Scrape Target for feeding data to senders
- ✅ Request Capture and validation (vs response validation)
- ✅ Reuses RFC markers, protobuf structures, test patterns from receiver tests
- ✅ Adds new components specific to sender testing
