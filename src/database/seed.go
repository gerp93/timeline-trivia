package database

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"
)

// Limits for card-import JSON (both the embedded default deck and any
// user-uploaded file), matching the CARD table's own constraints plus a
// sane cap on how much a single request may insert.
const (
	maxImportCards       = 1000
	maxImportEventLen    = 510 // CARD.TEXT VARCHAR(510)
	maxImportCategoryLen = 255
	minImportYear        = -10000
	maxImportYear        = 3000
)

// DefaultDeckCard is one entry in a card-import JSON payload, after
// validation. Category is accepted (and required) on input for strict
// schema matching, but isn't persisted — the CARD table has no category
// column.
type DefaultDeckCard struct {
	Year  int
	Event string
}

// importCardJSON is the strict on-the-wire shape: exactly {year, event,
// category}, nothing more, nothing less. Pointer fields so a missing key
// (nil) is distinguishable from an explicit zero value/empty string.
type importCardJSON struct {
	Year     *int    `json:"year"`
	Event    *string `json:"event"`
	Category *string `json:"category"`
}

// ParseCardImportJSON strictly parses and validates a card-import payload:
// a JSON array of {"year": int, "event": string, "category": string}
// objects, and nothing else. Any unknown field, wrong type, missing
// required field, or out-of-range value is rejected outright rather than
// silently ignored or coerced — this is untrusted input (an uploaded file
// or the embedded seed data) headed straight for a SQL INSERT and for
// html/template rendering, so the parser is deliberately strict.
func ParseCardImportJSON(data []byte) ([]DefaultDeckCard, error) {
	if len(data) == 0 {
		return nil, errors.New("no data provided")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var raw []importCardJSON
	if err := decoder.Decode(&raw); err != nil {
		return nil, errors.New(`invalid JSON: expected an array of {"year": number, "event": string, "category": string} objects`)
	}
	if decoder.More() {
		return nil, errors.New("invalid JSON: unexpected trailing data after the array")
	}

	if len(raw) == 0 {
		return nil, errors.New("no cards found: the array is empty")
	}
	if len(raw) > maxImportCards {
		return nil, fmt.Errorf("too many cards: %d provided, max is %d per import", len(raw), maxImportCards)
	}

	seenText := make(map[string]bool, len(raw))
	cards := make([]DefaultDeckCard, 0, len(raw))
	for i, r := range raw {
		if r.Year == nil {
			return nil, fmt.Errorf(`entry %d: "year" is required`, i)
		}
		if *r.Year < minImportYear || *r.Year > maxImportYear {
			return nil, fmt.Errorf("entry %d: year %d is out of the allowed range (%d to %d)", i, *r.Year, minImportYear, maxImportYear)
		}

		if r.Event == nil {
			return nil, fmt.Errorf(`entry %d: "event" is required`, i)
		}
		event := strings.TrimSpace(*r.Event)
		if event == "" {
			return nil, fmt.Errorf(`entry %d: "event" cannot be blank`, i)
		}
		if len(event) > maxImportEventLen {
			return nil, fmt.Errorf(`entry %d: "event" exceeds %d characters`, i, maxImportEventLen)
		}
		if seenText[event] {
			return nil, fmt.Errorf("entry %d: duplicate event text within this import", i)
		}
		seenText[event] = true

		if r.Category == nil {
			return nil, fmt.Errorf(`entry %d: "category" is required`, i)
		}
		category := strings.TrimSpace(*r.Category)
		if category == "" {
			return nil, fmt.Errorf(`entry %d: "category" cannot be blank`, i)
		}
		if len(category) > maxImportCategoryLen {
			return nil, fmt.Errorf(`entry %d: "category" exceeds %d characters`, i, maxImportCategoryLen)
		}

		cards = append(cards, DefaultDeckCard{Year: *r.Year, Event: event})
	}

	return cards, nil
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

	cards, err := ParseCardImportJSON(seedJSON)
	if err != nil {
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

// ImportCardsIntoDeck validates a card-import JSON payload and inserts the
// cards into an existing deck. Entries whose event text already exists in
// the deck are skipped (CARD has a UNIQUE(DECK_ID, TEXT) constraint) rather
// than aborting the whole import, so re-uploading the same file is a no-op
// for cards already present.
func ImportCardsIntoDeck(deckId uuid.UUID, data []byte) (imported int, skipped int, err error) {
	cards, err := ParseCardImportJSON(data)
	if err != nil {
		return 0, 0, err
	}

	existingTexts, err := getCardTextsInDeck(deckId)
	if err != nil {
		return 0, 0, err
	}

	for _, c := range cards {
		if existingTexts[c.Event] {
			skipped++
			continue
		}
		if _, err := CreateCard(deckId, c.Event, sql.NullInt64{Int64: int64(c.Year), Valid: true}); err != nil {
			return imported, skipped, err
		}
		imported++
	}

	return imported, skipped, nil
}

// getCardTextsInDeck returns the set of existing card texts in a deck, used
// to skip duplicates during import without relying on catching a DB
// constraint-violation error (the shared database layer doesn't preserve
// the underlying driver error for that).
func getCardTextsInDeck(deckId uuid.UUID) (map[string]bool, error) {
	sqlString := `SELECT TEXT FROM CARD WHERE DECK_ID = ?`
	rows, err := query(sqlString, deckId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	texts := make(map[string]bool)
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		texts[text] = true
	}
	return texts, nil
}
