package cases

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// Protocol Section Tests for Remote Write 2.0 Specification
// Ref: https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/

// =============================================================================
// 0. Strict RW 2.0-Only Receiver Tests
// =============================================================================

// StrictRW2ReceiverTest verifies that senders can communicate with RW 2.0-ONLY receivers
// that do NOT support backward compatibility with RW 1.0.
//
// This test uses a receiver that ONLY accepts remote.WriteV2MessageType, simulating
// a strict RW 2.0-only receiver that rejects RW 1.0 requests with HTTP 415.
//
// Expected behavior:
//   - RW 2.0 compliant senders (Prometheus v3.7.1+): PASS ✅
//   - RW 1.0 only senders (Prometheus <v3.0): FAIL (receiver rejects with 415)
//
// Configuration required for RW 2.0 compliance:
//   remote_write:
//     - url: <endpoint>
//       protobuf_message: "io.prometheus.write.v2.Request"
//
// This test validates that the sender:
//   1. Sends Content-Type: application/x-protobuf;proto=io.prometheus.write.v2.Request
//   2. Sends X-Prometheus-Remote-Write-Version: 2.0.0
//   3. Uses io.prometheus.write.v2.Request protobuf message format
func StrictRW2ReceiverTest() Test {
	return Test{
		Name: "StrictRW2Receiver",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "strict_rw2_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		// Force receiver to ONLY accept RW 2.0 format
		ReceiverVersion: []remote.WriteMessageType{remote.WriteV2MessageType},
		Expected: func(t *testing.T, bs []Batch) {
			// With Prometheus v3.7.1+ configured with protobuf_message: "io.prometheus.write.v2.Request",
			// this test should pass as the sender is RW 2.0 compliant
			if len(bs) == 0 {
				t.Logf("FAILURE: Sender does not support RW 2.0 format")
				t.Logf("The RW 2.0-only receiver rejected the request (likely HTTP 415)")
				t.Logf("Ensure your Prometheus configuration includes:")
				t.Logf("  remote_write:")
				t.Logf("    - url: <endpoint>")
				t.Logf("      protobuf_message: \"io.prometheus.write.v2.Request\"")
			}
			require.NotEmpty(t, bs, "RW 2.0-only receiver should accept data from compliant senders")
		},
	}
}

// =============================================================================
// 1. Protobuf Serialization Tests (4 tests)
// =============================================================================

// ProtobufV2FormatTest verifies that the sender uses io.prometheus.write.v2.Request
// for RW 2.0 receivers (SHOULD requirement for backward compatibility).
func ProtobufV2FormatTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "ProtobufV2Format",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "protocol_v2_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ct := r.Header.Get("Content-Type")
				// Check if Content-Type indicates RW 2.0 format
				if strings.Contains(ct, "io.prometheus.write.v2.Request") {
					// RW 2.0 format detected - this is what we expect
					next.ServeHTTP(w, r)
				} else if strings.Contains(ct, "prometheus.WriteRequest") || ct == "application/x-protobuf" {
					// RW 1.0 format - sender SHOULD upgrade to v2 for v2 receivers
					// But backward compatibility is allowed
					ec <- fmt.Errorf("sender used RW 1.0 format (Content-Type: %s), SHOULD use RW 2.0 for v2 receivers", ct)
					next.ServeHTTP(w, r)
				} else {
					ec <- fmt.Errorf("unexpected Content-Type: %s", ct)
					http.Error(w, "invalid content type", http.StatusUnsupportedMediaType)
				}
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			// SHOULD requirement - log warnings but don't fail for backward compatibility
			if len(errors) > 0 {
				for _, err := range errors {
					fmt.Printf("WARNING: %v\n", err)
				}
			}
		},
	}
}

// ProtobufBinaryFormatTest verifies that Binary Wire Format is used (MUST requirement).
func ProtobufBinaryFormatTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "ProtobufBinaryFormat",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "binary_format_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Read the compressed body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					ec <- fmt.Errorf("failed to read body: %w", err)
					http.Error(w, "failed to read body", http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))

				// Decompress with Snappy
				decoded, err := snappy.Decode(nil, body)
				if err != nil {
					ec <- fmt.Errorf("failed to decompress: %w", err)
					http.Error(w, "decompression failed", http.StatusBadRequest)
					return
				}

				// Binary format should NOT contain text like "timeseries:" or "samples:"
				// which would indicate text protobuf format
				textIndicators := []string{"timeseries:", "samples:", "labels:", "metadata:"}
				for _, indicator := range textIndicators {
					if bytes.Contains(decoded, []byte(indicator)) {
						ec <- fmt.Errorf("detected text protobuf format (found '%s'), MUST use binary format", indicator)
						break
					}
				}

				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "binary format validation errors: %v", errors)
		},
	}
}

// ProtobufDeserializationTest verifies that the message deserializes correctly (MUST requirement).
func ProtobufDeserializationTest() Test {
	return Test{
		Name: "ProtobufDeserialization",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "deserialize_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			// The default handler already deserializes the protobuf message.
			// If deserialization fails, the test will fail at the collector level.
			return next
		},
		Expected: func(t *testing.T, bs []Batch) {
			// If we receive batches, deserialization succeeded
			require.NotEmpty(t, bs, "deserialization failed: no batches received")
		},
	}
}

// =============================================================================
// 2. Snappy Compression Tests (3 tests)
// =============================================================================

// SnappyCompressionTest verifies that data is compressed with Snappy (MUST requirement).
// This extends the existing header test in headers.go with actual compression validation.
func SnappyCompressionTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "SnappyCompression",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "snappy_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify Content-Encoding header
				if enc := r.Header.Get("Content-Encoding"); enc != "snappy" {
					ec <- fmt.Errorf("Content-Encoding must be 'snappy', got '%s'", enc)
				}

				// Verify data is actually compressed
				body, err := io.ReadAll(r.Body)
				if err != nil {
					ec <- fmt.Errorf("failed to read body: %w", err)
					http.Error(w, "failed to read body", http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))

				// Try to decompress - this will fail if not Snappy compressed
				_, err = snappy.Decode(nil, body)
				if err != nil {
					ec <- fmt.Errorf("data is not Snappy compressed: %w", err)
				}

				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "snappy compression validation errors: %v", errors)
		},
	}
}

// SnappyBlockFormatTest verifies BLOCK format is used, NOT framed format (MUST requirement).
func SnappyBlockFormatTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "SnappyBlockFormat",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "block_format_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					ec <- fmt.Errorf("failed to read body: %w", err)
					http.Error(w, "failed to read body", http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))

				// Check for Snappy framed format magic bytes
				// Framed format starts with: 0xff 0x06 0x00 0x00 0x73 0x4e 0x61 0x50 0x70 0x59
				framedMagic := []byte{0xff, 0x06, 0x00, 0x00, 0x73, 0x4e, 0x61, 0x50, 0x70, 0x59}
				if len(body) >= len(framedMagic) && bytes.Equal(body[:len(framedMagic)], framedMagic) {
					ec <- fmt.Errorf("MUST NOT use Snappy framed format, MUST use block format")
				}

				// Verify we can decode with block format
				_, err = snappy.Decode(nil, body)
				if err != nil {
					ec <- fmt.Errorf("failed to decode with Snappy block format: %w", err)
				}

				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "snappy block format validation errors: %v", errors)
		},
	}
}

// SnappyDecompressionTest verifies data decompresses correctly (MUST requirement).
func SnappyDecompressionTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "SnappyDecompression",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "decompress_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					ec <- fmt.Errorf("failed to read body: %w", err)
					http.Error(w, "failed to read body", http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))

				// Decompress and verify we get non-empty data
				decoded, err := snappy.Decode(nil, body)
				if err != nil {
					ec <- fmt.Errorf("decompression failed: %w", err)
				} else if len(decoded) == 0 {
					ec <- fmt.Errorf("decompression produced empty data")
				}

				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "decompression validation errors: %v", errors)
		},
	}
}

// =============================================================================
// 3. HTTP Method & Request Tests (3 tests)
// =============================================================================

// HTTPPostMethodTest verifies HTTP POST method is used (MUST requirement).
func HTTPPostMethodTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "HTTPPostMethod",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "http_post_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					ec <- fmt.Errorf("MUST use POST method, got: %s", r.Method)
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "HTTP method validation errors: %v", errors)
		},
	}
}

// HTTPBodyContainsDataTest verifies body contains serialized+compressed protobuf (MUST requirement).
func HTTPBodyContainsDataTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "HTTPBodyContainsData",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "http_body_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					ec <- fmt.Errorf("failed to read body: %w", err)
					http.Error(w, "failed to read body", http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))

				if len(body) == 0 {
					ec <- fmt.Errorf("body must not be empty")
				}

				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "HTTP body validation errors: %v", errors)
		},
	}
}

// =============================================================================
// 4. Content-Type Header Tests (6 tests)
// =============================================================================

// ContentTypeHeaderPresentTest verifies Content-Type header is present (MUST requirement).
func ContentTypeHeaderPresentTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "ContentTypeHeaderPresent",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "content_type_present_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ct := r.Header.Get("Content-Type")
				if ct == "" {
					ec <- fmt.Errorf("Content-Type header MUST be present")
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "Content-Type header validation errors: %v", errors)
		},
	}
}

// ContentTypeBaseValueTest verifies base value is "application/x-protobuf" (MUST requirement).
func ContentTypeBaseValueTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "ContentTypeBaseValue",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "content_type_base_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ct := r.Header.Get("Content-Type")
				if !strings.HasPrefix(ct, "application/x-protobuf") {
					ec <- fmt.Errorf("Content-Type MUST start with 'application/x-protobuf', got: %s", ct)
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "Content-Type base value validation errors: %v", errors)
		},
	}
}

// ContentTypeRW2ProtoParamTest verifies RW 2.0 with proto parameter (SHOULD requirement).
func ContentTypeRW2ProtoParamTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "ContentTypeRW2ProtoParam",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "content_type_rw2_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ct := r.Header.Get("Content-Type")
				// For RW 2.0, SHOULD include proto parameter
				validFormats := []string{
					"application/x-protobuf;proto=io.prometheus.write.v2.Request",
					"application/x-protobuf; proto=io.prometheus.write.v2.Request", // with space
					"application/x-protobuf", // plain format for backward compat
				}

				valid := false
				for _, format := range validFormats {
					if strings.EqualFold(ct, format) {
						valid = true
						break
					}
				}

				if !valid {
					ec <- fmt.Errorf("Content-Type SHOULD be one of %v for RW 2.0, got: %s", validFormats, ct)
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			// SHOULD requirement - log warnings but don't fail
			if len(errors) > 0 {
				for _, err := range errors {
					fmt.Printf("WARNING: %v\n", err)
				}
			}
		},
	}
}

// ContentTypeRFC9110FormatTest verifies RFC 9110 format compliance (MUST requirement).
func ContentTypeRFC9110FormatTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "ContentTypeRFC9110Format",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "content_type_rfc_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ct := r.Header.Get("Content-Type")
				// RFC 9110: type/subtype with optional parameters
				// Example: application/x-protobuf;proto=io.prometheus.write.v2.Request
				rfc9110Pattern := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9!#$&^_.-]*/[a-zA-Z0-9][a-zA-Z0-9!#$&^_.-]*(?:\s*;\s*[a-zA-Z0-9_-]+=.*)?$`)
				if !rfc9110Pattern.MatchString(ct) {
					ec <- fmt.Errorf("Content-Type MUST follow RFC 9110 format, got: %s", ct)
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "RFC 9110 format validation errors: %v", errors)
		},
	}
}

// =============================================================================
// 5. X-Prometheus-Remote-Write-Version Header Tests (4 tests)
// =============================================================================

// VersionHeaderPresentTest verifies X-Prometheus-Remote-Write-Version header is present (MUST requirement).
func VersionHeaderPresentTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "VersionHeaderPresent",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "version_present_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				version := r.Header.Get("X-Prometheus-Remote-Write-Version")
				if version == "" {
					ec <- fmt.Errorf("X-Prometheus-Remote-Write-Version header MUST be present")
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "version header validation errors: %v", errors)
		},
	}
}

// VersionHeaderRW2ValueTest verifies value is "2.0.0" for RW 2.0 (SHOULD requirement).
func VersionHeaderRW2ValueTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "VersionHeaderRW2Value",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "version_rw2_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				version := r.Header.Get("X-Prometheus-Remote-Write-Version")
				ct := r.Header.Get("Content-Type")

				// If Content-Type indicates RW 2.0, version SHOULD be 2.0.0
				if strings.Contains(ct, "io.prometheus.write.v2.Request") && version != "2.0.0" {
					ec <- fmt.Errorf("version SHOULD be '2.0.0' for RW 2.0 receivers, got: %s", version)
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			// SHOULD requirement - log warnings but don't fail
			if len(errors) > 0 {
				for _, err := range errors {
					fmt.Printf("WARNING: %v\n", err)
				}
			}
		},
	}
}

// =============================================================================
// 6. User-Agent Header Tests (3 tests)
// =============================================================================

// UserAgentHeaderPresentTest verifies User-Agent header is present (MUST requirement).
func UserAgentHeaderPresentTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "UserAgentHeaderPresent",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "user_agent_present_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ua := r.Header.Get("User-Agent")
				if ua == "" {
					ec <- fmt.Errorf("User-Agent header MUST be present")
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			require.Empty(t, errors, "User-Agent validation errors: %v", errors)
		},
	}
}

// UserAgentRFC9110FormatTest verifies RFC 9110 format (SHOULD requirement).
func UserAgentRFC9110FormatTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "UserAgentRFC9110Format",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "user_agent_rfc_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ua := r.Header.Get("User-Agent")
				// RFC 9110 format: <product>/<version> [<comment>]
				// Example: Prometheus/2.45.0 or GrafanaAgent/0.19.0
				rfc9110Pattern := regexp.MustCompile(`^[A-Za-z0-9_-]+/[\d.]+.*`)
				if ua != "" && !rfc9110Pattern.MatchString(ua) {
					ec <- fmt.Errorf("User-Agent SHOULD follow RFC 9110 format (<product>/<version>), got: %s", ua)
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			// SHOULD requirement - log warnings but don't fail
			if len(errors) > 0 {
				for _, err := range errors {
					fmt.Printf("WARNING: %v\n", err)
				}
			}
		},
	}
}

// UserAgentIdentifiesSenderTest verifies sender identification (SHOULD requirement).
func UserAgentIdentifiesSenderTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "UserAgentIdentifiesSender",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "user_agent_id_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ua := r.Header.Get("User-Agent")
				// Should contain sender name and version
				if ua != "" {
					hasName := regexp.MustCompile(`[A-Za-z]+`).MatchString(ua)
					hasVersion := regexp.MustCompile(`[\d.]+`).MatchString(ua)
					if !hasName || !hasVersion {
						ec <- fmt.Errorf("User-Agent SHOULD identify sender name and version, got: %s", ua)
					}
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			// SHOULD requirement - log warnings but don't fail
			if len(errors) > 0 {
				for _, err := range errors {
					fmt.Printf("WARNING: %v\n", err)
				}
			}
		},
	}
}
