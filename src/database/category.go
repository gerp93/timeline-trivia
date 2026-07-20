package database

import (
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
)

// Category is one entry in the predefined, admin-managed list of card
// categories. Referential integrity to CARD.CATEGORY_ID is enforced here in
// Go (required on card create/edit; reassigned before a category is deleted)
// rather than a DB foreign key.
type Category struct {
	Id            uuid.UUID
	CreatedOnDate time.Time
	Name          string
}

// CategoryWithCount is a category plus how many cards currently reference it,
// for the admin management page.
type CategoryWithCount struct {
	Category
	CardCount int
}

func GetCategories() ([]Category, error) {
	sqlString := `
		SELECT
			ID,
			CREATED_ON_DATE,
			NAME
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
		if err := rows.Scan(&c.Id, &c.CreatedOnDate, &c.Name); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, c)
	}
	return result, nil
}

// GetCategoriesWithCounts returns every category with its current card count,
// ordered by name, for the admin management page.
func GetCategoriesWithCounts() ([]CategoryWithCount, error) {
	sqlString := `
		SELECT
			TC.ID,
			TC.CREATED_ON_DATE,
			TC.NAME,
			(SELECT COUNT(*) FROM CARD WHERE CARD.CATEGORY_ID = TC.ID) AS CARD_COUNT
		FROM TIMELINE_TRIVIA_CATEGORY AS TC
		ORDER BY TC.NAME
	`
	rows, err := query(sqlString)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]CategoryWithCount, 0)
	for rows.Next() {
		var c CategoryWithCount
		if err := rows.Scan(&c.Id, &c.CreatedOnDate, &c.Name, &c.CardCount); err != nil {
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
		SELECT
			ID,
			CREATED_ON_DATE,
			NAME
		FROM TIMELINE_TRIVIA_CATEGORY
		WHERE ID = ?
	`
	rows, err := query(sqlString, id)
	if err != nil {
		return c, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&c.Id, &c.CreatedOnDate, &c.Name); err != nil {
			log.Println(err)
			return c, errors.New("failed to scan row in query results")
		}
	}

	return c, nil
}

// GetCategoryId returns the id of the category with the given name, or Nil if
// no such category exists (used to map an import's category name to an id).
func GetCategoryId(name string) (uuid.UUID, error) {
	var id uuid.UUID

	sqlString := `SELECT ID FROM TIMELINE_TRIVIA_CATEGORY WHERE NAME = ?`
	rows, err := query(sqlString, name)
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

func CategoryNameExists(name string) (bool, error) {
	sqlString := `SELECT COUNT(*) FROM TIMELINE_TRIVIA_CATEGORY WHERE NAME = ?`
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

func CreateCategory(name string) (uuid.UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return id, errors.New("failed to generate new id")
	}

	sqlString := `INSERT INTO TIMELINE_TRIVIA_CATEGORY(ID, NAME) VALUES (?, ?)`
	return id, execute(sqlString, id, name)
}

// DeleteCategoryReassigning moves every card in the deleted category to the
// target category, then deletes the category. The reassign runs first so that
// even if the delete fails the cards are never left pointing at a missing
// category (worst case is a harmless empty category the admin can retry).
func DeleteCategoryReassigning(deleteId uuid.UUID, targetId uuid.UUID) error {
	if deleteId == targetId {
		return errors.New("cannot reassign a category to itself")
	}

	reassignSQL := `UPDATE CARD SET CATEGORY_ID = ? WHERE CATEGORY_ID = ?`
	if err := execute(reassignSQL, targetId, deleteId); err != nil {
		return err
	}

	deleteSQL := `DELETE FROM TIMELINE_TRIVIA_CATEGORY WHERE ID = ?`
	return execute(deleteSQL, deleteId)
}
