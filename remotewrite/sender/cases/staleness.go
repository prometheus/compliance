package cases

import (
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/stretchr/testify/require"
)

// StalenessTest exposes a single metric that is then removed before the next scrape
// and checks that the sender propagates a staleness marker.
func StalenessTest() Test {
	var (
		scraped = false
		gauge   = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "stale",
		})
		reg     = prometheus.NewPedanticRegistry()
		handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	)

	reg.MustRegister(gauge)

	return Test{
		Name: "Staleness",
		Metrics: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(w, r)

			if !scraped {
				reg.Unregister(gauge)
				scraped = true
			}
		}),
		Expected: func(t *testing.T, bs []Batch) {
			stalenessMarkers := countMetricWithValueFn(bs, labels.FromStrings("__name__", "stale"),
				func(_ int64, v float64) bool {
					return value.IsStaleNaN(v)
				})
			require.True(t, stalenessMarkers > 0, `found no staleness markers for stale{job="test"}`)
		},
	}
}
