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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// Protocol Tests for Remote Write 2.0 Specification
// Ref: https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/

var ProtocolTests = []func() Test{
	StrictRW2ReceiverTest,

	ProtobufV2FormatTest,
	ProtobufBinaryFormatTest,
	ProtobufDeserializationTest,

	SnappyTest,

	HTTPRequestTest,

	ContentTypeTest,
	VersionHeaderTest,
	UserAgentTest,
}

func StrictRW2ReceiverTest() Test {
	return Test{
		Name: "StrictRW2Receiver",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "strict_rw2_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Expected: func(t *testing.T, bs []Batch) {
			if len(bs) == 0 {
				t.Logf("FAILURE: Sender does not support RW 2.0 format")
				t.Logf("Ensure Prometheus config includes:")
				t.Logf("  protobuf_message: \"io.prometheus.write.v2.Request\"")
			}
			require.NotEmpty(t, bs, "RW 2.0-only receiver should accept data from compliant senders")
		},
	}
}

// ProtobufV2FormatTest verifies that the sender uses io.prometheus.write.v2.Request
// for RW 2.0 receivers (SHOULD requirement for backward compatibility).
func ProtobufV2FormatTest() Test {
	return newProtocolTestWithLevel("ProtobufV2Format", false, // SHOULD
		func(t *testing.T, r *http.Request) error {
			ct := r.Header.Get("Content-Type")
			if strings.Contains(ct, "io.prometheus.write.v2.Request") {
				return nil
			}
			if strings.Contains(ct, "prometheus.WriteRequest") || ct == "application/x-protobuf" {
				return fmt.Errorf("sender used RW 1.0 format (Content-Type: %s), SHOULD use RW 2.0", ct)
			}
			return fmt.Errorf("unexpected Content-Type: %s", ct)
		})
}

// ProtobufBinaryFormatTest verifies that Binary Wire Format is used (MUST requirement).
func ProtobufBinaryFormatTest() Test {
	return newProtocolTest("ProtobufBinaryFormat",
		func(t *testing.T, r *http.Request) error {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return fmt.Errorf("failed to read body: %w", err)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			// Decompress with Snappy
			decoded, err := snappy.Decode(nil, body)
			if err != nil {
				return fmt.Errorf("failed to decompress: %w", err)
			}

			// Binary format should NOT contain text markers
			textIndicators := []string{"timeseries:", "samples:", "labels:", "metadata:"}
			for _, indicator := range textIndicators {
				if bytes.Contains(decoded, []byte(indicator)) {
					return fmt.Errorf("detected text protobuf format (found '%s'), must use binary format", indicator)
				}
			}
			return nil
		})
}

// ProtobufDeserializationTest verifies that the message deserializes correctly (MUST requirement).
func ProtobufDeserializationTest() Test {
	// if batches are received, deserialization succeeded.
	return Test{
		Name: "ProtobufDeserialization",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "deserialize_test",
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "deserialization failed: no batches received")
		},
	}
}

// SnappyTest verifies all Snappy compression requirements.
func SnappyTest() Test {
	return newProtocolTest("Snappy",
		// MUST: Content-Encoding header is "snappy"
		func(t *testing.T, r *http.Request) error {
			if r.Header.Get("Content-Encoding") != "snappy" {
				return fmt.Errorf("header 'Content-Encoding' != 'snappy'; got '%s'", r.Header.Get("Content-Encoding"))
			}
			return nil
		},
		// MUST: Data is Snappy compressed and uses block format (not framed)
		func(t *testing.T, r *http.Request) error {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return fmt.Errorf("failed to read body: %w", err)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			// Check for Snappy framed format magic bytes
			framedMagic := []byte{0xff, 0x06, 0x00, 0x00, 0x73, 0x4e, 0x61, 0x50, 0x70, 0x59}
			if len(body) >= len(framedMagic) && bytes.Equal(body[:len(framedMagic)], framedMagic) {
				return fmt.Errorf("must NOT use Snappy framed format, must use block format")
			}

			// Verify we can decode with block format
			decoded, err := snappy.Decode(nil, body)
			if err != nil {
				return fmt.Errorf("failed to decode with Snappy block format: %w", err)
			}

			// MUST: Decompression produces non-empty data
			if len(decoded) == 0 {
				return fmt.Errorf("decompression produced empty data")
			}
			return nil
		})
}

// HTTPRequestTest verifies HTTP request requirements.
func HTTPRequestTest() Test {
	return newProtocolTest("HTTPRequest",
		// MUST: POST method is used
		func(t *testing.T, r *http.Request) error {
			if r.Method != http.MethodPost {
				return fmt.Errorf("HTTP method must be %s, got: %s", http.MethodPost, r.Method)
			}
			return nil
		},
		// MUST: Body contains data
		func(t *testing.T, r *http.Request) error {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return fmt.Errorf("failed to read body: %w", err)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			if len(body) == 0 {
				return fmt.Errorf("body must not be empty")
			}
			return nil
		})
}

// ContentTypeTest verifies all Content-Type header requirements.
func ContentTypeTest() Test {
	// Example: "application/x-protobuf;proto=io.prometheus.write.v2.Request"
	rfc9110Pattern := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9!#$&^_.-]*/[a-zA-Z0-9][a-zA-Z0-9!#$&^_.-]*(?:\s*;\s*[a-zA-Z0-9_-]+=.*)?$`)
	return newProtocolTestWithMixed("ContentType", []validator{
		// MUST: Header must be present
		{mustRequirement: true, validate: validateHeaderPresent("Content-Type")},
		// MUST: Base value must be "application/x-protobuf"
		{mustRequirement: true, validate: func(t *testing.T, r *http.Request) error {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "application/x-protobuf") {
				return fmt.Errorf("Content-Type must start with 'application/x-protobuf', got: %s", ct)
			}
			return nil
		}},
		// MUST: RFC 9110 format compliance
		{mustRequirement: true, validate: validateHeaderMatches("Content-Type", rfc9110Pattern)},
		// SHOULD: RW 2.0 proto parameter
		{mustRequirement: false, validate: func(t *testing.T, r *http.Request) error {
			ct := r.Header.Get("Content-Type")
			validFormats := []string{
				"application/x-protobuf;proto=io.prometheus.write.v2.Request",
				"application/x-protobuf; proto=io.prometheus.write.v2.Request",
				"application/x-protobuf",
			}
			for _, format := range validFormats {
				if strings.EqualFold(ct, format) {
					return nil
				}
			}
			return fmt.Errorf("Content-Type SHOULD be one of %v, got: %s", validFormats, ct)
		}},
	})
}

// VersionHeaderTest verifies X-Prometheus-Remote-Write-Version header requirements.
func VersionHeaderTest() Test {
	return newProtocolTestWithMixed("VersionHeader", []validator{
		// MUST: Header must be present
		{mustRequirement: true, validate: validateHeaderPresent("X-Prometheus-Remote-Write-Version")},
		// SHOULD: Value is "2.0.0" for RW 2.0
		{mustRequirement: false, validate: func(t *testing.T, r *http.Request) error {
			version := r.Header.Get("X-Prometheus-Remote-Write-Version")
			ct := r.Header.Get("Content-Type")
			if strings.Contains(ct, "io.prometheus.write.v2.Request") && version != "2.0.0" {
				return fmt.Errorf("version SHOULD be '2.0.0' for RW 2.0, got: %s", version)
			}
			return nil
		}},
	})
}

// UserAgentTest verifies User-Agent header requirements.
func UserAgentTest() Test {
	// Example: "Prometheus/3.7.1" or "Prometheus/3.7.1 (linux; amd64)"
	rfc9110Pattern := regexp.MustCompile(`^[A-Za-z0-9_-]+/[\d.]+.*`)
	return newProtocolTestWithMixed("UserAgent", []validator{
		// MUST: Header must be present
		{mustRequirement: true, validate: validateHeaderPresent("User-Agent")},
		// SHOULD: RFC 9110 format
		{mustRequirement: false, validate: validateHeaderMatches("User-Agent", rfc9110Pattern)},
		// SHOULD: Identifies sender name and version
		{mustRequirement: false, validate: func(t *testing.T, r *http.Request) error {
			ua := r.Header.Get("User-Agent")
			if ua == "" {
				return nil // Already tested above
			}
			hasName := regexp.MustCompile(`[A-Za-z]+`).MatchString(ua)
			hasVersion := regexp.MustCompile(`[\d.]+`).MatchString(ua)
			if !hasName || !hasVersion {
				return fmt.Errorf("User-Agent SHOULD identify sender name and version, got: %s", ua)
			}
			return nil
		}},
	})
}

// validator pairs a validation function with its requirement level
type validator struct {
	mustRequirement bool // true = MUST (fail), false = SHOULD (warn)
	validate        func(*testing.T, *http.Request) error
}

// newProtocolTestWithMixed creates a test with mixed MUST/SHOULD requirements
func newProtocolTestWithMixed(name string, validators []validator) Test {
	var mustErrors []error
	var shouldErrors []error

	return Test{
		Name: name,
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: strings.ToLower(strings.ReplaceAll(name, " ", "_")),
		}, func() float64 {
			return float64(time.Now().Unix())
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, v := range validators {
					if err := v.validate(nil, r); err != nil {
						if v.mustRequirement {
							mustErrors = append(mustErrors, err)
						} else {
							shouldErrors = append(shouldErrors, err)
						}
					}
				}
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			require.NotEmpty(t, bs, "expected at least one batch")
			// MUST requirements - fail test
			if len(mustErrors) > 0 {
				require.Empty(t, mustErrors, "%s MUST validation errors: %v", name, mustErrors)
			}
			// SHOULD requirements - log warnings
			for _, err := range shouldErrors {
				t.Logf("WARNING: %v", err)
			}
		},
	}
}

// newProtocolTestWithLevel creates a protocol test with specific RFC level (all validators same level)
func newProtocolTestWithLevel(name string, mustRequirement bool, validators ...func(*testing.T, *http.Request) error) Test {
	// Convert plain validators to structured validators with same requirement level
	structuredValidators := make([]validator, len(validators))
	for i, v := range validators {
		structuredValidators[i] = validator{mustRequirement: mustRequirement, validate: v}
	}
	return newProtocolTestWithMixed(name, structuredValidators)
}

// newProtocolTest creates a standard protocol test (all MUST requirements)
func newProtocolTest(name string, validators ...func(*testing.T, *http.Request) error) Test {
	return newProtocolTestWithLevel(name, true, validators...)
}

func validateHeaderPresent(name string) func(*testing.T, *http.Request) error {
	return func(t *testing.T, r *http.Request) error {
		if r.Header.Get(name) == "" {
			return fmt.Errorf("header '%s' must be present", name)
		}
		return nil
	}
}

func validateHeaderMatches(name string, pattern *regexp.Regexp) func(*testing.T, *http.Request) error {
	return func(t *testing.T, r *http.Request) error {
		value := r.Header.Get(name)
		if value != "" && !pattern.MatchString(value) {
			return fmt.Errorf("header '%s' doesn't match pattern %s, got: %s", name, pattern, value)
		}
		return nil
	}
}
