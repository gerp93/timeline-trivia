package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
)

// MinDecadeGuesses is how many guesses a user must have made on cards in a
// decade before that decade's success rate is considered statistically
// meaningful and shown in the "most/least successful decade" rankings.
const MinDecadeGuesses = 10

// readableDeckPredicate filters to decks the viewer is allowed to read: a
// public deck, one they've been explicitly granted, or any deck if they're an
// admin — and never a hidden deck. It assumes DECK is joined as D and CARD as
// C, and takes the viewer's user id as two positional parameters (one for the
// grant check, one for the admin check). This is the "readable" notion from
// the framework's SP_GET_READABLE_DECKS, which (unlike FN_USER_HAS_DECK_ACCESS)
// includes public decks — the right filter for stats.
const readableDeckPredicate = `
	D.IS_HIDDEN = 0
	AND (
		D.IS_PUBLIC_READONLY = 1
		OR EXISTS (SELECT 1 FROM USER_ACCESS_DECK UAD WHERE UAD.DECK_ID = D.ID AND UAD.USER_ID = ?)
		OR EXISTS (SELECT 1 FROM USER U WHERE U.ID = ? AND U.IS_ADMIN = 1)
	)
`

// UserCanReadDeck reports whether a viewer may read a deck (public, granted, or
// admin, and not hidden) — used to gate the per-card stats page.
func UserCanReadDeck(viewerId uuid.UUID, deckId uuid.UUID) (bool, error) {
	sqlString := `
		SELECT COUNT(*)
		FROM DECK AS D
		WHERE D.ID = ?
			AND ` + readableDeckPredicate
	rows, err := query(sqlString, deckId, viewerId, viewerId)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			log.Println(err)
			return false, errors.New("failed to scan row in query results")
		}
	}
	return count > 0, nil
}

// StatUser is a user's overall play totals (scoped to the viewer's readable
// decks, except GamesWon which is global — a win isn't tied to a deck).
type StatUser struct {
	UserId         uuid.UUID
	Name           string
	TotalGuesses   int
	CorrectGuesses int
	GamesWon       int
}

func (s StatUser) Accuracy() float64 {
	if s.TotalGuesses == 0 {
		return 0
	}
	return float64(s.CorrectGuesses) / float64(s.TotalGuesses) * 100
}

// DecadeStat is a user's guessing record for one decade.
type DecadeStat struct {
	Decade   int
	Attempts int
	Correct  int
}

func (d DecadeStat) Rate() float64 {
	if d.Attempts == 0 {
		return 0
	}
	return float64(d.Correct) / float64(d.Attempts) * 100
}

// Qualified reports whether this decade has enough guesses for its success
// rate to be considered statistically meaningful.
func (d DecadeStat) Qualified() bool {
	return d.Attempts >= MinDecadeGuesses
}

// Label renders the decade for display, e.g. "1920s" or "50s B.C.E".
func (d DecadeStat) Label() string {
	if d.Decade < 0 {
		return fmt.Sprintf("%ds B.C.E", -d.Decade)
	}
	return fmt.Sprintf("%ds", d.Decade)
}

// CategoryStat is a user's guessing record for one card category.
type CategoryStat struct {
	Name     string
	Attempts int
	Correct  int
}

func (c CategoryStat) Rate() float64 {
	if c.Attempts == 0 {
		return 0
	}
	return float64(c.Correct) / float64(c.Attempts) * 100
}

// GetUserStatTotals returns a user's overall totals, scoped to the viewer's
// readable decks (games won is global).
func GetUserStatTotals(viewerId uuid.UUID, targetId uuid.UUID) (StatUser, error) {
	var result StatUser
	result.UserId = targetId

	totalsSQL := `
		SELECT
			COUNT(*) AS TOTAL,
			COALESCE(SUM(LG.IS_CORRECT), 0) AS CORRECT
		FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
			INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
		WHERE LG.USER_ID = ?
			AND ` + readableDeckPredicate
	rows, err := query(totalsSQL, targetId, viewerId, viewerId)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		if err := rows.Scan(&result.TotalGuesses, &result.CorrectGuesses); err != nil {
			rows.Close()
			log.Println(err)
			return result, errors.New("failed to scan row in query results")
		}
	}
	rows.Close()

	winsSQL := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_LOG_WIN WHERE USER_ID = ?`
	winRows, err := query(winsSQL, targetId)
	if err != nil {
		return result, err
	}
	defer winRows.Close()
	for winRows.Next() {
		if err := winRows.Scan(&result.GamesWon); err != nil {
			log.Println(err)
			return result, errors.New("failed to scan row in query results")
		}
	}

	return result, nil
}

// GetUserDecadeStats returns a user's per-decade guessing record over the
// viewer's readable decks, ordered by decade.
func GetUserDecadeStats(viewerId uuid.UUID, targetId uuid.UUID) ([]DecadeStat, error) {
	sqlString := `
		SELECT
			FLOOR(LG.CARD_YEAR / 10) * 10 AS DECADE,
			COUNT(*) AS ATTEMPTS,
			COALESCE(SUM(LG.IS_CORRECT), 0) AS CORRECT
		FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
			INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
		WHERE LG.USER_ID = ?
			AND ` + readableDeckPredicate + `
		GROUP BY DECADE
		ORDER BY DECADE
	`
	rows, err := query(sqlString, targetId, viewerId, viewerId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]DecadeStat, 0)
	for rows.Next() {
		var d DecadeStat
		if err := rows.Scan(&d.Decade, &d.Attempts, &d.Correct); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, d)
	}
	return result, nil
}

// GetUserCategoryStats returns a user's per-category guessing record over the
// viewer's readable decks, ordered by category name. Cards with no category
// are excluded.
func GetUserCategoryStats(viewerId uuid.UUID, targetId uuid.UUID) ([]CategoryStat, error) {
	sqlString := `
		SELECT
			TC.NAME,
			COUNT(*) AS ATTEMPTS,
			COALESCE(SUM(LG.IS_CORRECT), 0) AS CORRECT
		FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
			INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			INNER JOIN TIMELINE_TRIVIA_CATEGORY AS TC ON TC.ID = C.CATEGORY_ID
		WHERE LG.USER_ID = ?
			AND ` + readableDeckPredicate + `
		GROUP BY TC.ID, TC.NAME
		ORDER BY TC.NAME
	`
	rows, err := query(sqlString, targetId, viewerId, viewerId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]CategoryStat, 0)
	for rows.Next() {
		var c CategoryStat
		if err := rows.Scan(&c.Name, &c.Attempts, &c.Correct); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, c)
	}
	return result, nil
}

// LeaderboardEntry is one user's row on the cross-user leaderboard. Guess
// totals are over public decks only (so the ranking is identical for everyone
// and never exposes private-deck play); games won is global.
type LeaderboardEntry struct {
	Rank           int
	UserId         uuid.UUID
	Name           string
	GamesWon       int
	TotalGuesses   int
	CorrectGuesses int
}

func (e LeaderboardEntry) Accuracy() float64 {
	if e.TotalGuesses == 0 {
		return 0
	}
	return float64(e.CorrectGuesses) / float64(e.TotalGuesses) * 100
}

// GetLeaderboard returns every user with any recorded play, ranked by games
// won then correct guesses. Guess counts are restricted to public decks.
func GetLeaderboard() ([]LeaderboardEntry, error) {
	sqlString := `
		SELECT * FROM (
			SELECT
				U.ID AS USER_ID,
				U.NAME AS NAME,
				(SELECT COUNT(*) FROM TIMELINE_TRIVIA_LOG_WIN AS LW WHERE LW.USER_ID = U.ID) AS GAMES_WON,
				(
					SELECT COUNT(*)
					FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
						INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
						INNER JOIN DECK AS D ON D.ID = C.DECK_ID
					WHERE LG.USER_ID = U.ID
						AND D.IS_HIDDEN = 0
						AND D.IS_PUBLIC_READONLY = 1
				) AS TOTAL_GUESSES,
				(
					SELECT COALESCE(SUM(LG.IS_CORRECT), 0)
					FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
						INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
						INNER JOIN DECK AS D ON D.ID = C.DECK_ID
					WHERE LG.USER_ID = U.ID
						AND D.IS_HIDDEN = 0
						AND D.IS_PUBLIC_READONLY = 1
				) AS CORRECT_GUESSES
			FROM USER AS U
		) AS T
		WHERE T.GAMES_WON > 0 OR T.TOTAL_GUESSES > 0
		ORDER BY T.GAMES_WON DESC, T.CORRECT_GUESSES DESC, T.NAME
		LIMIT 100
	`
	rows, err := query(sqlString)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]LeaderboardEntry, 0)
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(&e.UserId, &e.Name, &e.GamesWon, &e.TotalGuesses, &e.CorrectGuesses); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		e.Rank = len(result) + 1
		result = append(result, e)
	}
	return result, nil
}

// StatCard is the play record for a single card.
type StatCard struct {
	DeckName     string
	CategoryName sql.NullString
	Text         string
	Year         sql.NullInt64
	DrawCount    int
	WrongCount   int
	DiscardCount int
}

// YearLabel renders the card's year (BCE-aware), or "Unknown" if unset.
func (c StatCard) YearLabel() string {
	if !c.Year.Valid {
		return "Unknown"
	}
	return FormatYear(int(c.Year.Int64))
}

// GetCardStats returns the play record for one card: how often it was drawn as
// the event to guess, how many wrong guesses it drew, and how often it was
// discarded because everyone missed.
func GetCardStats(cardId uuid.UUID) (StatCard, error) {
	var result StatCard

	sqlString := `
		SELECT
			D.NAME AS DECK_NAME,
			TC.NAME AS CATEGORY_NAME,
			C.TEXT,
			C.CARD_YEAR,
			(SELECT COUNT(*) FROM TIMELINE_TRIVIA_LOG_CARD WHERE CARD_ID = C.ID AND EVENT_TYPE = 'DRAW') AS DRAW_COUNT,
			(SELECT COUNT(*) FROM TIMELINE_TRIVIA_LOG_GUESS WHERE CARD_ID = C.ID AND IS_CORRECT = 0) AS WRONG_COUNT,
			(SELECT COUNT(*) FROM TIMELINE_TRIVIA_LOG_CARD WHERE CARD_ID = C.ID AND EVENT_TYPE = 'DISCARD') AS DISCARD_COUNT
		FROM CARD AS C
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			LEFT JOIN TIMELINE_TRIVIA_CATEGORY AS TC ON TC.ID = C.CATEGORY_ID
		WHERE C.ID = ?
	`
	rows, err := query(sqlString, cardId)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(
			&result.DeckName,
			&result.CategoryName,
			&result.Text,
			&result.Year,
			&result.DrawCount,
			&result.WrongCount,
			&result.DiscardCount,
		); err != nil {
			log.Println(err)
			return result, errors.New("failed to scan row in query results")
		}
	}

	return result, nil
}

// TopDecade is one row of the global "top decades that come up to be guessed"
// aggregate, by draw volume.
type TopDecade struct {
	Decade    int
	DrawCount int
}

func (t TopDecade) Label() string {
	if t.Decade < 0 {
		return fmt.Sprintf("%ds B.C.E", -t.Decade)
	}
	return fmt.Sprintf("%ds", t.Decade)
}

// GetTopDecades returns the decades whose cards come up to be guessed most
// often, over the viewer's readable decks.
func GetTopDecades(viewerId uuid.UUID) ([]TopDecade, error) {
	sqlString := `
		SELECT
			FLOOR(C.CARD_YEAR / 10) * 10 AS DECADE,
			COUNT(*) AS DRAW_COUNT
		FROM TIMELINE_TRIVIA_LOG_CARD AS LC
			INNER JOIN CARD AS C ON C.ID = LC.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
		WHERE LC.EVENT_TYPE = 'DRAW'
			AND ` + readableDeckPredicate + `
		GROUP BY DECADE
		ORDER BY DRAW_COUNT DESC, DECADE
		LIMIT 15
	`
	rows, err := query(sqlString, viewerId, viewerId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TopDecade, 0)
	for rows.Next() {
		var t TopDecade
		if err := rows.Scan(&t.Decade, &t.DrawCount); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, t)
	}
	return result, nil
}

// StatCardRow is one entry in the card picker on the stats pages.
type StatCardRow struct {
	Id           uuid.UUID
	Text         string
	Year         sql.NullInt64
	DeckName     string
	CategoryName sql.NullString
}

// YearLabel renders the card's year (BCE-aware), or "Unknown" if unset.
func (c StatCardRow) YearLabel() string {
	if !c.Year.Valid {
		return "Unknown"
	}
	return FormatYear(int(c.Year.Int64))
}

// CountStatCardsWithAccess counts cards in the viewer's readable decks matching
// a text search, for the card-picker pagination.
func CountStatCardsWithAccess(viewerId uuid.UUID, text string) (int, error) {
	text = "%" + text + "%"
	sqlString := `
		SELECT COUNT(*)
		FROM CARD AS C
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
		WHERE C.TEXT LIKE ?
			AND ` + readableDeckPredicate
	rows, err := query(sqlString, text, viewerId, viewerId)
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

// SearchStatCardsWithAccess returns a page of cards in the viewer's readable
// decks matching a text search, for the card picker.
func SearchStatCardsWithAccess(viewerId uuid.UUID, text string, page int) ([]StatCardRow, error) {
	text = "%" + text + "%"
	if page < 1 {
		page = 1
	}
	sqlString := `
		SELECT
			C.ID,
			C.TEXT,
			C.CARD_YEAR,
			D.NAME,
			TC.NAME
		FROM CARD AS C
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			LEFT JOIN TIMELINE_TRIVIA_CATEGORY AS TC ON TC.ID = C.CATEGORY_ID
		WHERE C.TEXT LIKE ?
			AND ` + readableDeckPredicate + `
		ORDER BY C.CARD_YEAR, C.TEXT
		LIMIT 10 OFFSET ?
	`
	rows, err := query(sqlString, text, viewerId, viewerId, (page-1)*10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]StatCardRow, 0)
	for rows.Next() {
		var c StatCardRow
		if err := rows.Scan(&c.Id, &c.Text, &c.Year, &c.DeckName, &c.CategoryName); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, c)
	}
	return result, nil
}

// GetCardDeckId returns a card's deck id, for gating the per-card stats page.
func GetCardDeckId(cardId uuid.UUID) (uuid.UUID, error) {
	var deckId uuid.UUID
	sqlString := `SELECT DECK_ID FROM CARD WHERE ID = ?`
	rows, err := query(sqlString, cardId)
	if err != nil {
		return deckId, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&deckId); err != nil {
			log.Println(err)
			return deckId, errors.New("failed to scan row in query results")
		}
	}
	return deckId, nil
}
