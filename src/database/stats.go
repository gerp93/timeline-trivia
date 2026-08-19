package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"

	"github.com/google/uuid"
)

// MinBucketGuesses is how many guesses a user must have made on cards in a
// decade or era before that bucket's success rate is considered
// statistically meaningful and shown in the "most/least successful"
// rankings.
const MinBucketGuesses = 10

// decadeExpr buckets a year column into its decade. It truncates toward zero
// rather than using FLOOR, which matters only for B.C.E. (negative) years:
// FLOOR(-55/10)*10 is -60, labelling 55 B.C.E as "60s B.C.E" when the 50s
// B.C.E decade is 50-59 B.C.E — one decade too old, and it merged years from
// two real decades into one bucket. TRUNCATE gives -50, and is identical to
// FLOOR for C.E. years. Callers substitute their own column name.
const decadeExpr = `TRUNCATE(%s / 10, 0) * 10`

// readableDeckPredicate filters to decks the viewer is allowed to read: a
// public deck, one they've been explicitly granted, or any deck if they're an
// admin. It assumes DECK is joined as D and CARD as C, and takes the viewer's
// user id as two positional parameters (one for the grant check, one for the
// admin check). This is the "readable" notion from the framework's
// SP_GET_READABLE_DECKS, which (unlike FN_USER_HAS_DECK_ACCESS) includes public
// decks — the right filter for stats.
const readableDeckPredicate = `
	(
		D.IS_PUBLIC_READONLY = 1
		OR EXISTS (SELECT 1 FROM USER_ACCESS_DECK UAD WHERE UAD.DECK_ID = D.ID AND UAD.USER_ID = ?)
		OR EXISTS (SELECT 1 FROM USER U WHERE U.ID = ? AND U.IS_ADMIN = 1)
	)
`

// appendTimelineFilter appends a timeline-scoping predicate to sqlString and
// its args, restricting to one timeline; a uuid.Nil timelineId is a no-op
// ("Overall" — today's existing, unfiltered behavior). Assumes the query
// already has `LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID =
// D.ID` — a deck with no explicit row there belongs to DefaultTimelineId,
// the same lazy-default convention GetDeckTimelineId uses.
func appendTimelineFilter(sqlString string, args []interface{}, timelineId uuid.UUID) (string, []interface{}) {
	if timelineId == uuid.Nil {
		return sqlString, args
	}
	return sqlString + " AND COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) = ?", append(args, DefaultTimelineId, timelineId)
}

// appendEraFilter appends a predicate restricting a query's snapshotted
// LG.CARD_YEAR column to one era's [FromYear, ToYear] range (an unset bound
// is open-ended); a uuid.Nil eraId is a no-op (no further narrowing beyond
// whatever timeline filter is already applied). Only meaningful alongside
// LOG_GUESS-based queries, which alias that table LG.
func appendEraFilter(sqlString string, args []interface{}, eraId uuid.UUID) (string, []interface{}, error) {
	if eraId == uuid.Nil {
		return sqlString, args, nil
	}
	era, err := GetEra(eraId)
	if err != nil {
		return sqlString, args, err
	}
	if era.FromYear.Valid {
		sqlString += " AND LG.CARD_YEAR >= ?"
		args = append(args, era.FromYear.Int64)
	}
	if era.ToYear.Valid {
		sqlString += " AND LG.CARD_YEAR <= ?"
		args = append(args, era.ToYear.Int64)
	}
	return sqlString, args, nil
}

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

// StatUser is a user's overall play totals, scoped to the viewer's readable
// decks (and, per GetUserStatTotals's params, optionally to one timeline).
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
	return d.Attempts >= MinBucketGuesses
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
// readable decks. timelineId (uuid.Nil = "Overall", no filter) additionally
// scopes both guesses and games won to one timeline; eraId (uuid.Nil = whole
// timeline) further narrows guesses to one era within it — games won isn't
// further narrowed by era, a win isn't tied to a single card year.
func GetUserStatTotals(viewerId uuid.UUID, targetId uuid.UUID, timelineId uuid.UUID, eraId uuid.UUID) (StatUser, error) {
	var result StatUser
	result.UserId = targetId

	totalsSQL := `
		SELECT
			COUNT(*) AS TOTAL,
			COALESCE(SUM(LG.IS_CORRECT), 0) AS CORRECT
		FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
			INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
		WHERE LG.USER_ID = ?
			AND ` + readableDeckPredicate
	totalsArgs := []interface{}{targetId, viewerId, viewerId}
	totalsSQL, totalsArgs = appendTimelineFilter(totalsSQL, totalsArgs, timelineId)
	totalsSQL, totalsArgs, err := appendEraFilter(totalsSQL, totalsArgs, eraId)
	if err != nil {
		return result, err
	}

	rows, err := query(totalsSQL, totalsArgs...)
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
	winArgs := []interface{}{targetId}
	if timelineId != uuid.Nil {
		winsSQL += ` AND TIMELINE_TRIVIA_TIMELINE_ID = ?`
		winArgs = append(winArgs, timelineId)
	}
	winRows, err := query(winsSQL, winArgs...)
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
// viewer's readable decks, ordered by decade. This is the "Overall"
// breakdown (no timeline filter) — see GetUserEraStats for the per-timeline
// equivalent shown once a specific timeline is selected.
func GetUserDecadeStats(viewerId uuid.UUID, targetId uuid.UUID) ([]DecadeStat, error) {
	sqlString := `
		SELECT
			` + fmt.Sprintf(decadeExpr, "LG.CARD_YEAR") + ` AS DECADE,
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

// EraStat is a user's guessing record for one era of a timeline — the
// per-timeline counterpart to DecadeStat, shown once a specific timeline is
// selected instead of the Overall decade breakdown.
type EraStat struct {
	EraId        uuid.UUID
	Name         string
	Abbreviation string
	Attempts     int
	Correct      int
}

func (e EraStat) Rate() float64 {
	if e.Attempts == 0 {
		return 0
	}
	return float64(e.Correct) / float64(e.Attempts) * 100
}

// Qualified reports whether this era has enough guesses for its success
// rate to be considered statistically meaningful.
func (e EraStat) Qualified() bool {
	return e.Attempts >= MinBucketGuesses
}

// Label renders the era for display, e.g. "Third Age (T.A.)".
func (e EraStat) Label() string {
	if e.Abbreviation != "" {
		return e.Name + " (" + e.Abbreviation + ")"
	}
	return e.Name
}

// GetUserEraStats returns a user's per-era guessing record within one
// timeline, in the timeline's own earliest-to-latest order. Bucketing
// happens in Go (via FindEraForYear) rather than SQL, since era ranges are
// admin-defined per timeline and don't reduce to one formula the way
// decades do. A guess whose year falls in no era is omitted from the
// result — GetUserStatTotals's overall count is unaffected, only this
// breakdown. Only eras with at least one guess are returned, matching
// GetUserDecadeStats's GROUP BY behavior.
func GetUserEraStats(viewerId uuid.UUID, targetId uuid.UUID, timelineId uuid.UUID) ([]EraStat, error) {
	eras, err := GetErasForTimeline(timelineId)
	if err != nil {
		return nil, err
	}

	sqlString := `
		SELECT LG.CARD_YEAR, LG.IS_CORRECT
		FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
			INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
		WHERE LG.USER_ID = ?
			AND ` + readableDeckPredicate + `
			AND COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) = ?
	`
	rows, err := query(sqlString, targetId, viewerId, viewerId, DefaultTimelineId, timelineId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[uuid.UUID]*EraStat, len(eras))
	for _, e := range eras {
		counts[e.Id] = &EraStat{EraId: e.Id, Name: e.Name, Abbreviation: e.Abbreviation}
	}
	for rows.Next() {
		var year int
		var correct bool
		if err := rows.Scan(&year, &correct); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		era, ok := FindEraForYear(year, eras)
		if !ok {
			continue
		}
		stat := counts[era.Id]
		stat.Attempts++
		if correct {
			stat.Correct++
		}
	}

	result := make([]EraStat, 0, len(eras))
	for _, e := range eras {
		if stat := counts[e.Id]; stat.Attempts > 0 {
			result = append(result, *stat)
		}
	}
	return result, nil
}

// GetUserCategoryStats returns a user's per-category guessing record over the
// viewer's readable decks, ordered by category name. Cards with no category
// are excluded. timelineId/eraId scope it the same way GetUserStatTotals
// does.
func GetUserCategoryStats(viewerId uuid.UUID, targetId uuid.UUID, timelineId uuid.UUID, eraId uuid.UUID) ([]CategoryStat, error) {
	sqlString := `
		SELECT
			TC.NAME,
			COUNT(*) AS ATTEMPTS,
			COALESCE(SUM(LG.IS_CORRECT), 0) AS CORRECT
		FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
			INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			INNER JOIN TIMELINE_TRIVIA_CATEGORY AS TC ON TC.ID = C.CATEGORY_ID
			LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
		WHERE LG.USER_ID = ?
			AND ` + readableDeckPredicate
	args := []interface{}{targetId, viewerId, viewerId}
	sqlString, args = appendTimelineFilter(sqlString, args, timelineId)
	sqlString, args, err := appendEraFilter(sqlString, args, eraId)
	if err != nil {
		return nil, err
	}
	sqlString += `
		GROUP BY TC.ID, TC.NAME
		ORDER BY TC.NAME
	`
	rows, err := query(sqlString, args...)
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

// TimeoutStat is how many turns a user lost to the clock at one particular
// turn-timer setting. A lobby's timer can be changed mid-game, so the same
// user can have rows at several durations.
type TimeoutStat struct {
	TimerSeconds int
	Count        int
}

// UserTimeoutStats is a user's record of turns lost to the turn timer: the
// total, plus the breakdown by how long the timer was set to at the time.
type UserTimeoutStats struct {
	Total      int
	ByDuration []TimeoutStat
}

// GetUserTimeoutStats returns how often a user ran out of time, broken down by
// the turn-timer setting in force for each occurrence, over the viewer's
// readable decks (same scoping as the guess stats it sits beside).
// timelineId (uuid.Nil = "Overall") scopes it to one timeline.
func GetUserTimeoutStats(viewerId uuid.UUID, targetId uuid.UUID, timelineId uuid.UUID) (UserTimeoutStats, error) {
	var result UserTimeoutStats

	sqlString := `
		SELECT
			LT.TIMER_SECONDS,
			COUNT(*) AS TIMEOUT_COUNT
		FROM TIMELINE_TRIVIA_LOG_TIMEOUT AS LT
			INNER JOIN CARD AS C ON C.ID = LT.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
		WHERE LT.USER_ID = ?
			AND ` + readableDeckPredicate
	args := []interface{}{targetId, viewerId, viewerId}
	sqlString, args = appendTimelineFilter(sqlString, args, timelineId)
	sqlString += `
		GROUP BY LT.TIMER_SECONDS
		ORDER BY LT.TIMER_SECONDS
	`
	rows, err := query(sqlString, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	result.ByDuration = make([]TimeoutStat, 0)
	for rows.Next() {
		var t TimeoutStat
		if err := rows.Scan(&t.TimerSeconds, &t.Count); err != nil {
			log.Println(err)
			return result, errors.New("failed to scan row in query results")
		}
		result.Total += t.Count
		result.ByDuration = append(result.ByDuration, t)
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
// timelineId (uuid.Nil = "Overall") scopes both games won and guesses to one
// timeline.
func GetLeaderboard(timelineId uuid.UUID) ([]LeaderboardEntry, error) {
	winsFilter := ""
	guessFilter := ""
	args := make([]interface{}, 0, 5)
	if timelineId != uuid.Nil {
		winsFilter = "AND LW.TIMELINE_TRIVIA_TIMELINE_ID = ?"
		guessFilter = "AND COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) = ?"
	}

	sqlString := `
		SELECT * FROM (
			SELECT
				U.ID AS USER_ID,
				U.NAME AS NAME,
				(SELECT COUNT(*) FROM TIMELINE_TRIVIA_LOG_WIN AS LW WHERE LW.USER_ID = U.ID ` + winsFilter + `) AS GAMES_WON,
				(
					SELECT COUNT(*)
					FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
						INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
						INNER JOIN DECK AS D ON D.ID = C.DECK_ID
						LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
					WHERE LG.USER_ID = U.ID
						AND D.IS_PUBLIC_READONLY = 1
						` + guessFilter + `
				) AS TOTAL_GUESSES,
				(
					SELECT COALESCE(SUM(LG.IS_CORRECT), 0)
					FROM TIMELINE_TRIVIA_LOG_GUESS AS LG
						INNER JOIN CARD AS C ON C.ID = LG.CARD_ID
						INNER JOIN DECK AS D ON D.ID = C.DECK_ID
						LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
					WHERE LG.USER_ID = U.ID
						AND D.IS_PUBLIC_READONLY = 1
						` + guessFilter + `
				) AS CORRECT_GUESSES
			FROM USER AS U
		) AS T
		WHERE T.GAMES_WON > 0 OR T.TOTAL_GUESSES > 0
		ORDER BY T.GAMES_WON DESC, T.CORRECT_GUESSES DESC, T.NAME
		LIMIT 100
	`
	if timelineId != uuid.Nil {
		args = append(args, timelineId, DefaultTimelineId, timelineId, DefaultTimelineId, timelineId)
	}
	rows, err := query(sqlString, args...)
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
		// Competition ranking: users tied on the sort keys share a rank
		// rather than being handed arbitrary sequential numbers by row order.
		e.Rank = len(result) + 1
		if n := len(result); n > 0 {
			prev := result[n-1]
			if prev.GamesWon == e.GamesWon && prev.CorrectGuesses == e.CorrectGuesses {
				e.Rank = prev.Rank
			}
		}
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
// often, over the viewer's readable decks. This is the "Overall" breakdown
// (no timeline filter) — see GetTopErasForTimeline for the per-timeline
// equivalent shown once a specific timeline is selected.
func GetTopDecades(viewerId uuid.UUID) ([]TopDecade, error) {
	sqlString := `
		SELECT
			` + fmt.Sprintf(decadeExpr, "C.CARD_YEAR") + ` AS DECADE,
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

// TopEra is one row of the "top eras that come up to be guessed" aggregate
// for one timeline, by draw volume — the per-timeline counterpart to
// TopDecade.
type TopEra struct {
	EraId        uuid.UUID
	Name         string
	Abbreviation string
	DrawCount    int
}

// Label renders the era for display, e.g. "Third Age (T.A.)".
func (t TopEra) Label() string {
	if t.Abbreviation != "" {
		return t.Name + " (" + t.Abbreviation + ")"
	}
	return t.Name
}

// GetTopErasForTimeline returns a timeline's eras whose cards come up to be
// guessed most often, over the viewer's readable decks, ranked by draw
// count descending. Bucketing happens in Go (via FindEraForYear), same
// rationale as GetUserEraStats.
func GetTopErasForTimeline(viewerId uuid.UUID, timelineId uuid.UUID) ([]TopEra, error) {
	eras, err := GetErasForTimeline(timelineId)
	if err != nil {
		return nil, err
	}

	sqlString := `
		SELECT C.CARD_YEAR
		FROM TIMELINE_TRIVIA_LOG_CARD AS LC
			INNER JOIN CARD AS C ON C.ID = LC.CARD_ID
			INNER JOIN DECK AS D ON D.ID = C.DECK_ID
			LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
		WHERE LC.EVENT_TYPE = 'DRAW'
			AND ` + readableDeckPredicate + `
			AND COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) = ?
	`
	rows, err := query(sqlString, viewerId, viewerId, DefaultTimelineId, timelineId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[uuid.UUID]*TopEra, len(eras))
	for _, e := range eras {
		counts[e.Id] = &TopEra{EraId: e.Id, Name: e.Name, Abbreviation: e.Abbreviation}
	}
	for rows.Next() {
		var year int
		if err := rows.Scan(&year); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		era, ok := FindEraForYear(year, eras)
		if !ok {
			continue
		}
		counts[era.Id].DrawCount++
	}

	result := make([]TopEra, 0, len(eras))
	for _, e := range eras {
		if t := counts[e.Id]; t.DrawCount > 0 {
			result = append(result, *t)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].DrawCount > result[j].DrawCount })
	if len(result) > 15 {
		result = result[:15]
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
