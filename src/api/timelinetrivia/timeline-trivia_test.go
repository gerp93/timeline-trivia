package apiTimelineTrivia

import (
	"database/sql"
	"testing"
)

func TestTurnSecondsRemaining(t *testing.T) {
	tests := []struct {
		name         string
		timerSeconds int
		turnElapsed  sql.NullInt64
		want         int
	}{
		{"no timer configured", 0, sql.NullInt64{Int64: 5, Valid: true}, 0},
		{"no turn in progress", 45, sql.NullInt64{}, 0},
		{"just started, well inside grace", 45, sql.NullInt64{Int64: 1, Valid: true}, 45},
		{"right at the grace boundary", 45, sql.NullInt64{Int64: turnTimerGraceSeconds, Valid: true}, 45},
		{"past grace, ticking down", 45, sql.NullInt64{Int64: turnTimerGraceSeconds + 10, Valid: true}, 35},
		{"exactly expired", 45, sql.NullInt64{Int64: turnTimerGraceSeconds + 45, Valid: true}, 0},
		{"long past expiry, clamped at zero", 45, sql.NullInt64{Int64: turnTimerGraceSeconds + 999, Valid: true}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := turnSecondsRemaining(tc.timerSeconds, tc.turnElapsed)
			if got != tc.want {
				t.Errorf("turnSecondsRemaining(%d, %+v) = %d, want %d", tc.timerSeconds, tc.turnElapsed, got, tc.want)
			}
		})
	}
}
