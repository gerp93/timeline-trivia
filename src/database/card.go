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

	DeckId       uuid.UUID
	Text         string
	Year         sql.NullInt64
	CategoryId   uuid.NullUUID
	CategoryName sql.NullString
}

func SearchCardsInDeck(deckId uuid.UUID, text string, page int) ([]Card, error) {
	text = "%" + text + "%"

	if page < 1 {
		page = 1
	}

	sqlString := `
		SELECT
			C.ID,
			C.CREATED_ON_DATE,
			C.CHANGED_ON_DATE,
			C.DECK_ID,
			C.TEXT,
			C.CARD_YEAR,
			C.CATEGORY_ID,
			TC.NAME
		FROM CARD AS C
			LEFT JOIN TIMELINE_TRIVIA_CATEGORY AS TC ON TC.ID = C.CATEGORY_ID
		WHERE C.DECK_ID = ?
			AND C.TEXT LIKE ?
		ORDER BY C.CARD_YEAR, C.TEXT
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
			&card.Year,
			&card.CategoryId,
			&card.CategoryName); err != nil {
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
			C.ID,
			C.CREATED_ON_DATE,
			C.CHANGED_ON_DATE,
			C.DECK_ID,
			C.TEXT,
			C.CARD_YEAR,
			C.CATEGORY_ID,
			TC.NAME
		FROM CARD AS C
			LEFT JOIN TIMELINE_TRIVIA_CATEGORY AS TC ON TC.ID = C.CATEGORY_ID
		WHERE C.ID = ?
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
			&card.Year,
			&card.CategoryId,
			&card.CategoryName); err != nil {
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

func CreateCard(deckId uuid.UUID, text string, year sql.NullInt64, categoryId uuid.NullUUID) (uuid.UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return id, errors.New("failed to generate new id")
	}

	sqlString := `
		INSERT INTO CARD(ID, DECK_ID, TEXT, CARD_YEAR, CATEGORY_ID)
		VALUES (?, ?, ?, ?, ?)
	`
	return id, execute(sqlString, id, deckId, text, year, categoryId)
}

func UpdateCard(id uuid.UUID, text string, year sql.NullInt64, categoryId uuid.NullUUID) error {
	sqlString := `
		UPDATE CARD
		SET TEXT = ?,
			CARD_YEAR = ?,
			CATEGORY_ID = ?
		WHERE ID = ?
	`
	return execute(sqlString, text, year, categoryId, id)
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
		INSERT INTO AUDIT_CARD(AUDIT_TYPE, CARD_ID, DECK_ID, TEXT, CARD_YEAR, CATEGORY_ID)
		SELECT 'DELETE', ID, DECK_ID, TEXT, CARD_YEAR, CATEGORY_ID
		FROM CARD
		WHERE DECK_ID = ?
	`
	return execute(sqlString, deckId)
}

// GetCardsInDeckExport returns a deck's cards for CSV export (text, year,
// category name).
func GetCardsInDeckExport(deckId uuid.UUID) ([]Card, error) {
	sqlString := `
		SELECT
			C.TEXT,
			C.CARD_YEAR,
			TC.NAME
		FROM CARD AS C
			LEFT JOIN TIMELINE_TRIVIA_CATEGORY AS TC ON TC.ID = C.CATEGORY_ID
		WHERE C.DECK_ID = ?
		ORDER BY C.CARD_YEAR, C.TEXT
	`
	rows, err := query(sqlString, deckId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Card, 0)
	for rows.Next() {
		var card Card
		if err := rows.Scan(&card.Text, &card.Year, &card.CategoryName); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, card)
	}
	return result, nil
}
