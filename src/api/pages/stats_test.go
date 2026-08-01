package apiPages

import "testing"

// Regression test for "Most Successful" and "Least Successful" showing the
// exact same decades (just reordered) when few decades qualify — reported
// as the page looking broken with only 2 qualified decades.
func TestSuccessfulSplitSizeNeverOverlaps(t *testing.T) {
	cases := []struct {
		qualifiedCount int
		want           int
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{3, 1},
		{4, 2},
		{9, 4},
		{10, 5},
		{11, 5},
		{20, 5},
	}
	for _, c := range cases {
		got := successfulSplitSize(c.qualifiedCount)
		if got != c.want {
			t.Errorf("successfulSplitSize(%d) = %d, want %d", c.qualifiedCount, got, c.want)
		}
		// The actual invariant that matters: 2*got can never exceed
		// qualifiedCount, or the most/least lists would overlap.
		if 2*got > c.qualifiedCount {
			t.Errorf("successfulSplitSize(%d) = %d would let most/least successful overlap", c.qualifiedCount, got)
		}
	}
}
