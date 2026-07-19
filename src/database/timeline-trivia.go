package database

import (
	"errors"
	"fmt"
	"log"
	"time"

	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"
)

// TimelineTriviaGame represents a TimelineTrivia game instance
type TimelineTriviaGame struct {
	Id              uuid.UUID
	LobbyId         uuid.UUID
	CreatedOnDate   time.Time
	CurrentPlayerId uuid.NullUUID
	GameStatus      string
	CardsToWin      int
	WinnerId        uuid.NullUUID
}

// TimelineTriviaTimelineCard represents a card in a player's timeline
type TimelineTriviaTimelineCard struct {
	Id           uuid.UUID
	CardId       uuid.UUID
	CardText     string
	CardYear     int
	Position     int
	PlacedOnDate time.Time
}

// TimelineTriviaCurrentCard represents the current card being played
type TimelineTriviaCurrentCard struct {
	CardId   uuid.UUID
	CardText string
	CardYear int
}

// TimelineTriviaPlayer represents a player in a TimelineTrivia game with their timeline
type TimelineTriviaPlayer struct {
	PlayerId     uuid.UUID
	UserId       uuid.UUID
	UserName     string
	IsActive     bool
	TimelineSize int
	IsCurrent    bool
}

// TimelineTriviaPlayerTimeline represents a player with their full timeline for display
type TimelineTriviaPlayerTimeline struct {
	PlayerId   uuid.UUID
	PlayerName string
	IsCurrent  bool
	IsMe       bool
	Timeline   []TimelineTriviaTimelineCard
}

// GetTimelineTriviaGame retrieves the TimelineTrivia game for a lobby
func GetTimelineTriviaGame(lobbyId uuid.UUID) (TimelineTriviaGame, error) {
	return getTimelineTriviaGameByColumn("LOBBY_ID", lobbyId)
}

// GetTimelineTriviaGameById retrieves the TimelineTrivia game by its ID
func GetTimelineTriviaGameById(gameId uuid.UUID) (TimelineTriviaGame, error) {
	return getTimelineTriviaGameByColumn("ID", gameId)
}

// getTimelineTriviaGameByColumn is a helper to retrieve a game by a specific column
func getTimelineTriviaGameByColumn(column string, value uuid.UUID) (TimelineTriviaGame, error) {
	var game TimelineTriviaGame

	sqlString := fmt.Sprintf(`
		SELECT
			ID,
			LOBBY_ID,
			CREATED_ON_DATE,
			CURRENT_PLAYER_ID,
			GAME_STATUS,
			CARDS_TO_WIN,
			WINNER_ID
		FROM TIMELINE_TRIVIA_GAME
		WHERE %s = ?
	`, column)
	rows, err := query(sqlString, value)
	if err != nil {
		return game, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(
			&game.Id,
			&game.LobbyId,
			&game.CreatedOnDate,
			&game.CurrentPlayerId,
			&game.GameStatus,
			&game.CardsToWin,
			&game.WinnerId,
		); err != nil {
			log.Println(err)
			return game, errors.New("failed to scan row in query results")
		}
	}

	return game, nil
}

// CreateTimelineTriviaGame creates a new TimelineTrivia game for a lobby
func CreateTimelineTriviaGame(lobbyId uuid.UUID, cardsToWin int) (uuid.UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return id, errors.New("failed to generate new id")
	}

	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_GAME(
			ID,
			LOBBY_ID,
			CARDS_TO_WIN
		)
		VALUES (?, ?, ?)
	`
	return id, execute(sqlString, id, lobbyId, cardsToWin)
}

// CreateTimelineTriviaLobby creates a new lobby for TimelineTrivia, delegating base
// lobby creation to the gameshell framework.
func CreateTimelineTriviaLobby(name string, password string) (uuid.UUID, error) {
	return gsDatabase.CreateLobby(name, "", password)
}

// InitializeTimelineTriviaDrawPile populates the draw pile with cards from decks
// Cards must have a year in their text (extracted via regex)
func InitializeTimelineTriviaDrawPile(gameId uuid.UUID, deckIds []uuid.UUID) error {
	if len(deckIds) == 0 {
		return errors.New("no decks provided")
	}

	// Build placeholders for deck IDs
	placeholders := ""
	args := make([]interface{}, 0, len(deckIds)+1)
	args = append(args, gameId)
	for i, deckId := range deckIds {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
		args = append(args, deckId)
	}

	// Pull the deck cards that have an authored year into the draw pile.
	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_DRAW_PILE (ID, TIMELINE_TRIVIA_GAME_ID, CARD_ID, CARD_YEAR)
		SELECT UUID(), ?, C.ID, C.CARD_YEAR
		FROM CARD C
		WHERE C.DECK_ID IN (` + placeholders + `)
			AND C.CARD_YEAR IS NOT NULL
	`
	return execute(sqlString, args...)
}

// TimelineTriviaYearRange is one inclusive [FromYear, ToYear] filter for a game.
type TimelineTriviaYearRange struct {
	FromYear int
	ToYear   int
}

// AddYearRange stores one inclusive year-range filter for a game.
func AddYearRange(gameId uuid.UUID, fromYear int, toYear int) error {
	id, err := uuid.NewUUID()
	if err != nil {
		log.Println(err)
		return errors.New("failed to generate new id")
	}
	sqlString := `
		INSERT INTO TIMELINE_TRIVIA_YEAR_RANGE (ID, TIMELINE_TRIVIA_GAME_ID, FROM_YEAR, TO_YEAR)
		VALUES (?, ?, ?, ?)
	`
	return execute(sqlString, id, gameId, fromYear, toYear)
}

// GetYearRanges returns a game's year-range filters (empty = no filter).
func GetYearRanges(gameId uuid.UUID) ([]TimelineTriviaYearRange, error) {
	sqlString := `
		SELECT FROM_YEAR, TO_YEAR
		FROM TIMELINE_TRIVIA_YEAR_RANGE
		WHERE TIMELINE_TRIVIA_GAME_ID = ?
		ORDER BY FROM_YEAR
	`
	rows, err := query(sqlString, gameId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TimelineTriviaYearRange, 0)
	for rows.Next() {
		var r TimelineTriviaYearRange
		if err := rows.Scan(&r.FromYear, &r.ToYear); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, r)
	}
	return result, nil
}

// ApplyYearRangeFilter removes draw-pile cards whose year falls outside every
// configured range. No-op when the game has no ranges.
func ApplyYearRangeFilter(gameId uuid.UUID) error {
	ranges, err := GetYearRanges(gameId)
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		return nil
	}

	sqlDelete := `
		DELETE FROM TIMELINE_TRIVIA_DRAW_PILE
		WHERE TIMELINE_TRIVIA_GAME_ID = ?
			AND NOT EXISTS (
				SELECT 1
				FROM TIMELINE_TRIVIA_YEAR_RANGE R
				WHERE R.TIMELINE_TRIVIA_GAME_ID = ?
					AND CARD_YEAR BETWEEN R.FROM_YEAR AND R.TO_YEAR
			)
	`
	return execute(sqlDelete, gameId, gameId)
}

// DrawTimelineTriviaCard draws a random card from the draw pile and sets it as current
func DrawTimelineTriviaCard(gameId uuid.UUID) error {
	// Clear any existing current card
	sqlClear := `DELETE FROM TIMELINE_TRIVIA_CURRENT_CARD WHERE TIMELINE_TRIVIA_GAME_ID = ?`
	if err := execute(sqlClear, gameId); err != nil {
		return err
	}

	// Get a random undrawn card
	sqlDraw := `
		INSERT INTO TIMELINE_TRIVIA_CURRENT_CARD (ID, TIMELINE_TRIVIA_GAME_ID, CARD_ID, CARD_YEAR)
		SELECT UUID(), ?, CARD_ID, CARD_YEAR
		FROM TIMELINE_TRIVIA_DRAW_PILE
		WHERE TIMELINE_TRIVIA_GAME_ID = ? AND DRAWN = 0
		ORDER BY RAND()
		LIMIT 1
	`
	if err := execute(sqlDraw, gameId, gameId); err != nil {
		return err
	}

	// Mark the card as drawn
	sqlMark := `
		UPDATE TIMELINE_TRIVIA_DRAW_PILE
		SET DRAWN = 1
		WHERE TIMELINE_TRIVIA_GAME_ID = ?
		AND CARD_ID = (SELECT CARD_ID FROM TIMELINE_TRIVIA_CURRENT_CARD WHERE TIMELINE_TRIVIA_GAME_ID = ?)
	`
	return execute(sqlMark, gameId, gameId)
}

// GetTimelineTriviaCurrentCard gets the current card being played
func GetTimelineTriviaCurrentCard(gameId uuid.UUID) (TimelineTriviaCurrentCard, error) {
	var card TimelineTriviaCurrentCard

	sqlString := `
		SELECT CC.CARD_ID, C.TEXT, CC.CARD_YEAR
		FROM TIMELINE_TRIVIA_CURRENT_CARD CC
		INNER JOIN CARD C ON C.ID = CC.CARD_ID
		WHERE CC.TIMELINE_TRIVIA_GAME_ID = ?
	`
	rows, err := query(sqlString, gameId)
	if err != nil {
		return card, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&card.CardId, &card.CardText, &card.CardYear); err != nil {
			log.Println(err)
			return card, errors.New("failed to scan row in query results")
		}
	}

	return card, nil
}

// GetPlayerTimeline gets all cards in a player's timeline for a game
func GetPlayerTimeline(gameId uuid.UUID, playerId uuid.UUID) ([]TimelineTriviaTimelineCard, error) {
	sqlString := `
		SELECT PT.ID, PT.CARD_ID, C.TEXT, PT.CARD_YEAR, PT.POSITION, PT.PLACED_ON_DATE
		FROM TIMELINE_TRIVIA_PLAYER_TIMELINE PT
		INNER JOIN CARD C ON C.ID = PT.CARD_ID
		WHERE PT.TIMELINE_TRIVIA_GAME_ID = ? AND PT.PLAYER_ID = ?
		ORDER BY PT.POSITION ASC
	`
	rows, err := query(sqlString, gameId, playerId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TimelineTriviaTimelineCard, 0)
	for rows.Next() {
		var card TimelineTriviaTimelineCard
		if err := rows.Scan(
			&card.Id,
			&card.CardId,
			&card.CardText,
			&card.CardYear,
			&card.Position,
			&card.PlacedOnDate,
		); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, card)
	}

	return result, nil
}

// GetAllPlayerTimelines gets all players' timelines for a game, ordered with current player first
func GetAllPlayerTimelines(gameId uuid.UUID, currentPlayerId uuid.UUID, viewingPlayerId uuid.UUID) ([]TimelineTriviaPlayerTimeline, error) {
	// Get all active players
	players, err := GetTimelineTriviaPlayers(gameId)
	if err != nil {
		return nil, err
	}

	result := make([]TimelineTriviaPlayerTimeline, 0, len(players))

	// First add current player
	for _, p := range players {
		if p.IsActive && p.PlayerId == currentPlayerId {
			timeline, err := GetPlayerTimeline(gameId, p.PlayerId)
			if err != nil {
				timeline = []TimelineTriviaTimelineCard{}
			}
			result = append(result, TimelineTriviaPlayerTimeline{
				PlayerId:   p.PlayerId,
				PlayerName: p.UserName,
				IsCurrent:  true,
				IsMe:       p.PlayerId == viewingPlayerId,
				Timeline:   timeline,
			})
			break
		}
	}

	// Then add other players in order
	for _, p := range players {
		if p.IsActive && p.PlayerId != currentPlayerId {
			timeline, err := GetPlayerTimeline(gameId, p.PlayerId)
			if err != nil {
				timeline = []TimelineTriviaTimelineCard{}
			}
			result = append(result, TimelineTriviaPlayerTimeline{
				PlayerId:   p.PlayerId,
				PlayerName: p.UserName,
				IsCurrent:  false,
				IsMe:       p.PlayerId == viewingPlayerId,
				Timeline:   timeline,
			})
		}
	}

	return result, nil
}

// PlaceCardInTimeline attempts to place the current card in a player's timeline
// Returns true if placement was correct, false otherwise
func PlaceCardInTimeline(gameId uuid.UUID, playerId uuid.UUID, position int) (bool, error) {
	// Get the current card
	currentCard, err := GetTimelineTriviaCurrentCard(gameId)
	if err != nil {
		return false, err
	}
	if currentCard.CardId == uuid.Nil {
		return false, errors.New("no current card to place")
	}

	// Get player's current timeline
	timeline, err := GetPlayerTimeline(gameId, playerId)
	if err != nil {
		return false, err
	}

	// Validate position (must be between 0 and len(timeline))
	if position < 0 || position > len(timeline) {
		return false, errors.New("invalid position")
	}

	// Check if placement is correct
	correct := true
	if position > 0 {
		// Card before this position must have year <= current card's year
		if timeline[position-1].CardYear > currentCard.CardYear {
			correct = false
		}
	}
	if position < len(timeline) {
		// Card after this position must have year >= current card's year
		if timeline[position].CardYear < currentCard.CardYear {
			correct = false
		}
	}

	if correct {
		// Shift existing cards to make room
		sqlShift := `
			UPDATE TIMELINE_TRIVIA_PLAYER_TIMELINE
			SET POSITION = POSITION + 1
			WHERE TIMELINE_TRIVIA_GAME_ID = ? AND PLAYER_ID = ? AND POSITION >= ?
		`
		if err := execute(sqlShift, gameId, playerId, position); err != nil {
			return false, err
		}

		// Insert the new card
		id, err := uuid.NewUUID()
		if err != nil {
			return false, err
		}
		sqlInsert := `
			INSERT INTO TIMELINE_TRIVIA_PLAYER_TIMELINE (ID, TIMELINE_TRIVIA_GAME_ID, PLAYER_ID, CARD_ID, CARD_YEAR, POSITION)
			VALUES (?, ?, ?, ?, ?, ?)
		`
		if err := execute(sqlInsert, id, gameId, playerId, currentCard.CardId, currentCard.CardYear, position); err != nil {
			return false, err
		}
	}

	// Clear current card
	sqlClear := `DELETE FROM TIMELINE_TRIVIA_CURRENT_CARD WHERE TIMELINE_TRIVIA_GAME_ID = ?`
	if err := execute(sqlClear, gameId); err != nil {
		return correct, err
	}

	return correct, nil
}

// GetTimelineTriviaPlayers gets all players in a TimelineTrivia game with their timeline sizes
func GetTimelineTriviaPlayers(gameId uuid.UUID) ([]TimelineTriviaPlayer, error) {
	sqlString := `
		SELECT 
			P.ID,
			P.USER_ID,
			U.NAME,
			P.IS_ACTIVE,
			COALESCE(
				(SELECT COUNT(*) FROM TIMELINE_TRIVIA_PLAYER_TIMELINE PT 
				 WHERE PT.TIMELINE_TRIVIA_GAME_ID = CG.ID AND PT.PLAYER_ID = P.ID),
				0
			) AS TIMELINE_SIZE,
			CASE WHEN CG.CURRENT_PLAYER_ID = P.ID THEN 1 ELSE 0 END AS IS_CURRENT
		FROM TIMELINE_TRIVIA_GAME CG
		INNER JOIN LOBBY L ON L.ID = CG.LOBBY_ID
		INNER JOIN PLAYER P ON P.LOBBY_ID = L.ID
		INNER JOIN USER U ON U.ID = P.USER_ID
		WHERE CG.ID = ?
		ORDER BY P.JOIN_ORDER ASC
	`
	rows, err := query(sqlString, gameId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TimelineTriviaPlayer, 0)
	for rows.Next() {
		var player TimelineTriviaPlayer
		if err := rows.Scan(
			&player.PlayerId,
			&player.UserId,
			&player.UserName,
			&player.IsActive,
			&player.TimelineSize,
			&player.IsCurrent,
		); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, player)
	}

	return result, nil
}

// SetTimelineTriviaCurrentPlayer sets whose turn it is
func SetTimelineTriviaCurrentPlayer(gameId uuid.UUID, playerId uuid.UUID) error {
	sqlString := `UPDATE TIMELINE_TRIVIA_GAME SET CURRENT_PLAYER_ID = ? WHERE ID = ?`
	return execute(sqlString, playerId, gameId)
}

// AdvanceTimelineTriviaTurn moves to the next active player
func AdvanceTimelineTriviaTurn(gameId uuid.UUID) error {
	players, err := GetTimelineTriviaPlayers(gameId)
	if err != nil {
		return err
	}

	// Find current player index
	currentIdx := -1
	for i, p := range players {
		if p.IsCurrent {
			currentIdx = i
			break
		}
	}

	// Find next active player
	nextIdx := currentIdx
	for i := 0; i < len(players); i++ {
		nextIdx = (nextIdx + 1) % len(players)
		if players[nextIdx].IsActive {
			break
		}
	}

	if nextIdx < len(players) {
		return SetTimelineTriviaCurrentPlayer(gameId, players[nextIdx].PlayerId)
	}

	return errors.New("no active players found")
}

// StartTimelineTriviaGame starts the game by setting status and first player
func StartTimelineTriviaGame(gameId uuid.UUID) error {
	players, err := GetTimelineTriviaPlayers(gameId)
	if err != nil {
		return err
	}

	if len(players) == 0 {
		return errors.New("no players in game")
	}

	// Deal one card to each player to start their timeline
	for _, player := range players {
		if player.IsActive {
			// Draw a card from the pile
			var cardId uuid.UUID
			var cardYear int
			sqlGetCard := `
				SELECT CARD_ID, CARD_YEAR
				FROM TIMELINE_TRIVIA_DRAW_PILE
				WHERE TIMELINE_TRIVIA_GAME_ID = ? AND DRAWN = 0
				ORDER BY RAND()
				LIMIT 1
			`
			rows, err := query(sqlGetCard, gameId)
			if err != nil {
				return err
			}
			defer rows.Close()

			if rows.Next() {
				if err := rows.Scan(&cardId, &cardYear); err != nil {
					return err
				}
			} else {
				return errors.New("not enough cards to deal initial cards")
			}

			// Mark card as drawn
			sqlMarkDrawn := `UPDATE TIMELINE_TRIVIA_DRAW_PILE SET DRAWN = 1 WHERE TIMELINE_TRIVIA_GAME_ID = ? AND CARD_ID = ?`
			if err := execute(sqlMarkDrawn, gameId, cardId); err != nil {
				return err
			}

			// Add to player's timeline at position 0
			id, err := uuid.NewUUID()
			if err != nil {
				return err
			}
			sqlAddToTimeline := `
				INSERT INTO TIMELINE_TRIVIA_PLAYER_TIMELINE (ID, TIMELINE_TRIVIA_GAME_ID, PLAYER_ID, CARD_ID, CARD_YEAR, POSITION)
				VALUES (?, ?, ?, ?, ?, 0)
			`
			if err := execute(sqlAddToTimeline, id, gameId, player.PlayerId, cardId, cardYear); err != nil {
				return err
			}
		}
	}

	// Find first active player
	var firstPlayer uuid.UUID
	for _, p := range players {
		if p.IsActive {
			firstPlayer = p.PlayerId
			break
		}
	}

	if firstPlayer == uuid.Nil {
		return errors.New("no active players")
	}

	// Set game as active and set first player
	sqlString := `UPDATE TIMELINE_TRIVIA_GAME SET GAME_STATUS = 'active', CURRENT_PLAYER_ID = ? WHERE ID = ?`
	if err := execute(sqlString, firstPlayer, gameId); err != nil {
		return err
	}

	// Draw first card for play
	return DrawTimelineTriviaCard(gameId)
}

// ResetTimelineTriviaGame resets a finished game to play again
func ResetTimelineTriviaGame(gameId uuid.UUID) error {
	// Clear all player timelines
	sqlClearTimelines := `DELETE FROM TIMELINE_TRIVIA_PLAYER_TIMELINE WHERE TIMELINE_TRIVIA_GAME_ID = ?`
	if err := execute(sqlClearTimelines, gameId); err != nil {
		return err
	}

	// Clear current card
	sqlClearCurrentCard := `DELETE FROM TIMELINE_TRIVIA_CURRENT_CARD WHERE TIMELINE_TRIVIA_GAME_ID = ?`
	if err := execute(sqlClearCurrentCard, gameId); err != nil {
		return err
	}

	// Reset draw pile - mark all cards as not drawn
	sqlResetDrawPile := `UPDATE TIMELINE_TRIVIA_DRAW_PILE SET DRAWN = 0 WHERE TIMELINE_TRIVIA_GAME_ID = ?`
	if err := execute(sqlResetDrawPile, gameId); err != nil {
		return err
	}

	// Reset game status to waiting
	sqlResetGame := `UPDATE TIMELINE_TRIVIA_GAME SET GAME_STATUS = 'waiting', CURRENT_PLAYER_ID = NULL, WINNER_ID = NULL WHERE ID = ?`
	if err := execute(sqlResetGame, gameId); err != nil {
		return err
	}

	return nil
}

// CheckTimelineTriviaWinner checks if any player has won
func CheckTimelineTriviaWinner(gameId uuid.UUID) (uuid.UUID, error) {
	game, err := GetTimelineTriviaGameById(gameId)
	if err != nil {
		return uuid.Nil, err
	}

	players, err := GetTimelineTriviaPlayers(gameId)
	if err != nil {
		return uuid.Nil, err
	}

	for _, p := range players {
		if p.TimelineSize >= game.CardsToWin {
			// Set winner
			sqlString := `UPDATE TIMELINE_TRIVIA_GAME SET GAME_STATUS = 'finished', WINNER_ID = ? WHERE ID = ?`
			if err := execute(sqlString, p.UserId, gameId); err != nil {
				return uuid.Nil, err
			}
			return p.UserId, nil
		}
	}

	return uuid.Nil, nil
}

// GetTimelineTriviaDrawPileCount returns the number of cards remaining in the draw pile
func GetTimelineTriviaDrawPileCount(gameId uuid.UUID) (int, error) {
	sqlString := `
		SELECT COUNT(*)
		FROM TIMELINE_TRIVIA_DRAW_PILE
		WHERE TIMELINE_TRIVIA_GAME_ID = ? AND DRAWN = 0
	`
	rows, err := query(sqlString, gameId)
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

// TimelineTriviaLobbyDetails represents a TimelineTrivia lobby for listing
type TimelineTriviaLobbyDetails struct {
	Id          uuid.UUID
	Name        string
	PlayerCount int
	GameStatus  string
	HasPassword bool
}

// SearchTimelineTriviaLobbies searches for TimelineTrivia-type lobbies
func SearchTimelineTriviaLobbies(name string, page int) ([]TimelineTriviaLobbyDetails, error) {
	name = "%" + name + "%"

	if page < 1 {
		page = 1
	}

	sqlString := `
		SELECT
			L.ID,
			L.NAME,
			L.PASSWORD_HASH IS NOT NULL AS HAS_PASSWORD,
			COALESCE(CG.GAME_STATUS, 'waiting') AS GAME_STATUS,
			COUNT(P.ID) AS PLAYER_COUNT
		FROM LOBBY AS L
			LEFT JOIN TIMELINE_TRIVIA_GAME AS CG ON CG.LOBBY_ID = L.ID
			LEFT JOIN PLAYER AS P ON P.LOBBY_ID = L.ID AND P.IS_ACTIVE = 1
		WHERE L.NAME LIKE ?
		GROUP BY L.ID
		ORDER BY L.CREATED_ON_DATE DESC
		LIMIT 10 OFFSET ?
	`
	rows, err := query(sqlString, name, (page-1)*10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TimelineTriviaLobbyDetails, 0)
	for rows.Next() {
		var ld TimelineTriviaLobbyDetails
		if err := rows.Scan(
			&ld.Id,
			&ld.Name,
			&ld.HasPassword,
			&ld.GameStatus,
			&ld.PlayerCount,
		); err != nil {
			log.Println(err)
			return nil, errors.New("failed to scan row in query results")
		}
		result = append(result, ld)
	}
	return result, nil
}

// CountTimelineTriviaLobbies counts TimelineTrivia-type lobbies matching name
func CountTimelineTriviaLobbies(name string) (int, error) {
	name = "%" + name + "%"

	sqlString := `
		SELECT COUNT(*)
		FROM LOBBY
		WHERE NAME LIKE ?
	`
	rows, err := query(sqlString, name)
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
