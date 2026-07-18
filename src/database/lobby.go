package database

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
)

type Lobby struct {
	Id            uuid.UUID
	CreatedOnDate time.Time

	Name         string
	Message      sql.NullString
	PasswordHash sql.NullString
}

func GetLobby(id uuid.UUID) (Lobby, error) {
	var lobby Lobby

	sqlString := `
		SELECT
			ID,
			CREATED_ON_DATE,
			NAME,
			MESSAGE,
			PASSWORD_HASH
		FROM LOBBY
		WHERE ID = ?
	`
	rows, err := query(sqlString, id)
	if err != nil {
		return lobby, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(
			&lobby.Id,
			&lobby.CreatedOnDate,
			&lobby.Name,
			&lobby.Message,
			&lobby.PasswordHash); err != nil {
			log.Println(err)
			return lobby, errors.New("failed to scan row in query results")
		}
	}

	return lobby, nil
}
