package database

import (
	"github.com/google/uuid"
)

// This file holds the append-only gameplay logging used by the stats pages.
// The three TIMELINE_TRIVIA_LOG_* tables have no foreign keys on purpose: the
// game/lobby/player rows they reference cascade away when a lobby's last
// websocket client disconnects, but stats must outlive individual games. Rows
// are joined back to CARD by CARD_ID at query time (a deleted card simply
// drops out of stats). Logging failures are non-fatal to gameplay — the
// callers log and continue rather than failing a player's turn.

// LogGuess records a single card-placement attempt. CARD_YEAR is snapshotted
// (it's the answer for that guess) so decade breakdowns stay stable even if
// the card is later edited.
func LogGuess(userId uuid.UUID, cardId uuid.UUID, cardYear int, isCorrect bool) error {
	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_LOG_GUESS(USER_ID, CARD_ID, CARD_YEAR, IS_CORRECT)
		VALUES (?, ?, ?, ?)
	`
	return execute(sqlString, userId, cardId, cardYear, isCorrect)
}

// LogCardDraw records that a card became the event to guess.
func LogCardDraw(cardId uuid.UUID) error {
	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_LOG_CARD(CARD_ID, EVENT_TYPE)
		VALUES (?, 'DRAW')
	`
	return execute(sqlString, cardId)
}

// LogCardDiscard records that a card was discarded because every active player
// missed it.
func LogCardDiscard(cardId uuid.UUID) error {
	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_LOG_CARD(CARD_ID, EVENT_TYPE)
		VALUES (?, 'DISCARD')
	`
	return execute(sqlString, cardId)
}

// LogWin records that a user won a game.
func LogWin(userId uuid.UUID) error {
	sqlString := `INSERT INTO TIMELINE_TRIVIA_LOG_WIN(USER_ID) VALUES (?)`
	return execute(sqlString, userId)
}
