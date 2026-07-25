package database

import "testing"

// ValidateCardsToWin is a pure function — no database needed to exercise its
// bounds. min=5/max=20 mirror the lobby-creation form's min/max attributes,
// which are only a client-side hint; this is the enforcement that actually
// matters.
func TestValidateCardsToWinBounds(t *testing.T) {
	const ampleCards = 1000 // never the limiting factor in these cases

	cases := []struct {
		name       string
		cardsToWin int
		wantErr    bool
	}{
		{"below minimum", MinCardsToWin - 1, true},
		{"at minimum", MinCardsToWin, false},
		{"typical default", 10, false},
		{"at maximum", MaxCardsToWin, false},
		{"above maximum", MaxCardsToWin + 1, true},
		{"way above maximum", 500, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCardsToWin(tc.cardsToWin, ampleCards)
			if tc.wantErr && err == nil {
				t.Errorf("cardsToWin=%d: expected an error, got nil", tc.cardsToWin)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("cardsToWin=%d: unexpected error: %v", tc.cardsToWin, err)
			}
		})
	}
}
