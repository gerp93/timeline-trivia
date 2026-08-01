package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
)

// Regression test for B.C.E. years being bucketed one decade too old.
// GetUserDecadeStats used FLOOR(CARD_YEAR/10)*10, which rounds toward
// negative infinity: 55 B.C.E (-55) became -60, labelled "60s B.C.E" even
// though the 50s B.C.E decade spans 50-59 B.C.E. Worse, it merged years from
// two real decades into one bucket (202 B.C.E and 210 B.C.E both landed in
// -210), so the attempt counts themselves were wrong, not just the labels.
// Confirmed against a production backup: 9 of 12 real B.C.E guesses were
// mislabelled. C.E years are unaffected either way.
func TestDecadeBucketingHandlesBCEYears(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("TIMELINE_TRIVIA_SQL_DATABASE"), "tt_e2e") {
		t.Skip("set TIMELINE_TRIVIA_SQL_DATABASE=tt_e2e")
	}
	gsDatabase.SetEnvVarPrefix("TIMELINE_TRIVIA")
	if _, err := gsDatabase.CreateDatabaseConnection(); err != nil {
		t.Fatalf("db: %v", err)
	}
	setupSchema(t)

	userName := "decade_bucket_user_" + uuid.NewString()[:8]
	if err := gsDatabase.CreateUser(userName, "unused", true); err != nil {
		t.Fatalf("create user: %v", err)
	}
	userId, err := gsDatabase.GetUserIdByName(userName)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	deckId, err := gsDatabase.CreateDeck("decade bucket deck "+uuid.NewString()[:8], "", true)
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}

	// year -> expected decade label. The -202/-210 pair is the important one:
	// under the old FLOOR bucketing they collapsed into a single "210s B.C.E"
	// row with 2 attempts instead of two rows with 1 each.
	cases := []struct {
		year  int
		label string
	}{
		{-2291, "2290s B.C.E"},
		{-55, "50s B.C.E"},
		{-202, "200s B.C.E"},
		{-210, "210s B.C.E"},
		{-27, "20s B.C.E"},
		{-60, "60s B.C.E"},
		{1985, "1980s"},
		{5, "0s"},
	}

	for _, c := range cases {
		cardId, err := database.CreateCard(deckId, fmt.Sprintf("decade probe %d", c.year),
			sql.NullInt64{Int64: int64(c.year), Valid: true}, uuid.NullUUID{})
		if err != nil {
			t.Fatalf("create card %d: %v", c.year, err)
		}
		if err := database.LogGuess(userId, cardId, c.year, true); err != nil {
			t.Fatalf("log guess %d: %v", c.year, err)
		}
	}

	decades, err := database.GetUserDecadeStats(userId, userId)
	if err != nil {
		t.Fatalf("decade stats: %v", err)
	}

	got := map[string]int{}
	for _, d := range decades {
		got[d.Label()] = d.Attempts
	}
	for _, c := range cases {
		if got[c.label] != 1 {
			t.Errorf("year %d: expected exactly 1 attempt in bucket %q, got %d (all buckets: %v)",
				c.year, c.label, got[c.label], got)
		}
	}
}
