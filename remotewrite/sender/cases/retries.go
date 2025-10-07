package cases

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// Retries500Test rejects the first remote write and checks it gets resent.
func Retries500Test() Test {
	var (
		mtx    sync.Mutex
		accept bool
		ts     int64
	)

	return Test{
		Name: "Retries500",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "now",
		}, func() float64 {
			return float64(time.Now().Unix() * 1000)
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mtx.Lock()
				defer mtx.Unlock()

				if accept {
					next.ServeHTTP(w, r)
					return
				}

				// We're going to pick a timestamp from this batch, and then make sure
				// it gets resent.  First we need to decode this batch.
				ts = getFirstTimestamp(w, r)
				accept = true
				http.Error(w, "internal server error", http.StatusInternalServerError)
			})

		},
		Expected: func(t *testing.T, bs []Batch) {
			found := false
			forAllSamples(bs, func(s sample) {
				if labelsContain(s.l, labels.FromStrings("__name__", "now")) && s.t == ts {
					found = true
				}
			})
			require.True(t, found, `failed to find sample that should have been retried`)
		},
	}
}

// Retries400Test rejects the first remote write and checks it doesn't get resent.
func Retries400Test() Test {
	var (
		mtx    sync.Mutex
		accept bool
		ts     int64
	)

	return Test{
		Name: "Retries400",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "now",
		}, func() float64 {
			return float64(time.Now().Unix() * 1000)
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mtx.Lock()
				defer mtx.Unlock()

				if accept {
					next.ServeHTTP(w, r)
					return
				}

				// We're going to pick a timestamp from this batch, and then make sure
				// it gets resent.  First we need to decode this batch.
				ts = getFirstTimestamp(w, r)
				accept = true
				http.Error(w, "bad request", http.StatusBadRequest)
			})

		},
		Expected: func(t *testing.T, bs []Batch) {
			found := false
			forAllSamples(bs, func(s sample) {
				if labelsContain(s.l, labels.FromStrings("__name__", "now")) && s.t == ts {
					found = true
				}
			})
			require.False(t, found, `found sample that should not have been retried`)
		},
	}
}

func getFirstTimestamp(w http.ResponseWriter, r *http.Request) int64 {
	collector := SampleCollector{}
	h := remote.NewWriteHandler(&collector, remote.MessageTypes{remote.WriteV1MessageType})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code/100 != 2 {
		http.Error(w, "", rec.Code)
		return -1
	}

	// Find a sample for "now{}" and record its timestamp.
	var ts int64 = -1
	forAllSamples(collector.Batches, func(s sample) {
		if labelsContain(s.l, labels.FromStrings("__name__", "now")) {
			ts = s.t
		}
	})
	return ts
}
