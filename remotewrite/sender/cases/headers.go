package cases

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HeadersTest exports a single metric - a gauge - and checks that we receive
// the right headers on remote write requests.
func HeadersTest() Test {
	ec := make(chan error)
	errors := []error{}
	go func() {
		for err := range ec {
			errors = append(errors, err)
		}
	}()

	return Test{
		Name: "Headers",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "now",
		}, func() float64 {
			return float64(time.Now().Unix() * 1000)
		})),
		Writes: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertHeaderExistsAndHasValue(r, ec, "Content-Encoding", "snappy")
				assertHeaderExistsAndHasValue(r, ec, "Content-Type", "application/x-protobuf")
				assertHeaderExistsAndHasValue(r, ec, "X-Prometheus-Remote-Write-Version", "0.1.0")
				next.ServeHTTP(w, r)
			})
		},
		Expected: func(t *testing.T, bs []Batch) {
			nows := countMetricWithValueFn(bs, labels.FromStrings("__name__", "now"),
				func(ts int64, v float64) bool {
					assert.InEpsilon(t, float64(ts), v, 0.01)
					return true
				})
			require.True(t, nows > 0, `found zero samples for {__name__="now"}`)
			require.Empty(t, errors)
		},
	}
}

func assertHeaderExistsAndHasValue(r *http.Request, errs chan error, name, expected string) {
	if actual := r.Header.Get(name); actual != expected {
		errs <- fmt.Errorf("header '%s' != '%s'; value is '%s'", name, expected, actual)
	}
}
