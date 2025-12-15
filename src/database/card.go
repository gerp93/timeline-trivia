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

	DeckId   uuid.UUID
	Category string
	Text     string
	YouTube  sql.NullString
	Image    sql.NullString
}

func SearchCardsInDeck(deckId uuid.UUID, category string, text string, page int) ([]Card, error) {
	if category == "" {
		category = "%"
	}

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
			CATEGORY,
			TEXT,
			YOUTUBE,
			IMAGE
		FROM CARD
		WHERE DECK_ID = ?
			AND CATEGORY LIKE ?
			AND TEXT LIKE ?
		ORDER BY CATEGORY, TEXT
		LIMIT 10 OFFSET ?
	`
	rows, err := query(sqlString, deckId, category, text, (page-1)*10)
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
			&card.Category,
			&card.Text,
			&card.YouTube,
			&card.Image); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, card)
	}
	return result, nil
}

func CountCardsInDeck(deckId uuid.UUID, category string, text string) (int, error) {
	if category == "" {
		category = "%"
	}

	text = "%" + text + "%"

	sqlString := `
		SELECT
			COUNT(*)
		FROM CARD
		WHERE DECK_ID = ?
			AND CATEGORY LIKE ?
			AND TEXT LIKE ?
	`
	rows, err := query(sqlString, deckId, category, text)
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
