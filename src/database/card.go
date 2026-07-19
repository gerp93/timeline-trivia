package database

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
)

type Card struct {
	Id            uuid.UUID
	CreatedOnDate time.Time
	ChangedOnDate time.Time

	DeckId uuid.UUID
	Text   string
	Year   sql.NullInt64
}

func SearchCardsInDeck(deckId uuid.UUID, text string, page int) ([]Card, error) {
	text = "%" + text + "%"

	if page < 1 {
		page = 1
	}

	sqlString := `
		SELECT
			ID,
			CREATED_ON_DATE,
			CHANGED_ON_DATE,
			DECK_ID,
			TEXT,
			CARD_YEAR
		FROM CARD
		WHERE DECK_ID = ?
			AND TEXT LIKE ?
		ORDER BY CARD_YEAR, TEXT
		LIMIT 10 OFFSET ?
	`
	rows, err := query(sqlString, deckId, text, (page-1)*10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Card, 0)
	for rows.Next() {
		var card Card
		if err := rows.Scan(
			&card.Id,
			&card.CreatedOnDate,
			&card.ChangedOnDate,
			&card.DeckId,
			&card.Text,
			&card.Year); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, card)
	}
	return result, nil
}

func CountCardsInDeck(deckId uuid.UUID, text string) (int, error) {
	text = "%" + text + "%"

	sqlString := `
		SELECT
			COUNT(*)
		FROM CARD
		WHERE DECK_ID = ?
			AND TEXT LIKE ?
	`
	rows, err := query(sqlString, deckId, text)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			log.Println(err)
			return 0, errors.New("failed to scan row in query results")
		}
	}

	return count, nil
}

func GetCard(id uuid.UUID) (Card, error) {
	var card Card

	sqlString := `
		SELECT
			ID,
			CREATED_ON_DATE,
			CHANGED_ON_DATE,
			DECK_ID,
			TEXT,
			CARD_YEAR
		FROM CARD
		WHERE ID = ?
	`
	rows, err := query(sqlString, id)
	if err != nil {
		return card, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(
			&card.Id,
			&card.CreatedOnDate,
			&card.ChangedOnDate,
			&card.DeckId,
			&card.Text,
			&card.Year); err != nil {
			log.Println(err)
			return card, errors.New("failed to scan row in query results")
		}
	}

	return card, nil
}

func GetCardId(deckId uuid.UUID, text string) (uuid.UUID, error) {
	var id uuid.UUID

	sqlString := `
		SELECT
			ID
		FROM CARD
		WHERE DECK_ID = ?
			AND TEXT = ?
	`
	rows, err := query(sqlString, deckId, text)
	if err != nil {
		return id, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&id); err != nil {
			log.Println(err)
			return id, errors.New("failed to scan row in query results")
		}
	}

	return id, nil
}

func CreateCard(deckId uuid.UUID, text string, year sql.NullInt64) (uuid.UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return id, errors.New("failed to generate new id")
	}

	sqlString := `
		INSERT INTO CARD(ID, DECK_ID, TEXT, CARD_YEAR)
		VALUES (?, ?, ?, ?)
	`
	return id, execute(sqlString, id, deckId, text, year)
}

func UpdateCard(id uuid.UUID, text string, year sql.NullInt64) error {
	sqlString := `
		UPDATE CARD
		SET TEXT = ?,
			CARD_YEAR = ?
		WHERE ID = ?
	`
	return execute(sqlString, text, year, id)
}

func DeleteCard(id uuid.UUID) error {
	sqlString := `
		DELETE
		FROM CARD
		WHERE ID = ?
	`
	return execute(sqlString, id)
}

// AuditDeckCardsAsDeleted snapshots all of a deck's cards into AUDIT_CARD as
// 'DELETE'. Called from the OnDeckDeleting hook because MariaDB FK cascade does
// not fire the CARD delete trigger when the framework deletes the deck.
func AuditDeckCardsAsDeleted(deckId uuid.UUID) error {
	sqlString := `
		INSERT INTO AUDIT_CARD(AUDIT_TYPE, CARD_ID, DECK_ID, TEXT, CARD_YEAR)
		SELECT 'DELETE', ID, DECK_ID, TEXT, CARD_YEAR
		FROM CARD
		WHERE DECK_ID = ?
	`
	return execute(sqlString, deckId)
}

// GetCardsInDeckExport returns a deck's cards for CSV export (text, year).
func GetCardsInDeckExport(deckId uuid.UUID) ([]Card, error) {
	sqlString := `
		SELECT
			TEXT,
			CARD_YEAR
		FROM CARD
		WHERE DECK_ID = ?
		ORDER BY CARD_YEAR, TEXT
	`
	rows, err := query(sqlString, deckId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Card, 0)
	for rows.Next() {
		var card Card
		if err := rows.Scan(&card.Text, &card.Year); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, card)
	}
	return result, nil
}
