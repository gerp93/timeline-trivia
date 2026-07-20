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
// validation. Category is carried through and required — cards are placed into
// one of the predefined TIMELINE_TRIVIA_CATEGORY entries.
type DefaultDeckCard struct {
	Year     int
	Event    string
	Category string
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

		cards = append(cards, DefaultDeckCard{Year: *r.Year, Event: event, Category: category})
	}

	return cards, nil
}

// distinctCategoryNames returns the unique category names from a parsed import,
// preserving first-seen order.
func distinctCategoryNames(cards []DefaultDeckCard) []string {
	seen := make(map[string]bool)
	names := make([]string, 0)
	for _, c := range cards {
		if !seen[c.Category] {
			seen[c.Category] = true
			names = append(names, c.Category)
		}
	}
	return names
}

// SeedCategoriesIfEmpty seeds the predefined category list from the distinct
// categories in the given import JSON, but only when the category table is
// empty. Runs on every startup (idempotent) and independently of deck seeding,
// so a database whose default deck already exists still gets its base
// categories.
func SeedCategoriesIfEmpty(seedJSON []byte) error {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_CATEGORY`
	rows, err := query(sqlString)
	if err != nil {
		return err
	}
	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			rows.Close()
			log.Println(err)
			return errors.New("failed to scan row in query results")
		}
	}
	rows.Close()
	if count > 0 {
		return nil
	}

	cards, err := ParseCardImportJSON(seedJSON)
	if err != nil {
		log.Println(err)
		return errors.New("failed to parse default deck seed data for categories")
	}

	for _, name := range distinctCategoryNames(cards) {
		if _, err := CreateCategory(name); err != nil {
			return err
		}
	}

	return nil
}

// SeedDefaultDeckIfEmpty creates a public read-only deck from the given seed
// JSON, but only when the database has no decks yet — it never touches or
// duplicates an existing deck. Each card is placed into its predefined
// category (which SeedCategoriesIfEmpty must have created first).
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

	categoryIds, err := resolveCategoryIds(distinctCategoryNames(cards))
	if err != nil {
		return err
	}

	deckId, err := gsDatabase.CreateDeck("History Trivia - Default", "", true)
	if err != nil {
		return err
	}

	for _, c := range cards {
		categoryId := uuid.NullUUID{UUID: categoryIds[c.Category], Valid: true}
		if _, err := CreateCard(deckId, c.Event, sql.NullInt64{Int64: int64(c.Year), Valid: true}, categoryId); err != nil {
			return err
		}
	}

	return nil
}

// BackfillDefaultDeckCategories sets CATEGORY_ID on cards that predate the
// category system by matching each card's text against the seed JSON. It only
// touches cards whose category is currently NULL, so it's safe to run on every
// startup and never overrides an admin's later choice.
func BackfillDefaultDeckCategories(seedJSON []byte) error {
	cards, err := ParseCardImportJSON(seedJSON)
	if err != nil {
		log.Println(err)
		return errors.New("failed to parse default deck seed data for backfill")
	}

	categoryIds, err := resolveCategoryIds(distinctCategoryNames(cards))
	if err != nil {
		return err
	}

	sqlString := `
		UPDATE CARD
		SET CATEGORY_ID = ?
		WHERE TEXT = ?
			AND CATEGORY_ID IS NULL
	`
	for _, c := range cards {
		if err := execute(sqlString, categoryIds[c.Category], c.Event); err != nil {
			return err
		}
	}

	return nil
}

// resolveCategoryIds maps each category name to its id, returning an error
// naming the first category that isn't in the predefined list.
func resolveCategoryIds(names []string) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(names))
	for _, name := range names {
		id, err := GetCategoryId(name)
		if err != nil {
			return nil, err
		}
		if id == uuid.Nil {
			return nil, fmt.Errorf("category %q is not in the predefined list; an admin must create it first", name)
		}
		ids[name] = id
	}
	return ids, nil
}

// ImportCardsIntoDeck validates a card-import JSON payload and inserts the
// cards into an existing deck. Every category referenced must already exist in
// the predefined list, otherwise the whole import is rejected (naming the
// missing category). Entries whose event text already exists in the deck are
// skipped (CARD has a UNIQUE(DECK_ID, TEXT) constraint) rather than aborting
// the whole import, so re-uploading the same file is a no-op for cards already
// present.
func ImportCardsIntoDeck(deckId uuid.UUID, data []byte) (imported int, skipped int, err error) {
	cards, err := ParseCardImportJSON(data)
	if err != nil {
		return 0, 0, err
	}

	categoryIds, err := resolveCategoryIds(distinctCategoryNames(cards))
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
		categoryId := uuid.NullUUID{UUID: categoryIds[c.Category], Valid: true}
		if _, err := CreateCard(deckId, c.Event, sql.NullInt64{Int64: int64(c.Year), Valid: true}, categoryId); err != nil {
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

// SeedDefaultUserIfEmpty creates a default user account if none exist.
// This is used on fresh deployments to provide an initial login.
func SeedDefaultUserIfEmpty() error {
	sqlString := `SELECT COUNT(*) FROM USER`
	rows, err := query(sqlString)
	if err != nil {
		log.Println(err)
		return errors.New("failed to check if users exist")
	}
	defer rows.Close()

	var count int
	if !rows.Next() {
		return errors.New("failed to query user count")
	}
	if err := rows.Scan(&count); err != nil {
		log.Println(err)
		return errors.New("failed to scan user count")
	}

	if count > 0 {
		return nil
	}

	return gsDatabase.CreateUser("default", "password", false)
}
