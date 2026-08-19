package database

import (
	"database/sql"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// DefaultTimelineId is the well-known "Real Life" timeline every deck
// belongs to unless an admin explicitly reassigns it (see
// TIMELINE_TRIVIA_DECK_TIMELINE and GetDeckTimelineId). It's also the
// TIMELINE_TRIVIA_GAME.TIMELINE_TRIVIA_TIMELINE_ID default/backfill value —
// the literal here must match TIMELINE_TRIVIA_GAME.sql and
// MIG_TIMELINE_TRIVIA_GAME_ADD_TIMELINE_ID.sql.
var DefaultTimelineId = uuid.MustParse("9f9a1a00-d22a-11f0-b4d2-60cf84649547")

// defaultEraBCEId and defaultEraCEId are the two eras seeded under
// DefaultTimelineId, replicating the exact BCE/CE display behavior that
// existed before timelines/eras were introduced.
var defaultEraBCEId = uuid.MustParse("9f9a1a01-d22a-11f0-b4d2-60cf84649547")
var defaultEraCEId = uuid.MustParse("9f9a1a02-d22a-11f0-b4d2-60cf84649547")

// EraDirectionForward eras count up as the absolute year increases (e.g.
// Common Era, After Battle of Yavin, or any of Tolkien's Ages counting up
// from their own start). EraDirectionBackward eras count up as the
// absolute year decreases (e.g. Before Common Era, Before Battle of Yavin)
// — used for the half of a split-epoch convention that counts back into
// the past. See Era.EpochOffset and FormatYearInEras.
const (
	EraDirectionForward  = "FORWARD"
	EraDirectionBackward = "BACKWARD"
)

// Timeline is an alternate history/universe a lobby can be created against
// (e.g. "Real Life", "Star Wars").
type Timeline struct {
	Id            uuid.UUID
	CreatedOnDate time.Time
	Name          string
}

// Era is a named, ordered year-range label within one timeline (e.g.
// "Before Common Era"/"B.C.E"). Purely a display + admin-organization
// concern — see FormatYearInEras.
//
// EpochOffset is the absolute CARD_YEAR value equal to this era's own
// year 0; Direction (EraDirectionForward/EraDirectionBackward) says which
// way the era's own relative year counts from that point. Together they
// let an era represent either half of a split-epoch convention (B.C.E/C.E,
// BBY/ABY — both offset 0, opposite directions) or one era in a sequence
// that resets its counter at each boundary (Tolkien's Ages, Elder Scrolls'
// 1E/2E/3E/4E — all Forward, each with the prior era's ending absolute
// year as its offset).
type Era struct {
	Id            uuid.UUID
	CreatedOnDate time.Time
	TimelineId    uuid.UUID
	Name          string
	Abbreviation  string
	SortOrder     int
	FromYear      sql.NullInt64
	ToYear        sql.NullInt64
	EpochOffset   int
	Direction     string
}

// TimelineWithEras is one timeline plus its eras, its categories, and how
// many decks currently resolve to it, for the admin management page.
type TimelineWithEras struct {
	Timeline
	Eras       []Era
	Categories []CategoryWithCount
	DeckCount  int
}

// GetTimelines returns every timeline, oldest first — the seeded Real Life
// timeline is always created first, so it naturally sorts to the top.
func GetTimelines() ([]Timeline, error) {
	sqlString := `
		SELECT ID, CREATED_ON_DATE, NAME
		FROM TIMELINE_TRIVIA_TIMELINE
		ORDER BY CREATED_ON_DATE
	`
	rows, err := query(sqlString)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Timeline, 0)
	for rows.Next() {
		var t Timeline
		if err := rows.Scan(&t.Id, &t.CreatedOnDate, &t.Name); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, t)
	}
	return result, nil
}

// GetTimelinesWithEras returns every timeline with its eras (sorted by
// SORT_ORDER) and current deck count, for the /timelines admin page.
func GetTimelinesWithEras() ([]TimelineWithEras, error) {
	timelines, err := GetTimelines()
	if err != nil {
		return nil, err
	}

	result := make([]TimelineWithEras, 0, len(timelines))
	for _, t := range timelines {
		eras, err := GetErasForTimeline(t.Id)
		if err != nil {
			return nil, err
		}
		categories, err := GetCategoriesForTimelineWithCounts(t.Id)
		if err != nil {
			return nil, err
		}
		deckCount, err := countDecksForTimeline(t.Id)
		if err != nil {
			return nil, err
		}
		result = append(result, TimelineWithEras{Timeline: t, Eras: eras, Categories: categories, DeckCount: deckCount})
	}
	return result, nil
}

// countDecksForTimeline counts decks currently resolving to timelineId,
// including decks with no explicit TIMELINE_TRIVIA_DECK_TIMELINE row when
// timelineId is DefaultTimelineId (the lazy-default convention).
func countDecksForTimeline(timelineId uuid.UUID) (int, error) {
	sqlString := `
		SELECT COUNT(*)
		FROM DECK D
		LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE DT ON DT.DECK_ID = D.ID
		WHERE COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) = ?
	`
	rows, err := query(sqlString, DefaultTimelineId, timelineId)
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

func GetTimeline(id uuid.UUID) (Timeline, error) {
	var t Timeline

	sqlString := `
		SELECT ID, CREATED_ON_DATE, NAME
		FROM TIMELINE_TRIVIA_TIMELINE
		WHERE ID = ?
	`
	rows, err := query(sqlString, id)
	if err != nil {
		return t, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&t.Id, &t.CreatedOnDate, &t.Name); err != nil {
			log.Println(err)
			return t, errors.New("failed to scan row in query results")
		}
	}
	return t, nil
}

func TimelineExists(id uuid.UUID) (bool, error) {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_TIMELINE WHERE ID = ?`
	rows, err := query(sqlString, id)
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

func TimelineNameExists(name string) (bool, error) {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_TIMELINE WHERE NAME = ?`
	rows, err := query(sqlString, name)
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

func CreateTimeline(name string) (uuid.UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return id, errors.New("failed to generate new id")
	}

	sqlString := `INSERT INTO TIMELINE_TRIVIA_TIMELINE(ID, NAME) VALUES (?, ?)`
	return id, execute(sqlString, id, name)
}

// DeleteTimelineReassigning moves every deck explicitly assigned to
// deleteId over to targetId, and every one of deleteId's categories over to
// targetId as well (categories are a soft reference, not DB-FK-cascaded
// away like eras are), then deletes the timeline (its eras cascade away).
// The reassigns run first so a failed delete never leaves a deck or
// category pointing at a missing timeline. Callers must reject deleteId ==
// DefaultTimelineId before calling this — Real Life is never deletable.
// If targetId already has a category with the same name as one being moved,
// the composite unique constraint rejects the whole delete — a rare admin
// edge case left for the admin to resolve by renaming first, not silently
// handled here.
func DeleteTimelineReassigning(deleteId uuid.UUID, targetId uuid.UUID) error {
	if deleteId == targetId {
		return errors.New("cannot reassign a timeline to itself")
	}

	reassignDecksSQL := `UPDATE TIMELINE_TRIVIA_DECK_TIMELINE SET TIMELINE_TRIVIA_TIMELINE_ID = ? WHERE TIMELINE_TRIVIA_TIMELINE_ID = ?`
	if err := execute(reassignDecksSQL, targetId, deleteId); err != nil {
		return err
	}

	reassignCategoriesSQL := `UPDATE TIMELINE_TRIVIA_CATEGORY SET TIMELINE_TRIVIA_TIMELINE_ID = ? WHERE TIMELINE_TRIVIA_TIMELINE_ID = ?`
	if err := execute(reassignCategoriesSQL, targetId, deleteId); err != nil {
		return err
	}

	deleteSQL := `DELETE FROM TIMELINE_TRIVIA_TIMELINE WHERE ID = ?`
	return execute(deleteSQL, deleteId)
}

// eraSelectColumns is shared by GetErasForTimeline and GetEra so their
// SELECT list and Scan order can never drift apart.
const eraSelectColumns = `ID, CREATED_ON_DATE, TIMELINE_TRIVIA_TIMELINE_ID, NAME, ABBREVIATION, SORT_ORDER, FROM_YEAR, TO_YEAR, EPOCH_OFFSET, DIRECTION`

func scanEra(rows *sql.Rows, e *Era) error {
	return rows.Scan(&e.Id, &e.CreatedOnDate, &e.TimelineId, &e.Name, &e.Abbreviation, &e.SortOrder, &e.FromYear, &e.ToYear, &e.EpochOffset, &e.Direction)
}

// GetErasForTimeline returns a timeline's eras, earliest to latest.
func GetErasForTimeline(timelineId uuid.UUID) ([]Era, error) {
	sqlString := `
		SELECT ` + eraSelectColumns + `
		FROM TIMELINE_TRIVIA_ERA
		WHERE TIMELINE_TRIVIA_TIMELINE_ID = ?
		ORDER BY SORT_ORDER
	`
	rows, err := query(sqlString, timelineId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Era, 0)
	for rows.Next() {
		var e Era
		if err := scanEra(rows, &e); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, e)
	}
	return result, nil
}

// GetEra retrieves a single era by id.
func GetEra(id uuid.UUID) (Era, error) {
	var e Era

	sqlString := `
		SELECT ` + eraSelectColumns + `
		FROM TIMELINE_TRIVIA_ERA
		WHERE ID = ?
	`
	rows, err := query(sqlString, id)
	if err != nil {
		return e, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scanEra(rows, &e); err != nil {
			log.Println(err)
			return e, errors.New("failed to scan row in query results")
		}
	}
	return e, nil
}

func EraExists(id uuid.UUID) (bool, error) {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_ERA WHERE ID = ?`
	rows, err := query(sqlString, id)
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

func CreateEra(timelineId uuid.UUID, name string, abbreviation string, sortOrder int, fromYear sql.NullInt64, toYear sql.NullInt64, epochOffset int, direction string) (uuid.UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return id, errors.New("failed to generate new id")
	}

	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_ERA(ID, TIMELINE_TRIVIA_TIMELINE_ID, NAME, ABBREVIATION, SORT_ORDER, FROM_YEAR, TO_YEAR, EPOCH_OFFSET, DIRECTION)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	return id, execute(sqlString, id, timelineId, name, abbreviation, sortOrder, fromYear, toYear, epochOffset, direction)
}

// DeleteEra removes an era. No reassignment needed — eras aren't referenced
// by a foreign key from CARD. Blocked when this is the last era of a
// timeline that decks depend on (see SetDeckTimeline's matching check) —
// without this, a timeline could be emptied out from under decks already
// assigned to it, silently reintroducing the "deck with no era to author
// against" problem SetDeckTimeline exists to prevent. This applies to
// DefaultTimelineId too: any deck with no explicit assignment lazily
// depends on Real Life, so its eras can't be emptied out either as long as
// any deck (assigned or defaulted) exists.
// ErrEraNotFound and ErrEraInUse are sentinel errors DeleteEra returns for
// its two validation-rejection cases, distinguished from a genuine
// server/DB error so the API handler can respond 400/404 instead of 500.
var ErrEraNotFound = errors.New("era not found")
var ErrEraInUse = errors.New("this is the only era left in a timeline that decks depend on; reassign or delete those decks first, or add another era before removing this one")

func DeleteEra(id uuid.UUID) error {
	era, err := GetEra(id)
	if err != nil {
		return err
	}
	if era.Id == uuid.Nil {
		return ErrEraNotFound
	}

	siblings, err := GetErasForTimeline(era.TimelineId)
	if err != nil {
		return err
	}
	if len(siblings) <= 1 {
		deckCount, err := countDecksForTimeline(era.TimelineId)
		if err != nil {
			return err
		}
		if deckCount > 0 {
			return ErrEraInUse
		}
	}

	sqlString := `DELETE FROM TIMELINE_TRIVIA_ERA WHERE ID = ?`
	return execute(sqlString, id)
}

// GetDeckTimelineId returns the timeline a deck belongs to: its explicit
// TIMELINE_TRIVIA_DECK_TIMELINE assignment, or DefaultTimelineId if none.
func GetDeckTimelineId(deckId uuid.UUID) (uuid.UUID, error) {
	sqlString := `SELECT TIMELINE_TRIVIA_TIMELINE_ID FROM TIMELINE_TRIVIA_DECK_TIMELINE WHERE DECK_ID = ?`
	rows, err := query(sqlString, deckId)
	if err != nil {
		return uuid.Nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			log.Println(err)
			return uuid.Nil, errors.New("failed to scan row in query results")
		}
		return id, nil
	}
	return DefaultTimelineId, nil
}

// GetDeckTimelineIds is the batched form of GetDeckTimelineId, keyed by
// deck id. Decks with no explicit assignment resolve to DefaultTimelineId.
func GetDeckTimelineIds(deckIds []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	result := make(map[uuid.UUID]uuid.UUID, len(deckIds))
	if len(deckIds) == 0 {
		return result, nil
	}
	for _, id := range deckIds {
		result[id] = DefaultTimelineId
	}

	placeholders := ""
	args := make([]interface{}, 0, len(deckIds))
	for i, id := range deckIds {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}

	sqlString := `
		SELECT DECK_ID, TIMELINE_TRIVIA_TIMELINE_ID
		FROM TIMELINE_TRIVIA_DECK_TIMELINE
		WHERE DECK_ID IN (` + placeholders + `)
	`
	rows, err := query(sqlString, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var deckId, timelineId uuid.UUID
		if err := rows.Scan(&deckId, &timelineId); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result[deckId] = timelineId
	}
	return result, nil
}

// Sentinel errors ValidateTimelineAssignable returns for its
// validation-rejection cases, distinguished from a genuine server/DB error
// so a caller can respond 400 instead of 500 (via errors.Is) — the same
// treatment DeleteEra's/DeleteCategoryReassigning's sentinels get.
var ErrTimelineDoesNotExist = errors.New("selected timeline does not exist")
var ErrTimelineHasNoEras = errors.New("this timeline has no eras yet; add at least one era before assigning decks to it")
var ErrTimelineHasNoCategories = errors.New("this timeline has no categories yet; add at least one category before assigning decks to it")

// ValidateTimelineAssignable checks that timelineId names a real timeline
// with at least one era and at least one category — the invariant a deck
// must satisfy before it can be assigned there, since a card can't be
// authored without either. Shared by every path that assigns a deck to a
// timeline (apiTimeline.SetDeckTimeline's HTTP handler, and
// game.OnDeckCreated's deck-creation hook) so the rule can't drift between
// them.
func ValidateTimelineAssignable(timelineId uuid.UUID) error {
	exists, err := TimelineExists(timelineId)
	if err != nil {
		return err
	}
	if !exists {
		return ErrTimelineDoesNotExist
	}

	eras, err := GetErasForTimeline(timelineId)
	if err != nil {
		return err
	}
	if len(eras) == 0 {
		return ErrTimelineHasNoEras
	}

	categories, err := GetCategoriesForTimeline(timelineId)
	if err != nil {
		return err
	}
	if len(categories) == 0 {
		return ErrTimelineHasNoCategories
	}

	return nil
}

// SetDeckTimeline assigns a deck to a timeline, replacing any previous
// assignment.
func SetDeckTimeline(deckId uuid.UUID, timelineId uuid.UUID) error {
	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_DECK_TIMELINE(DECK_ID, TIMELINE_TRIVIA_TIMELINE_ID)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE TIMELINE_TRIVIA_TIMELINE_ID = VALUES(TIMELINE_TRIVIA_TIMELINE_ID)
	`
	return execute(sqlString, deckId, timelineId)
}

// EraContainsAbsoluteYear reports whether year falls within era's own
// [FromYear, ToYear] bounds — an unset bound is open-ended. Shared by the
// card-entry bounds check (manual entry in api/card and JSON import in
// this package) that rejects a year computed for one era but that actually
// falls outside it.
func EraContainsAbsoluteYear(year int, era Era) bool {
	if era.FromYear.Valid && int64(year) < era.FromYear.Int64 {
		return false
	}
	if era.ToYear.Valid && int64(year) > era.ToYear.Int64 {
		return false
	}
	return true
}

// FindEraForYear returns the first era in eras (all belonging to one
// timeline) whose range contains year. Shared by FormatYearInEras and by
// the card-entry UI's reverse lookup (an existing card's era + in-era
// year, for prefilling the edit dialog).
func FindEraForYear(year int, eras []Era) (Era, bool) {
	for _, e := range eras {
		if EraContainsAbsoluteYear(year, e) {
			return e, true
		}
	}
	return Era{}, false
}

// RelativeYearInEra converts an absolute CARD_YEAR into its era's own
// counting: how far year has advanced from the era's EpochOffset, in the
// direction the era counts. This is the inverse of AbsoluteYearFromEra.
func RelativeYearInEra(year int, era Era) int {
	if era.Direction == EraDirectionBackward {
		return era.EpochOffset - year
	}
	return year - era.EpochOffset
}

// AbsoluteYearFromEra converts an author-entered "year within this era"
// back into the absolute CARD_YEAR used for sorting/placement — the inverse
// of RelativeYearInEra. Used when a card is created/edited by picking an
// era and typing the era's own year (e.g. era=Fourth Age, year=201) rather
// than the raw absolute value.
func AbsoluteYearFromEra(era Era, relativeYear int) int {
	if era.Direction == EraDirectionBackward {
		return era.EpochOffset - relativeYear
	}
	return era.EpochOffset + relativeYear
}

// FormatYearInEras renders a card year using whichever era in eras (all
// belonging to one timeline) contains it: the year's own position within
// that era (see RelativeYearInEra), plus a space and the era's
// abbreviation when it has one. A year matching no era (a gap, or an empty
// era list) falls back to a bare magnitude with no label.
func FormatYearInEras(year int, eras []Era) string {
	if era, ok := FindEraForYear(year, eras); ok {
		relative := RelativeYearInEra(year, era)
		if era.Abbreviation == "" {
			return strconv.Itoa(relative)
		}
		return strconv.Itoa(relative) + " " + era.Abbreviation
	}

	magnitude := year
	if magnitude < 0 {
		magnitude = -magnitude
	}
	return strconv.Itoa(magnitude)
}

// DeckWithTimeline is one deck plus the timeline it belongs to, for the
// /decks page's timeline column. Field names deliberately match
// gsDatabase.DeckDetails's own (Id, Name, CardCount, IsPublicReadOnly) so
// the shared decks.html chrome renders it identically to the framework's
// own type without any changes there; TimelineId/TimelineName are only
// referenced by this game's own deck-list-extra-column block.
type DeckWithTimeline struct {
	Id               uuid.UUID
	Name             string
	CardCount        int
	IsPublicReadOnly bool
	TimelineId       uuid.UUID
	TimelineName     string
}

// SearchDecksWithTimeline is gsDatabase.SearchDecks plus each deck's
// timeline, optionally narrowed to one timeline (timelineFilter ==
// uuid.Nil means no filter, matching the "Nil = no filter" convention used
// elsewhere, e.g. GetLeaderboard). A deck with no explicit
// TIMELINE_TRIVIA_DECK_TIMELINE row lazily belongs to DefaultTimelineId,
// the same convention countDecksForTimeline uses — the LEFT JOIN to
// TIMELINE_TRIVIA_TIMELINE is keyed off that same COALESCE so its name
// resolves correctly for those decks too.
func SearchDecksWithTimeline(name string, timelineFilter uuid.UUID, page int) ([]DeckWithTimeline, error) {
	name = "%" + name + "%"
	if page < 1 {
		page = 1
	}

	sqlString := `
		SELECT
			D.ID,
			D.NAME,
			(SELECT COUNT(*) FROM CARD AS C WHERE C.DECK_ID = D.ID) AS CARD_COUNT,
			D.IS_PUBLIC_READONLY,
			COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) AS TIMELINE_ID,
			T.NAME AS TIMELINE_NAME
		FROM DECK AS D
		LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
		LEFT JOIN TIMELINE_TRIVIA_TIMELINE AS T ON T.ID = COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?)
		WHERE D.NAME LIKE ?
	`
	args := []interface{}{DefaultTimelineId, DefaultTimelineId, name}
	if timelineFilter != uuid.Nil {
		sqlString += ` AND COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) = ?`
		args = append(args, DefaultTimelineId, timelineFilter)
	}
	sqlString += ` ORDER BY D.NAME LIMIT 10 OFFSET ?`
	args = append(args, (page-1)*10)

	rows, err := query(sqlString, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]DeckWithTimeline, 0)
	for rows.Next() {
		var d DeckWithTimeline
		if err := rows.Scan(&d.Id, &d.Name, &d.CardCount, &d.IsPublicReadOnly, &d.TimelineId, &d.TimelineName); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, d)
	}
	return result, nil
}

// CountDecksWithTimeline is gsDatabase.CountDecks with the same optional
// timeline filter SearchDecksWithTimeline takes, for that page's pagination.
func CountDecksWithTimeline(name string, timelineFilter uuid.UUID) (int, error) {
	name = "%" + name + "%"

	sqlString := `
		SELECT COUNT(*)
		FROM DECK AS D
		LEFT JOIN TIMELINE_TRIVIA_DECK_TIMELINE AS DT ON DT.DECK_ID = D.ID
		WHERE D.NAME LIKE ?
	`
	args := []interface{}{name}
	if timelineFilter != uuid.Nil {
		sqlString += ` AND COALESCE(DT.TIMELINE_TRIVIA_TIMELINE_ID, ?) = ?`
		args = append(args, DefaultTimelineId, timelineFilter)
	}

	rows, err := query(sqlString, args...)
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

// SeedDefaultTimelineIfEmpty seeds the well-known "Real Life" timeline and
// its BCE/CE eras, but only when no timeline exists yet — safe to call on
// every startup. The seeded eras reproduce the exact output of the old
// hardcoded BCE/CE formatting logic they replace.
func SeedDefaultTimelineIfEmpty() error {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_TIMELINE`
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

	insertTimeline := `INSERT INTO TIMELINE_TRIVIA_TIMELINE(ID, NAME) VALUES (?, ?)`
	if err := execute(insertTimeline, DefaultTimelineId, "Real Life"); err != nil {
		return err
	}

	insertEra := `
		INSERT INTO TIMELINE_TRIVIA_ERA(ID, TIMELINE_TRIVIA_TIMELINE_ID, NAME, ABBREVIATION, SORT_ORDER, FROM_YEAR, TO_YEAR, EPOCH_OFFSET, DIRECTION)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if err := execute(insertEra, defaultEraBCEId, DefaultTimelineId, "Before Common Era", "B.C.E", 0, nil, -1, 0, EraDirectionBackward); err != nil {
		return err
	}
	if err := execute(insertEra, defaultEraCEId, DefaultTimelineId, "Common Era", "", 1, 0, nil, 0, EraDirectionForward); err != nil {
		return err
	}

	return nil
}
