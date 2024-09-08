package cases

import (
	"math/rand"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// OrderingTest exports a lot of metrics, and inserts random delays in the push handler
// to see if we can force agents to send us metrics in the wrong order.
func OrderingTest() Test {
	r := prometheus.NewPedanticRegistry()
	for i := 0; i < 1000; i++ {
		c := prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "test",
			ConstLabels: prometheus.Labels{"i": strconv.Itoa(i)},
		})
		c.Set(1.0)
		r.MustRegister(c)
	}

	return Test{
		Name:    "Ordering",
		Metrics: promhttp.HandlerFor(r, promhttp.HandlerOpts{}),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(time.Duration(rand.Int63n(int64(5 * time.Second))))
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			last := map[string]int64{}
			forAllSamples(bs, func(s sample) {
				l := s.l.String()
				require.Less(t, last[l], s.t)
				last[l] = s.t
			})
			tests := countMetricWithValue(t, bs, labels.FromStrings("__name__", "test"), 1.0)
			require.True(t, tests > 0, `found zero samples for tests{}`)
		},
	}
}
