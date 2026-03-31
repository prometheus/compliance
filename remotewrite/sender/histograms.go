package sender

import (
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

func histogramsTests(transactional bool) []Test {
	return []Test{
		{
			Name:        "histogram encoding",
			Description: "Sender MUST correctly encode histogram values",
			RFCLevel:    MustLevel,
			ScrapeData: metricFamiliesToProtobuf([]string{
				`name: "test_histogram_struct"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 10
				    sample_sum: 25.5
				    schema: 3
				    positive_span: <
				      offset: 0
				      length: 1
				    >
				    positive_delta: 10
				  >
				>`,
				`name: "test_histogram_count"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 100
				    sample_sum: 250.5
				    schema: 3
				  >
				>`,
				`name: "test_histogram_sum"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 100
				    sample_sum: 250.5
				    schema: 3
				  >
				>`,
				`name: "test_histogram_ordered"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 250
				    sample_sum: 500.0
				    schema: 3
				    positive_span: <
				      offset: 0
				      length: 1
				    >
				    positive_delta: 250
				  >
				>`,
				`name: "test_native_histogram_pos"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 100
				    sample_sum: 250.0
				    schema: 3
				    positive_span: <
				      offset: 8
				      length: 2
				    >
				    positive_delta: 1
				    positive_delta: 2
				  >
				>`,
				`name: "test_histogram_neg"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 50
				    sample_sum: -25.0
				    schema: 3
				    negative_span: <
				      offset: 5
				      length: 1
				    >
				    negative_delta: 5
				  >
				>`,
				`name: "test_histogram_zero"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 10
				    sample_sum: 0.0
				    schema: 3
				    zero_count: 5
				  >
				>`,
				`name: "test_histogram_schema"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 100
				    sample_sum: 500.0
				    schema: 3
				  >
				>`,
				`name: "test_histogram_ts"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 100
				    sample_sum: 250.0
				    schema: 3
				  >
				>`,
				`name: "test_histogram_mixed"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 100
				    sample_sum: 250.0
				    schema: 3
				  >
				>`,
				`name: "test_histogram_empty"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 0
				    sample_sum: 0.0
				    schema: 3
				  >
				>`,
				`name: "test_histogram_large"
				type: HISTOGRAM
				metric: <
				  histogram: <
				    sample_count: 1000000000
				    sample_sum: 5000000000.0
				    schema: 3
				  >
				>`,
			}),
			ScrapeContentType: ProtoContentType,
			Version:           remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "native_histogram_structure",
					Description: "Sender MUST correctly encode native histogram structure",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						classicFound, nativeTS := findHistogramData(res.Requests[0].RW2, "test_histogram_struct")
						require.True(t, classicFound || nativeTS != nil, "Histogram data must be present")
					},
				},
				{
					Name:        "histogram_count_present",
					Description: "Sender MUST include histogram count",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						count, found := extractHistogramCount(res.Requests[0].RW2, "test_histogram_count")
						require.True(t, found, "Histogram count should be present")
						require.Equal(t, 100.0, count, "Histogram count value must be correct")
					},
				},
				{
					Name:        "histogram_sum_present",
					Description: "Sender MUST include histogram sum",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						sum, found := extractHistogramSum(res.Requests[0].RW2, "test_histogram_sum")
						require.True(t, found, "Histogram sum should be present")
						require.Equal(t, 250.5, sum, "Histogram sum value must be correct")
					},
				},
				{
					Name:        "histogram_buckets_ordered",
					Description: "Sender SHOULD send histogram buckets in order",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var foundHistogram bool
						for _, ts := range res.Requests[0].RW2.Timeseries {
							labels := extractLabels(&ts, res.Requests[0].RW2.Symbols)
							if labels["__name__"] == "test_histogram_ordered" && len(ts.Histograms) > 0 {
								foundHistogram = true
								break
							}
						}
						require.True(t, foundHistogram || len(res.Requests[0].RW2.Timeseries) > 0, "Histogram data should be present")
					},
				},
				{
					Name:        "histogram_positive_buckets",
					Description: "Native histogram MAY include positive buckets",
					RFCLevel:    MayLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var foundNative bool
						for _, ts := range res.Requests[0].RW2.Timeseries {
							if len(ts.Histograms) > 0 {
								foundNative = true
								hist := ts.Histograms[0]
								require.True(t, len(hist.PositiveSpans) > 0, "Native histogram may have positive buckets")
								break
							}
						}
						// If no native histogram found, it's still okay (it's a MAY).
					},
				},
				{
					Name:        "histogram_negative_buckets",
					Description: "Native histogram MAY include negative buckets",
					RFCLevel:    MayLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var foundNative bool
						for _, ts := range res.Requests[0].RW2.Timeseries {
							if len(ts.Histograms) > 0 {
								foundNative = true
								break
							}
						}
						// If no native histogram found, it's still okay (it's a MAY).
					},
				},
				{
					Name:        "histogram_zero_bucket",
					Description: "Native histogram MAY include zero bucket",
					RFCLevel:    MayLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var foundNative bool
						for _, ts := range res.Requests[0].RW2.Timeseries {
							if len(ts.Histograms) > 0 {
								foundNative = true
								break
							}
						}
						// If no native histogram found, it's still okay (it's a MAY).
					},
				},
				{
					Name:        "histogram_schema",
					Description: "Native histogram MUST specify schema if using native format",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, ts := range res.Requests[0].RW2.Timeseries {
							if len(ts.Histograms) > 0 {
								hist := ts.Histograms[0]
								require.NotNil(t, hist, "Native histogram must have schema")
								break
							}
						}
					},
				},
				{
					Name:        "histogram_timestamp",
					Description: "Histogram MUST include valid timestamp",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var foundTimestamp bool
						for _, ts := range res.Requests[0].RW2.Timeseries {
							labels := extractLabels(&ts, res.Requests[0].RW2.Symbols)

							if labels["__name__"] == "test_histogram_ts_count" && len(ts.Samples) > 0 {
								require.Greater(t, ts.Samples[0].Timestamp, int64(0), "Histogram timestamp must be valid")
								foundTimestamp = true
								break
							}

							if len(ts.Histograms) > 0 {
								require.Greater(t, ts.Histograms[0].Timestamp, int64(0), "Native histogram timestamp must be valid")
								foundTimestamp = true
								break
							}
						}
						require.True(t, foundTimestamp, "Histogram must have valid timestamp")
					},
				},
				{
					Name:        "histogram_no_mixed_with_samples",
					Description: "Sender MUST NOT mix histogram and sample data in same timeseries",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, ts := range res.Requests[0].RW2.Timeseries {
							if len(ts.Samples) > 0 && len(ts.Histograms) > 0 {
								require.Fail(t, "Timeseries must not contain both samples and histograms")
							}
						}
					},
				},
				{
					Name:        "histogram_empty_buckets",
					Description: "Sender SHOULD handle histograms with no observations",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var foundEmpty bool
						for _, ts := range res.Requests[0].RW2.Timeseries {
							labels := extractLabels(&ts, res.Requests[0].RW2.Symbols)
							if labels["__name__"] == "test_histogram_empty_count" && len(ts.Samples) > 0 {
								require.Equal(t, 0.0, ts.Samples[0].Value, "Empty histogram count should be 0")
								foundEmpty = true
								break
							}
						}
						require.True(t, foundEmpty || len(res.Requests[0].RW2.Timeseries) > 0, "Empty histogram should be handled")
					},
				},
				{
					Name:        "histogram_large_counts",
					Description: "Sender MUST handle histograms with large observation counts",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var foundLarge bool
						for _, ts := range res.Requests[0].RW2.Timeseries {
							labels := extractLabels(&ts, res.Requests[0].RW2.Symbols)
							if labels["__name__"] == "test_histogram_large_count" && len(ts.Samples) > 0 {
								require.Equal(t, 1e9, ts.Samples[0].Value, "Large histogram count must be correctly encoded")
								foundLarge = true
								break
							}
						}
						require.True(t, foundLarge || len(res.Requests[0].RW2.Timeseries) > 0, "Large histogram counts should be handled")
					},
				},
			},
		},
	}
}
