package cases

import (
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestSampleSlice(t *testing.T) {
	act := sampleSlice(15*time.Second,
		"1x1", "0x3", "5x3", "9", "8", "-2x2",
	)

	expVals := []float64{1, 1, 1, 1, 6, 11, 16, 9, 8, 6, 4}
	exp := make([]prompb.Sample, 0, len(act))
	ts := time.Unix(0, 0)
	for _, v := range expVals {
		exp = append(exp, prompb.Sample{
			Value:     v,
			Timestamp: timestamp.FromTime(ts),
		})
		ts = ts.Add(15 * time.Second)
	}

	require.Equal(t, exp, act)
}
