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

// basicMetric returns a minimal label set with just a metric name.
func basicMetric(name string) map[string]string {
	return map[string]string{"__name__": name}
}

// testJobInstanceLabels returns a commonly used job/instance label set.
func testJobInstanceLabels() map[string]string {
	return map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9090"}
}

// traceExemplar returns exemplar labels carrying a trace ID.
func traceExemplar(traceID string) map[string]string {
	return map[string]string{"trace_id": traceID}
}
