package cases

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

func TestLabelsContain(t *testing.T) {
	for _, tc := range []struct {
		outer, inner labels.Labels
		found        bool
	}{
		{
			outer: labels.FromStrings("foo", "bar"),
			inner: labels.FromStrings("foo", "bar"),
			found: true,
		},
		{
			outer: labels.FromStrings("foo", "bar"),
			inner: labels.FromStrings("foo", "baz"),
			found: false,
		},
		{
			outer: labels.FromStrings("foo", "bar", "blip", "blop"),
			inner: labels.FromStrings("foo", "bar"),
			found: true,
		},
		{
			outer: labels.FromStrings(),
			inner: labels.FromStrings("foo", "bar"),
			found: false,
		},
	} {
		t.Run(fmt.Sprintf("%s ∩ %s", tc.inner.String(), tc.outer.String()), func(t *testing.T) {
			require.Equal(t, tc.found, labelsContain(tc.outer, tc.inner))
		})
	}
}
