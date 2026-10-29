// Copyright The Prometheus Authors
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

package receiver

import (
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

func TestExternalLabelsInjection(t *testing.T) {
	_, err := config.LoadFile("config_example.yml", false, nil)
	require.NoError(t, err)

	promCfg, err := config.Load(`
global:
  external_labels:
    project_id: test-project
    location: us-central1
    job: default-job
remote_write:
  - name: test-receiver
    url: http://127.0.0.1:9090/api/v1/write
`, nil)
	require.NoError(t, err)
	externalLabels := promCfg.GlobalConfig.ExternalLabels.Map()

	t.Run("merges_and_sorts_series_labels_without_affecting_exemplar_labels", func(t *testing.T) {
		httpReq, err := generateRequest(RequestOpts{
			Samples: []SampleWithLabels{
				{
					Labels: map[string]string{"__name__": "test_metric_total", "job": "override-job"},
					Value:  1.0,
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "test_metric_total", "job": "override-job"},
					ExemplarLabels: map[string]string{"trace_id": "abc123"},
					Value:          1.0,
				},
			},
		}, externalLabels)
		require.NoError(t, err)

		compressed, err := io.ReadAll(httpReq.Body)
		require.NoError(t, err)
		raw, err := snappy.Decode(nil, compressed)
		require.NoError(t, err)

		var req writev2.Request
		require.NoError(t, req.Unmarshal(raw))
		require.Len(t, req.Timeseries, 1)

		ts := req.Timeseries[0]
		gotSeriesLabels := make([]string, 0, len(ts.LabelsRefs))
		for _, ref := range ts.LabelsRefs {
			gotSeriesLabels = append(gotSeriesLabels, req.Symbols[ref])
		}
		// Series labels must include externalLabels in lexicographical order, with explicit test labels taking precedence.
		require.Equal(t, []string{
			"__name__", "test_metric_total",
			"job", "override-job",
			"location", "us-central1",
			"project_id", "test-project",
		}, gotSeriesLabels)

		require.Len(t, ts.Exemplars, 1)
		gotExemplarLabels := make([]string, 0, len(ts.Exemplars[0].LabelsRefs))
		for _, ref := range ts.Exemplars[0].LabelsRefs {
			gotExemplarLabels = append(gotExemplarLabels, req.Symbols[ref])
		}
		// Exemplar attachment labels must not receive externalLabels.
		require.Equal(t, []string{"trace_id", "abc123"}, gotExemplarLabels)
	})

	t.Run("merges_and_sorts_rw1_series_labels", func(t *testing.T) {
		httpReq, err := generateRW1Request([]SampleWithLabels{
			{
				Labels: map[string]string{"__name__": "rw1_metric_total", "job": "override-job"},
				Value:  1.0,
			},
		}, externalLabels)
		require.NoError(t, err)

		compressed, err := io.ReadAll(httpReq.Body)
		require.NoError(t, err)
		raw, err := snappy.Decode(nil, compressed)
		require.NoError(t, err)

		var req prompb.WriteRequest
		require.NoError(t, req.Unmarshal(raw))
		require.Len(t, req.Timeseries, 1)
		require.Equal(t, []prompb.Label{
			{Name: "__name__", Value: "rw1_metric_total"},
			{Name: "job", Value: "override-job"},
			{Name: "location", Value: "us-central1"},
			{Name: "project_id", Value: "test-project"},
		}, req.Timeseries[0].Labels)
	})

	t.Run("preserves_empty_labels_for_negative_tests", func(t *testing.T) {
		symbols := writev2.NewSymbolTable()
		require.Nil(t, symbolizeSortedLabels(&symbols, map[string]string{}, externalLabels))
	})
}

func TestFilterAndRemoteWriteConfigReceiver(t *testing.T) {
	promCfg, err := config.Load(`
remote_write:
  - name: alpha
    url: http://127.0.0.1:9091/api/v1/write
    headers:
      X-Scope-OrgID: tenant-a
  - name: beta
    url: http://127.0.0.1:9092/api/v1/write
`, nil)
	require.NoError(t, err)

	filtered := FilterRemoteWriteConfigs(promCfg.RemoteWriteConfigs, " beta , missing ")
	require.Len(t, filtered, 1)
	require.Equal(t, "beta", filtered[0].Name)

	rec, err := NewRemoteWriteConfigReceiver(promCfg.RemoteWriteConfigs[0])
	require.NoError(t, err)
	require.Equal(t, "alpha", rec.Name())

	_, err = NewRemoteWriteConfigReceiver(nil)
	require.Error(t, err)
}

func TestNextRequestTimeConcurrent(t *testing.T) {
	const workers = 6
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		times = make([]time.Time, 0, workers)
	)
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			ts := nextRequestTime()
			mu.Lock()
			times = append(times, ts)
			mu.Unlock()
		}()
	}
	wg.Wait()

	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	for i := 1; i < len(times); i++ {
		require.Greater(t, times[i].UnixMilli(), times[i-1].UnixMilli(), "concurrent calls must produce strictly increasing millisecond timestamps")
	}
}

