package database

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
)

// Category is one entry in a timeline's predefined, admin-managed list of
// card categories. Scoped to exactly one timeline, the same way an Era is —
// see database/timeline.go. Referential integrity to CARD.CATEGORY_ID is
// enforced here in Go (required on card create/edit; reassigned before a
// category is deleted) rather than a DB foreign key.
type Category struct {
	Id            uuid.UUID
	CreatedOnDate time.Time
	TimelineId    uuid.UUID
	Name          string
}

// CategoryWithCount is a category plus how many cards currently reference it,
// for the admin management page.
type CategoryWithCount struct {
	Category
	CardCount int
}

// categorySelectColumns is shared by every category query so their SELECT
// list and Scan order can never drift apart.
const categorySelectColumns = `ID, CREATED_ON_DATE, TIMELINE_TRIVIA_TIMELINE_ID, NAME`

func scanCategory(rows *sql.Rows, c *Category) error {
	return rows.Scan(&c.Id, &c.CreatedOnDate, &c.TimelineId, &c.Name)
}

// GetCategoriesForTimeline returns a timeline's categories, alphabetically.
func GetCategoriesForTimeline(timelineId uuid.UUID) ([]Category, error) {
	sqlString := `
		SELECT ` + categorySelectColumns + `
		FROM TIMELINE_TRIVIA_CATEGORY
		WHERE TIMELINE_TRIVIA_TIMELINE_ID = ?
		ORDER BY NAME
	`
	rows, err := query(sqlString, timelineId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Category, 0)
	for rows.Next() {
		var c Category
		if err := scanCategory(rows, &c); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, c)
	}
	return result, nil
}

// GetAllCategories returns every category across every timeline, ordered by
// name — used where categories from multiple timelines need to be shown
// together (e.g. the lobby-creation form's category filter, which the
// client further narrows to the selected timeline).
func GetAllCategories() ([]Category, error) {
	sqlString := `
		SELECT ` + categorySelectColumns + `
		FROM TIMELINE_TRIVIA_CATEGORY
		ORDER BY NAME
	`
	rows, err := query(sqlString)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Category, 0)
	for rows.Next() {
		var c Category
		if err := scanCategory(rows, &c); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, c)
	}
	return result, nil
}

// GetCategoriesForTimelineWithCounts returns a timeline's categories with
// each one's current card count, alphabetically, for the admin management
// page.
func GetCategoriesForTimelineWithCounts(timelineId uuid.UUID) ([]CategoryWithCount, error) {
	sqlString := `
		SELECT
			TC.ID,
			TC.CREATED_ON_DATE,
			TC.TIMELINE_TRIVIA_TIMELINE_ID,
			TC.NAME,
			(SELECT COUNT(*) FROM CARD WHERE CARD.CATEGORY_ID = TC.ID) AS CARD_COUNT
		FROM TIMELINE_TRIVIA_CATEGORY AS TC
		WHERE TC.TIMELINE_TRIVIA_TIMELINE_ID = ?
		ORDER BY TC.NAME
	`
	rows, err := query(sqlString, timelineId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]CategoryWithCount, 0)
	for rows.Next() {
		var c CategoryWithCount
		if err := rows.Scan(&c.Id, &c.CreatedOnDate, &c.TimelineId, &c.Name, &c.CardCount); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, c)
	}
	return result, nil
}

func GetCategory(id uuid.UUID) (Category, error) {
	var c Category

	sqlString := `
		SELECT ` + categorySelectColumns + `
		FROM TIMELINE_TRIVIA_CATEGORY
		WHERE ID = ?
	`
	rows, err := query(sqlString, id)
	if err != nil {
		return c, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scanCategory(rows, &c); err != nil {
			log.Println(err)
			return c, errors.New("failed to scan row in query results")
		}
	}

	return c, nil
}

// GetCategoryIdForTimeline returns the id of the category with the given
// name within timelineId, or Nil if no such category exists (used to map an
// import's category name to an id, scoped to the target deck's timeline).
func GetCategoryIdForTimeline(timelineId uuid.UUID, name string) (uuid.UUID, error) {
	var id uuid.UUID

	sqlString := `SELECT ID FROM TIMELINE_TRIVIA_CATEGORY WHERE TIMELINE_TRIVIA_TIMELINE_ID = ? AND NAME = ?`
	rows, err := query(sqlString, timelineId, name)
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

func CategoryExists(id uuid.UUID) (bool, error) {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_CATEGORY WHERE ID = ?`
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

func CategoryNameExistsInTimeline(timelineId uuid.UUID, name string) (bool, error) {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_CATEGORY WHERE TIMELINE_TRIVIA_TIMELINE_ID = ? AND NAME = ?`
	rows, err := query(sqlString, timelineId, name)
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

func CreateCategory(timelineId uuid.UUID, name string) (uuid.UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return id, errors.New("failed to generate new id")
	}

	sqlString := `INSERT INTO TIMELINE_TRIVIA_CATEGORY(ID, TIMELINE_TRIVIA_TIMELINE_ID, NAME) VALUES (?, ?, ?)`
	return id, execute(sqlString, id, timelineId, name)
}

// Sentinel errors DeleteCategoryReassigning returns for its
// validation-rejection cases, distinguished from a genuine server/DB error
// so the API handler can respond 404/400 instead of 500 — the same
// treatment DeleteEra's ErrEraNotFound/ErrEraInUse get.
var ErrCategoryNotFound = errors.New("category not found")
var ErrCategoryTargetNotFound = errors.New("target category not found")
var ErrCategorySelfReassign = errors.New("cannot reassign a category to itself")
var ErrCategoryTimelineMismatch = errors.New("target category must belong to the same timeline")

// DeleteCategoryReassigning moves every card in the deleted category to the
// target category, then deletes the category. The target must belong to the
// same timeline as the category being deleted — reassigning a Real Life
// card's category into a Star Wars one would leave it pointing at a
// category outside its own deck's timeline. This also means a timeline's
// last category can never be deleted through this path (there would be no
// valid same-timeline target to pick), the same "a timeline always has
// something to author cards against" guarantee DeleteEra enforces for eras.
// The reassign runs first so that even if the delete fails the cards are
// never left pointing at a missing category (worst case is a harmless empty
// category the admin can retry).
func DeleteCategoryReassigning(deleteId uuid.UUID, targetId uuid.UUID) error {
	if deleteId == targetId {
		return ErrCategorySelfReassign
	}

	category, err := GetCategory(deleteId)
	if err != nil {
		return err
	}
	if category.Id == uuid.Nil {
		return ErrCategoryNotFound
	}

	target, err := GetCategory(targetId)
	if err != nil {
		return err
	}
	if target.Id == uuid.Nil {
		return ErrCategoryTargetNotFound
	}
	if target.TimelineId != category.TimelineId {
		return ErrCategoryTimelineMismatch
	}

	reassignSQL := `UPDATE CARD SET CATEGORY_ID = ? WHERE CATEGORY_ID = ?`
	if err := execute(reassignSQL, targetId, deleteId); err != nil {
		return err
	}

	deleteSQL := `DELETE FROM TIMELINE_TRIVIA_CATEGORY WHERE ID = ?`
	return execute(deleteSQL, deleteId)
}
