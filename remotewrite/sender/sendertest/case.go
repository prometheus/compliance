package sendertest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	"github.com/stretchr/testify/require"

	writev1 "github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

type RFCLevel string

const (
	MustLevel        RFCLevel = "MUST"
	ShouldLevel      RFCLevel = "SHOULD"
	MayLevel         RFCLevel = "MAY"
	RecommendedLevel RFCLevel = "RECOMMENDED"
)

func (r RFCLevel) annotate(t *testing.T) {
	t.Attr("rfcLevel", string(r))
}

func descAnnotate(t *testing.T, desc string) {
	t.Attr("description", desc)
}

// ReceiverRequest represents a captured HTTP request from a sender.
type ReceiverRequest struct {
	Method  string
	Headers http.Header
	Body    []byte

	RW2 *writev2.Request
	RW1 *writev1.WriteRequest

	Received time.Time
	Err      error
}

// ReceiverResult is a test result after scrape -> sender -> receiver cycles.
type ReceiverResult struct {
	// RemoteWrite version that this test was configured with.
	Version remote.WriteMessageType
	// All sender requests received by the test receiver.
	Requests []ReceiverRequest
}

// RequestsProtoToString prints proto messages for debugging.
func (res ReceiverResult) RequestsProtoToString() string {
	var sb strings.Builder
	for i, r := range res.Requests {
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(": ")
		sb.WriteString(r.RW1.String())
		sb.WriteString(r.RW2.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

// ReceiverResponse configures the response behavior of the test receiver.
type ReceiverResponse struct {
	StatusCode        int
	Headers           map[string]string
	Body              string
	SamplesWritten    int
	ExemplarsWritten  int
	HistogramsWritten int
}

type ValidateFunc func(t *testing.T, res ReceiverResult)

// Case defines a test scenario to run against a single sender for a single test.
type Case struct {
	// Name is a unique name for a test case.
	// If non-empty, this adds "/<name>/" sub-test.
	// Can be left empty if tests has only one test case.
	Name string
	// Description is a description for a test case.
	// TODO(bwplotka): Fix index.html to actually render it.
	Description string
	// RFCLevel is the requirement level for a test to pass.
	// This includes all the errors in the '__setup__' stage, including Validate
	// ValidateCases can have different semantics.
	RFCLevel RFCLevel
	// RemoteWrite version to configure the target with. This indicates version of Remote Write protocol,
	// that the target must use.
	// This adds another "/rw<number>/" sub-test.
	Version remote.WriteMessageType
	// ScrapeData provides input scrape data to use against the tested target.
	ScrapeData string
	// TestResponses controls the receiver response to respond with to a sender, sequentially.
	// Tests finishes when there is nothing else to respond with, or timeout.
	// By default, the case responds with a single NoContent (success) response.
	TestResponses []ReceiverResponse
	// Validate allows validating requests that were sent after test.
	Validate ValidateFunc
	// ValidateCase configures sub-cases for the validation. This is useful
	// when the same input case can be shared across multiple tests/RFC levels.
	ValidateCases []ValidateCase
}

func Repeat(r ReceiverResponse, n int) []ReceiverResponse {
	ret := make([]ReceiverResponse, n)
	for i := range n {
		ret[i] = r
	}
	return ret
}

type ValidateCase struct {
	// Name is  unique name for a test case.
	// This adds "/<name>/" sub-test.
	Name string
	// Description is a description for a validate test case.
	// TODO(bwplotka): Fix index.html to actually render it.
	Description string
	// RFCLevel is the requirement level for a test to pass.
	// This includes all the errors in the tests, including Validate
	RFCLevel RFCLevel
	// Validate allows validating requests that were sent after test.
	Validate ValidateFunc
}

// Run runs a test scenario(s) for each configured target.
func Run(t *testing.T, testTargets map[string]targets.Target, tcs ...Case) {
	t.Helper()

	require.NotEmpty(t, testTargets)
	require.NotEmpty(t, tcs)

	for _, tc := range tcs {
		runTestForEachSender(t, tc.Name, testTargets, tc)
	}
}

func runTestForEachSender(t *testing.T, name string, testTargets map[string]targets.Target, tc Case) {
	t.Helper()

	protoMsgName := ""
	switch tc.Version {
	case remote.WriteV1MessageType:
		protoMsgName = "rw1"
	case remote.WriteV2MessageType:
		protoMsgName = "rw2"
	default:
		t.Fatalf("unsupported remote write message type: %v", tc.Version)
	}

	allLevels := []RFCLevel{tc.RFCLevel}
	for _, vc := range tc.ValidateCases {
		allLevels = append(allLevels, vc.RFCLevel)
	}

	for targetName, runTarget := range testTargets {
		subCaseName := fmt.Sprintf("%v/%v", protoMsgName, targetName)
		if name != "" {
			subCaseName = fmt.Sprintf("%v/%v", name, subCaseName)
		}
		t.Attr("version", protoMsgName)
		t.Run(subCaseName, func(t *testing.T) {
			t.Parallel()

			if len(tc.TestResponses) == 0 {
				tc.TestResponses = []ReceiverResponse{{}} // By default assume a single successful response.
			}
			receiver := NewSyncReceiver(tc.Version, tc.TestResponses)
			scrapeTarget := newScrapeTarget(tc.ScrapeData)

			// Setup subcase exists to cleanly visualise errors that happens before certain validation cases.
			// TODO(bwplotka): This is odd for tests with 0 cases and a single validation.
			// Consider reshaping a bit or not allowing 0 cases tests.
			// Notably for non-0 cases you have extra MUST calculated etc.
			setupOk := safeRun(t, "_setup", targetName, tc.Description, tc.RFCLevel, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)

				cancelGroup := func(err error) { cancel() }

				var g run.Group
				g.Add(func() error {
					return receiver.Run(ctx)
				}, cancelGroup)
				g.Add(func() error {
					scrapeTarget.Run(ctx)
					return nil
				}, cancelGroup)
				g.Add(func() error {
					return runTarget(ctx, targets.TargetOptions{
						ScrapeTargetJobName:    "test",
						ScrapeTargetHostPort:   scrapeTarget.HostPort(t),
						RemoteWriteEndpointURL: receiver.URL(),
						RemoteWriteMessage:     tc.Version,
					})
				}, cancelGroup)
				if err := g.Run(); err != nil {
					t.Errorf("premature scrape endpoint, receiver or target stop; consider re-runing with DEBUG='1'")
				}

				res := receiver.Result()
				for i, r := range res.Requests {
					if r.Err != nil {
						t.Errorf("sender request %v failed", i)
					}
				}
				if tc.Validate != nil {
					tc.Validate(t, res)
				}
			})

			// Run sub-cases even if setup failed so the overview is clear.
			res := receiver.Result()
			for _, vc := range tc.ValidateCases {
				safeRun(t, vc.Name, targetName, vc.Description, vc.RFCLevel, func(t *testing.T) {
					if !setupOk {
						t.Fatal("setup failed")
					}

					vc.Validate(t, res)
				})
			}
		})
	}
}

func safeRun(t *testing.T, name, target, desc string, rfc RFCLevel, f func(t *testing.T)) bool {
	return t.Run(name, func(t *testing.T) {
		t.Attr("rw", target)
		rfc.annotate(t)
		descAnnotate(t, desc)

		if os.Getenv("DEBUG") == "" {
			// Only capture panics on non-debug. When debugging, it's useful to know the exact trace.
			defer func() {
				if val := recover(); val != nil {
					t.Fatal("test panicked:", val)
				}
			}()
		}
		f(t)
	})
}

type Receiver struct {
	mu      sync.Mutex
	closeCh chan struct{}
	closed  bool

	server *httptest.Server

	msg       remote.WriteMessageType
	requests  []ReceiverRequest
	responses []ReceiverResponse

	debug bool
}

// NewSyncReceiver returns a new mock HTTP receiver for sender testing. This receiver
// captures requests and respond with the given responses sequentially until they finish or context is done.
//
// NOTE: This kind of receiver will only work for non-sharded, single sender.
func NewSyncReceiver(msg remote.WriteMessageType, responses []ReceiverResponse) *Receiver {
	r := &Receiver{
		closeCh:   make(chan struct{}, 1),
		requests:  make([]ReceiverRequest, 0, len(responses)),
		responses: responses,
		msg:       msg,

		debug: os.Getenv("DEBUG") != "",
	}

	r.server = httptest.NewServer(http.HandlerFunc(r.handleSyncRequest))
	return r
}

// Run runs test receiver until it finishes or context is done.
//
// It returns nil error when received the len(responses) requests.
//
// NOTE: This kind of receiver will only work for non-sharded, single sender.
func (r *Receiver) Run(ctx context.Context) error {
	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-r.closeCh:
	}

	r.mu.Lock()
	close(r.closeCh)
	r.closed = true
	r.mu.Unlock()

	r.server.Close()
	return err
}

// TODO(bwplotka): Pass proper logging that uses t.Log.
func (r *Receiver) debugLogf(msg string, args ...any) {
	if !r.debug {
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, msg+"\n", args...)
}

// handleSyncRequest handles incoming, sequential, remote write requests.
func (r *Receiver) handleSyncRequest(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		r.debugLogf("sendertest.Receiver: got request on closed receiver")
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
		http.Error(w, "Test finished", http.StatusGone)
		return
	}

	var (
		err     error
		headers = req.Header.Clone()
		got     = ReceiverRequest{
			Method:   req.Method,
			Headers:  headers,
			Received: time.Now(),
		}
		decoded []byte
	)

	// Add requests even on errors.
	defer func() {
		r.requests = append(r.requests, got)
		r.debugLogf("sendertest.Receiver: got %#v request; got/expected %v/%v", got, len(r.requests), len(r.responses))
	}()

	got.Body, err = io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		got.Err = err
		return
	}
	defer req.Body.Close()

	contentEncoding := req.Header.Get("Content-Encoding")
	if contentEncoding == "snappy" {
		decoded, err = snappy.Decode(nil, got.Body)
		if err != nil {
			http.Error(w, "Failed to decode snappy", http.StatusBadRequest)
			got.Err = err
			return
		}
	} else {
		decoded = got.Body
	}

	switch r.msg {
	case remote.WriteV1MessageType:
		var rw1 writev1.WriteRequest
		if err := rw1.Unmarshal(decoded); err != nil {
			http.Error(w, "Failed to unmarshal protobuf", http.StatusBadRequest)
			got.Err = err
			return
		}
		got.RW1 = &rw1
	case remote.WriteV2MessageType:
		var rw2 writev2.Request
		if err := rw2.Unmarshal(decoded); err != nil {
			http.Error(w, "Failed to unmarshal protobuf", http.StatusBadRequest)
			got.Err = err
			return
		}
		got.RW2 = &rw2
	}

	if len(r.requests)+1 >= len(r.responses) {
		// Achieved the goal.
		r.closeCh <- struct{}{}
		r.closed = true
		r.debugLogf("sendertest.Receiver: got all expected %v requests; closing...", len(r.requests)+1)
	}

	// Fetch the response to send, based on the test case.
	resp := r.responses[len(r.requests)]

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if resp.StatusCode == 0 {
		resp.StatusCode = http.StatusNoContent
	}

	if r.msg != remote.WriteV1MessageType {
		// Set X-Prometheus-Remote-Write-*-Written headers if response is successful.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// If response counts are explicitly configured, use those. Otherwise, automatically count what we actually received.
			samplesWritten := resp.SamplesWritten
			exemplarsWritten := resp.ExemplarsWritten
			histogramsWritten := resp.HistogramsWritten

			// Auto-count if not explicitly set.
			if samplesWritten == 0 && exemplarsWritten == 0 && histogramsWritten == 0 {
				if got.RW2 != nil {
					for _, ts := range got.RW2.Timeseries {
						samplesWritten += len(ts.Samples)
						exemplarsWritten += len(ts.Exemplars)
						histogramsWritten += len(ts.Histograms)
					}
				}
				if got.RW1 != nil {
					for _, ts := range got.RW1.Timeseries {
						samplesWritten += len(ts.Samples)
						exemplarsWritten += len(ts.Exemplars)
						histogramsWritten += len(ts.Histograms)
					}
				}

			}

			w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", fmt.Sprintf("%d", samplesWritten))
			w.Header().Set("X-Prometheus-Remote-Write-Exemplars-Written", fmt.Sprintf("%d", exemplarsWritten))
			w.Header().Set("X-Prometheus-Remote-Write-Histograms-Written", fmt.Sprintf("%d", histogramsWritten))
		}
	}

	w.WriteHeader(resp.StatusCode)
	if resp.Body != "" {
		w.Write([]byte(resp.Body))
	}
}

// URL returns the URL of the mock receiver.
func (r *Receiver) URL() string {
	return r.server.URL
}

// Result returns the data gathered by the receiver.
func (r *Receiver) Result() ReceiverResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	return ReceiverResult{
		Requests: append([]ReceiverRequest{}, r.requests...),
	}
}

// scrapeTarget is an HTTP server that serves metrics in Prometheus format.
type scrapeTarget struct {
	server  *httptest.Server
	mu      sync.Mutex
	metrics string
}

func newScrapeTarget(metrics string) *scrapeTarget {
	st := &scrapeTarget{
		metrics: metrics,
	}
	st.server = httptest.NewServer(http.HandlerFunc(st.handleScrape))
	return st
}

// Run runs scrapeTarget until context is done.
func (st *scrapeTarget) Run(ctx context.Context) {
	<-ctx.Done()
	st.server.Close()
	return
}

const om1ContentType = "application/openmetrics-text; version=1.0.0; charset=utf-8"

// handleScrape serves metrics in OpenMetrics 1 exposition format.
func (st *scrapeTarget) handleScrape(w http.ResponseWriter, r *http.Request) {
	st.mu.Lock()
	defer st.mu.Unlock()

	metrics := st.metrics
	if !strings.HasSuffix(metrics, "# EOF\n") {
		// Annoying, add it if missing.
		metrics = metrics + "# EOF\n"
	}
	w.Header().Set("Content-Type", om1ContentType)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(metrics))
	return
}

// HostPort returns the host:port of the mock scrape target.
func (st *scrapeTarget) HostPort(t *testing.T) string {
	u, err := url.Parse(st.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host // host:port
}

// UpdateMetrics updates the metrics served by the scrape target.
func (st *scrapeTarget) UpdateMetrics(metrics string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.metrics = metrics
}
