package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"

	gsDatabase "github.com/gerp93/gameshell-framework/database"
)

// DefaultDeckCard is one entry in the embedded default-deck seed data.
type DefaultDeckCard struct {
	Year  int    `json:"year"`
	Event string `json:"event"`
}

// SeedDefaultDeckIfEmpty creates a public read-only deck from the given seed
// JSON, but only when the database has no decks yet — it never touches or
// duplicates an existing deck.
func SeedDefaultDeckIfEmpty(seedJSON []byte) error {
	deckCount, err := gsDatabase.CountDecks("")
	if err != nil {
		return err
	}
	if deckCount > 0 {
		return nil
	}

	var cards []DefaultDeckCard
	if err := json.Unmarshal(seedJSON, &cards); err != nil {
		log.Println(err)
		return errors.New("failed to parse default deck seed data")
	}

	deckId, err := gsDatabase.CreateDeck("History Trivia", "", true)
	if err != nil {
		return err
	}

	for _, c := range cards {
		if _, err := CreateCard(deckId, c.Event, sql.NullInt64{Int64: int64(c.Year), Valid: true}); err != nil {
			return err
		}
	}

	return nil
}
