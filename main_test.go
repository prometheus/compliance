package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/remote-write-compliance/targets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var logger = log.NewNopLogger()

func TestRemoteWrite(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metrics  http.Handler
		expected validator
	}{
		{
			name: "Basic Test",
			metrics: funcHandler("now", func() float64 {
				return float64(time.Now().Unix() * 1000)
			}),
			expected: func(t *testing.T, bs []batch) {
				ups, nows := 0, 0
				require.True(t, len(bs) > 0)
				for _, b := range bs {
					require.True(t, len(b.samples) > 0)
					for _, s := range b.samples {
						ls := removeLabel(s.l, "instance")
						require.Equal(t, "__name__", ls[0].Name)
						switch s.l[0].Value {
						case "up":
							require.Equal(t, labels.FromStrings("__name__", "up", "job", "test"), ls)
							require.Equal(t, float64(1), s.v)
							ups++
						case "now":
							require.Equal(t, labels.FromStrings("__name__", "now", "job", "test"), ls)
							assert.InEpsilon(t, float64(s.t), s.v, 0.01)
							nows++
						case "scrape_duration_seconds", "scrape_samples_scraped", "scrape_samples_post_metric_relabeling", "scrape_series_added":
							// this is optional, but acceptable metric.
						default:
							require.False(t, true, "unkown metric: %s", s.l)
						}
					}
				}
				require.Equal(t, ups, nows)
			},
		},
		{
			name: "Invalid Scrape",
			metrics: staticHandler([]byte(`
			# this is not valid prometheus
			1234notvali}{ 444
			`)),
			expected: func(t *testing.T, bs []batch) {
				require.True(t, len(bs) > 0)
				for _, b := range bs {
					require.True(t, len(b.samples) > 0)
					for _, s := range b.samples {
						ls := removeLabel(s.l, "instance")
						require.Equal(t, "__name__", ls[0].Name)
						switch s.l[0].Value {
						case "up":
							require.Equal(t, labels.FromStrings("__name__", "up", "job", "test"), ls)
							require.Equal(t, float64(0), s.v)
						case "scrape_duration_seconds":
							// this is optional, but acceptable metric.
						default:
							require.False(t, false, "unkown metric: %s", s.l)
						}
					}
				}
			},
		},
	} {
		for name, runner := range runners {
			t.Run(fmt.Sprintf("%s/%s", tc.name, name), func(t *testing.T) {
				runTest(t, tc.metrics, tc.expected, runner)
			})
		}
	}
}

type validator func(t *testing.T, bs []batch)

func removeLabel(ls labels.Labels, name string) labels.Labels {
	for i := 0; i < len(ls); i++ {
		if ls[i].Name == name {
			return ls[:i+copy(ls[i:], ls[i+1:])]
		}
	}
	return ls
}

var runners = map[string]targets.Target{
	"prometheus":    targets.RunPrometheus,
	"otelcollector": targets.RunOtelCollector,
}

func funcHandler(name string, f func() float64) http.Handler {
	gauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: name,
	}, f)
	r := prometheus.NewPedanticRegistry()
	r.Register(gauge)
	return promhttp.HandlerFor(r, promhttp.HandlerOpts{})
}

func staticHandler(contents []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(contents)
	})
}

func runTest(t *testing.T, metrics http.Handler, expected validator, runner targets.Target) {
	ap := appendable{}

	// Start a HTTP server to expose some metrics and a receive remote write requests.
	m := http.NewServeMux()
	m.Handle("/metrics", metrics)
	m.Handle("/push", remote.NewWriteHandler(logger, &ap))
	s := http.Server{
		Handler: m,
	}
	l, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	go s.Serve(l)
	defer s.Close()

	// Run Prometheus to scrape and send metrics.
	scrapeTarget := l.Addr().String()
	receiveEndpoint := fmt.Sprintf("http://%s/push", l.Addr().String())
	require.NoError(t, runner(targets.TargetOptions{
		ScrapeTarget:    scrapeTarget,
		ReceiveEndpoint: receiveEndpoint,
	}))

	// Check we got some data.
	require.True(t, len(ap.batches) > 0)
	expected(t, ap.batches)
}

type appendable struct {
	sync.Mutex
	batches []batch
}

type batch struct {
	appender *appendable
	samples  []sample
}

type sample struct {
	l labels.Labels
	t int64
	v float64
}

func (m *appendable) Appender(_ context.Context) storage.Appender {
	b := &batch{
		appender: m,
	}
	return b
}

func (m *batch) Append(_ uint64, l labels.Labels, t int64, v float64) (uint64, error) {
	m.samples = append(m.samples, sample{l, t, v})
	return 0, nil
}

func (m *batch) Commit() error {
	m.appender.Mutex.Lock()
	defer m.appender.Mutex.Unlock()
	m.appender.batches = append(m.appender.batches, *m)
	return nil
}

func (*batch) Rollback() error {
	return nil
}

func (*batch) AppendExemplar(ref uint64, l labels.Labels, e exemplar.Exemplar) (uint64, error) {
	return 0, nil
}
