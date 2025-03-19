// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promqle2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/compliance/promqle2e"
)

func TestPrometheus_Counter_e2e(t *testing.T) {
	const interval = 30 * time.Second

	prom1 := promqle2e.PrometheusBackend{Name: "prom1", Image: "quay.io/prometheus/prometheus:v3.2.0"}
	prom2 := promqle2e.PrometheusBackend{Name: "prom2", Image: "quay.io/prometheus/prometheus:v2.55.0"}

	pt := promqle2e.NewScrapeStyleTest(t)

	//nolint:promlinter // Test metric.
	counter := promauto.With(pt.Registerer()).NewCounterVec(prometheus.CounterOpts{
		Name: "promqle2e_test_counter_total",
		Help: "Test counter used by promqle2e test framework for acceptance tests.",
	}, []string{"foo"})
	var c prometheus.Counter

	// No metric expected, counterVec empty.
	pt.RecordScrape(interval)

	c = counter.WithLabelValues("bar")
	c.Add(200)
	pt.RecordScrape(interval).
		Expect(c, 200, prom1).
		Expect(c, 200, prom2)

	c.Add(10)
	pt.RecordScrape(interval).
		Expect(c, 210, prom1).
		Expect(c, 210, prom2)

	c.Add(40)
	pt.RecordScrape(interval).
		Expect(c, 250, prom1).
		Expect(c, 250, prom2)

	// Reset to 0 (simulating instrumentation resetting metric or restarting target).
	counter.Reset()
	c = counter.WithLabelValues("bar")
	pt.RecordScrape(interval).
		Expect(c, 0, prom1).
		Expect(c, 0, prom2)

	c.Add(150)
	pt.RecordScrape(interval).
		Expect(c, 150, prom1).
		Expect(c, 150, prom2)

	// Reset to 0 with addition.
	counter.Reset()
	c = counter.WithLabelValues("bar")
	c.Add(20)
	pt.RecordScrape(interval).
		Expect(c, 20, prom1).
		Expect(c, 20, prom2)

	c.Add(50)
	pt.RecordScrape(interval).
		Expect(c, 70, prom1).
		Expect(c, 70, prom2)

	c.Add(10)
	pt.RecordScrape(interval).
		Expect(c, 80, prom1).
		Expect(c, 80, prom2)

	// Tricky reset case, unnoticeable reset for Prometheus without created timestamp.
	counter.Reset()
	c = counter.WithLabelValues("bar")
	c.Add(600)
	pt.RecordScrape(interval).
		Expect(c, 600, prom1).
		Expect(c, 600, prom2)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pt.Run(ctx)
}
