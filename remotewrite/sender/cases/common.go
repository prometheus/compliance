package cases

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

type Test struct {
	Name     string
	Metrics  http.Handler
	Expected Validator

	// Optional "middleware" to intercept the write requests.
	Writes func(http.Handler) http.Handler
}

func metricHandler(c prometheus.Collector) http.Handler {
	r := prometheus.NewPedanticRegistry()
	r.MustRegister(c)
	return promhttp.HandlerFor(r, promhttp.HandlerOpts{})
}

func StaticHandler(contents []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(contents); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

type Validator func(t *testing.T, bs []Batch)

type Appendable struct {
	sync.Mutex
	Batches []Batch
}

type Batch struct {
	appender *Appendable
	samples  []sample
}

type sample struct {
	l labels.Labels
	t int64
	v float64

	exemplar       *exemplar.Exemplar
	histogram      *histogram.Histogram
	floatHistogram *histogram.FloatHistogram
	metadata       *metadata.Metadata
	ctZeroSample   *ctZeroSample
}

type ctZeroSample struct {
	t  int64
	ct int64
}

func (m *Appendable) Appender(_ context.Context) storage.Appender {
	b := &Batch{
		appender: m,
	}
	return b
}

func (m *Batch) Append(_ storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	m.samples = append(m.samples, sample{l: l, t: t, v: v})
	return 0, nil
}

func (m *Batch) Commit() error {
	m.appender.Mutex.Lock()
	defer m.appender.Mutex.Unlock()
	m.appender.Batches = append(m.appender.Batches, *m)
	return nil
}

func (*Batch) Rollback() error {
	return nil
}

func (m *Batch) AppendExemplar(_ storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	m.samples = append(m.samples, sample{l: l, exemplar: &e})
	return 0, nil
}

func (m *Batch) AppendHistogram(_ storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	m.samples = append(m.samples, sample{l: l, t: t, histogram: h, floatHistogram: fh})
	return 0, nil
}

func (m *Batch) UpdateMetadata(_ storage.SeriesRef, l labels.Labels, md metadata.Metadata) (storage.SeriesRef, error) {
	m.samples = append(m.samples, sample{l: l, metadata: &md})
	return 0, nil
}

func (m *Batch) AppendCTZeroSample(_ storage.SeriesRef, l labels.Labels, t, ct int64) (storage.SeriesRef, error) {
	m.samples = append(m.samples, sample{l: l, ctZeroSample: &ctZeroSample{t: t, ct: ct}})
	return 0, nil
}
