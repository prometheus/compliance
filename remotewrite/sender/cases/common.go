package cases

import (
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
)

type Test struct {
	Name     string
	Metrics  http.Handler
	Expected Validator

	// Optional "middleware" to intercept the write requests.
	Writes func(http.Handler) http.Handler

	// ReceiverVersion specifies which Remote Write version(s) the receiver supports.
	// If nil, defaults to accepting both RW 1.0 and RW 2.0 for backward compatibility.
	// Use this to test strict version compliance:
	//   - []remote.WriteMessageType{remote.WriteV2MessageType} for RW 2.0-only receiver
	//   - []remote.WriteMessageType{remote.WriteV1MessageType} for RW 1.0-only receiver
	ReceiverVersion []remote.WriteMessageType
}

func metricHandler(c prometheus.Collector) http.Handler {
	r := prometheus.NewPedanticRegistry()
	r.MustRegister(c)
	return promhttp.HandlerFor(r, promhttp.HandlerOpts{})
}

func staticHandler(contents []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(contents); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

type Validator func(t *testing.T, bs []Batch)

type SampleCollector struct {
	sync.Mutex
	Batches []Batch
}

type Batch struct {
	collector *SampleCollector
	samples   []sample
}

type sample struct {
	l labels.Labels
	t int64
	v float64
}

// addBatch adds a batch of samples to the collector.
func (c *SampleCollector) addBatch(samples []sample) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	b := Batch{
		collector: c,
		samples:   samples,
	}
	c.Batches = append(c.Batches, b)
}

// Store implements the writeStorage interface from client_golang/exp/api/remote.
func (c *SampleCollector) Store(req *http.Request, _ remote.WriteMessageType) (*remote.WriteResponse, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return remote.NewWriteResponse(), err
	}

	var writeReq prompb.WriteRequest
	if err := writeReq.Unmarshal(body); err != nil {
		return remote.NewWriteResponse(), err
	}

	samples := make([]sample, 0)

	for _, ts := range writeReq.Timeseries {
		labelPairs := make([]labels.Label, len(ts.Labels))
		for i, l := range ts.Labels {
			labelPairs[i] = labels.Label{Name: l.Name, Value: l.Value}
		}
		lb := labels.New(labelPairs...)

		for _, s := range ts.Samples {
			samples = append(samples, sample{
				l: lb,
				t: s.Timestamp,
				v: s.Value,
			})
		}
	}

	c.addBatch(samples)

	return remote.NewWriteResponse(), nil
}
