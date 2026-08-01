package apiPages

import (
	"sort"
	"testing"

	"github.com/gerp93/timeline-trivia/database"
)

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

// Regression test for a decade appearing in BOTH "most successful" and
// "least successful". Capping each list by count is not sufficient when
// rates tie: ranking the same slice ascending and descending with a stable
// sort leaves the first tied element at the head of both. Caught by
// rendering a real user's page — 1980s/1990s/2000s/2020s at 83/80/80/70%
// showed 1990s on both sides.
func TestSplitSuccessfulDecadesNeverRepeatsADecade(t *testing.T) {
	cases := [][]database.DecadeStat{
		// The exact real-data shape that exposed the bug.
		{
			{Decade: 1980, Attempts: 12, Correct: 10},
			{Decade: 1990, Attempts: 10, Correct: 8},
			{Decade: 2000, Attempts: 10, Correct: 8},
			{Decade: 2020, Attempts: 10, Correct: 7},
		},
		// Every rate identical — the worst case for tie handling.
		{
			{Decade: 1900, Attempts: 10, Correct: 5},
			{Decade: 1910, Attempts: 10, Correct: 5},
			{Decade: 1920, Attempts: 10, Correct: 5},
			{Decade: 1930, Attempts: 10, Correct: 5},
		},
		// Odd count, so the middle decade belongs to neither list.
		{
			{Decade: 1900, Attempts: 10, Correct: 9},
			{Decade: 1910, Attempts: 10, Correct: 5},
			{Decade: 1920, Attempts: 10, Correct: 1},
		},
		{},
		{{Decade: 1900, Attempts: 10, Correct: 5}},
	}

	for _, qualified := range cases {
		most, least := splitSuccessfulDecades(qualified)

		seen := map[int]bool{}
		for _, d := range most {
			seen[d.Decade] = true
		}
		for _, d := range least {
			if seen[d.Decade] {
				t.Errorf("qualified=%d: decade %s is in both most and least successful",
					len(qualified), d.Label())
			}
		}

		if len(most) != len(least) {
			t.Errorf("qualified=%d: list sizes differ, most=%d least=%d",
				len(qualified), len(most), len(least))
		}
		if len(most)+len(least) > len(qualified) {
			t.Errorf("qualified=%d: lists total %d, more decades than exist",
				len(qualified), len(most)+len(least))
		}

		// most must read best-first, least worst-first.
		if !sort.SliceIsSorted(most, func(i, j int) bool { return most[i].Rate() > most[j].Rate() }) {
			t.Errorf("qualified=%d: most successful not ordered best-first", len(qualified))
		}
		if !sort.SliceIsSorted(least, func(i, j int) bool { return least[i].Rate() < least[j].Rate() }) {
			t.Errorf("qualified=%d: least successful not ordered worst-first", len(qualified))
		}
	}
}
