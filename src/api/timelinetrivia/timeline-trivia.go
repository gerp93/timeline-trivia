package apiTimelineTrivia

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsWebsocket "github.com/gerp93/gameshell-framework/websocket"
	"github.com/google/uuid"

	"github.com/gerp93/card-timeline/database"
	"github.com/gerp93/card-timeline/static"
)

// TimelineTrivia deck ID
var timelineTriviaDeckId = uuid.MustParse("88026803-d22a-11f0-b4d2-60cf84649547")

// ensureGameExists makes sure a TimelineTrivia game exists for a lobby, creating one if needed
func ensureGameExists(lobbyId uuid.UUID) (database.TimelineTriviaGame, error) {
	game, err := database.GetTimelineTriviaGame(lobbyId)
	if err == nil && game.Id != uuid.Nil {
		return game, nil
	}

	// Auto-create the game with default settings
	gameId, createErr := database.CreateTimelineTriviaGame(lobbyId, 5) // 5 cards to win default
	if createErr != nil {
		return game, createErr
	}

	// Initialize draw pile with the TimelineTrivia deck (cards use authored years)
	_ = database.InitializeTimelineTriviaDrawPile(gameId, []uuid.UUID{timelineTriviaDeckId})

	// Re-fetch the game
	return database.GetTimelineTriviaGame(lobbyId)
}

// Create creates a new TimelineTrivia lobby and game
func Create(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("failed to parse form"))
		return
	}

	name := r.FormValue("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("name is required"))
		return
	}

	password := r.FormValue("password")

	cardsToWin := 5
	if cardsToWinStr := r.FormValue("cardsToWin"); cardsToWinStr != "" {
		if val, err := strconv.Atoi(cardsToWinStr); err == nil && val > 0 {
			cardsToWin = val
		}
	}

	// Create the lobby with game_type = 'timeline-trivia'
	lobbyId, err := database.CreateTimelineTriviaLobby(name, password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to create lobby"))
		return
	}

	// Create the TimelineTrivia game
	gameId, err := database.CreateTimelineTriviaGame(lobbyId, cardsToWin)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to create game"))
		return
	}

	// Get selected deck IDs
	deckIdStrings := r.Form["deckId"]
	if len(deckIdStrings) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("at least one deck is required"))
		return
	}

	deckIds := make([]uuid.UUID, 0, len(deckIdStrings))
	for _, idStr := range deckIdStrings {
		if id, err := uuid.Parse(idStr); err == nil {
			deckIds = append(deckIds, id)
		}
	}

	// Initialize draw pile with cards from decks (cards use authored years)
	if err := database.InitializeTimelineTriviaDrawPile(gameId, deckIds); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to initialize draw pile"))
		return
	}

	// Store any year-range filters (parallel fromYear/toYear form arrays) and
	// prune the draw pile to cards within those ranges. No ranges = no filter.
	fromYears := r.Form["fromYear"]
	toYears := r.Form["toYear"]
	for i := range fromYears {
		if i >= len(toYears) {
			break
		}
		if fromYears[i] == "" && toYears[i] == "" {
			continue // empty row, ignore
		}
		from, fromErr := strconv.Atoi(fromYears[i])
		to, toErr := strconv.Atoi(toYears[i])
		if fromErr != nil || toErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("year ranges must be whole numbers"))
			return
		}
		if from > to {
			from, to = to, from // tolerate reversed input
		}
		if err := database.AddYearRange(gameId, from, to); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("failed to save year range"))
			return
		}
	}
	if err := database.ApplyYearRangeFilter(gameId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to apply year range filter"))
		return
	}

	// Redirect to the new lobby
	w.Header().Set("HX-Redirect", fmt.Sprintf("/timeline-trivia/%s", lobbyId))
	w.WriteHeader(http.StatusOK)
}

// StartGame starts the TimelineTrivia game
func StartGame(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	game, err := database.GetTimelineTriviaGame(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}

	if game.GameStatus != "waiting" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("game already started"))
		return
	}

	if err := database.StartTimelineTriviaGame(game.Id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to start game: " + err.Error()))
		return
	}

	// Notify all players via WebSocket to reload the page
	gsWebsocket.LobbyBroadcast(lobbyId, "reload")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Game started!"))
}

// ResetGame resets a finished TimelineTrivia game to start a new one
func ResetGame(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	game, err := database.GetTimelineTriviaGame(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}

	if game.GameStatus != "finished" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("game is not finished"))
		return
	}

	if err := database.ResetTimelineTriviaGame(game.Id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to reset game: " + err.Error()))
		return
	}

	// Notify all players to reload the page
	gsWebsocket.LobbyBroadcast(lobbyId, "reload")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Game reset! Starting new game..."))
}

// PlaceCard handles a player placing the current card in their timeline
func PlaceCard(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	userId := gsApi.GetUserId(r)

	// Get player ID for this user in this lobby
	player, err := gsDatabase.GetLobbyUserPlayer(lobbyId, userId)
	if err != nil || player.Id == uuid.Nil {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("not a player in this game"))
		return
	}

	game, err := database.GetTimelineTriviaGame(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}

	// Check if it's this player's turn
	if !game.CurrentPlayerId.Valid || game.CurrentPlayerId.UUID != player.Id {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("not your turn"))
		return
	}

	// Get position from form
	positionStr := r.FormValue("position")
	position, err := strconv.Atoi(positionStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid position"))
		return
	}

	// Attempt to place the card
	correct, err := database.PlaceCardInTimeline(game.Id, player.Id, position)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	// Check for winner
	winnerId, err := database.CheckTimelineTriviaWinner(game.Id)
	if err == nil && winnerId != uuid.Nil {
		// Game over!
		gsWebsocket.LobbyBroadcast(lobbyId, fmt.Sprintf("result:%s:correct:You win!", player.Name))
		gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("You win!"))
		return
	}

	// Always advance to next player after each turn
	if err := database.AdvanceTimelineTriviaTurn(game.Id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to advance turn"))
		return
	}

	// Draw new card for next player
	if err := database.DrawTimelineTriviaCard(game.Id); err != nil {
		// No more cards - game over
		gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
		w.WriteHeader(http.StatusOK)
		if correct {
			_, _ = w.Write([]byte("Correct! No more cards."))
		} else {
			_, _ = w.Write([]byte("Incorrect. No more cards."))
		}
		return
	}

	if correct {
		gsWebsocket.LobbyBroadcast(lobbyId, fmt.Sprintf("result:%s:correct:Correct!", player.Name))
		gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Correct! Next player's turn."))
	} else {
		gsWebsocket.LobbyBroadcast(lobbyId, fmt.Sprintf("result:%s:incorrect:Wrong!", player.Name))
		gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Incorrect. Next player's turn."))
	}
}

// GetGameState returns the current game state HTML
func GetGameState(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	userId := gsApi.GetUserId(r)

	game, err := database.GetTimelineTriviaGame(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}

	// Get player for this user
	player, err := gsDatabase.GetLobbyUserPlayer(lobbyId, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get player"))
		return
	}

	// Get current card
	currentCard, _ := database.GetTimelineTriviaCurrentCard(game.Id)

	// Get all players with their timeline sizes
	players, _ := database.GetTimelineTriviaPlayers(game.Id)

	// Get this player's timeline
	var timeline []database.TimelineTriviaTimelineCard
	if player.Id != uuid.Nil {
		timeline, _ = database.GetPlayerTimeline(game.Id, player.Id)
	}

	// Get draw pile count
	drawPileCount, _ := database.GetTimelineTriviaDrawPileCount(game.Id)

	// Is it this player's turn?
	isMyTurn := game.CurrentPlayerId.Valid && player.Id != uuid.Nil && game.CurrentPlayerId.UUID == player.Id

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/components/timeline-trivia/game-state.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse template"))
		return
	}

	type data struct {
		Game          database.TimelineTriviaGame
		CurrentCard   database.TimelineTriviaCurrentCard
		Players       []database.TimelineTriviaPlayer
		Timeline      []database.TimelineTriviaTimelineCard
		DrawPileCount int
		IsMyTurn      bool
		PlayerId      uuid.UUID
		LobbyId       uuid.UUID
	}

	_ = tmpl.Execute(w, data{
		Game:          game,
		CurrentCard:   currentCard,
		Players:       players,
		Timeline:      timeline,
		DrawPileCount: drawPileCount,
		IsMyTurn:      isMyTurn,
		PlayerId:      player.Id,
		LobbyId:       lobbyId,
	})
}

// GetTimeline returns the player's timeline HTML
func GetTimeline(w http.ResponseWriter, r *http.Request) {
	// Prevent caching
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	userId := gsApi.GetUserId(r)

	game, err := ensureGameExists(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}

	player, err := gsDatabase.GetLobbyUserPlayer(lobbyId, userId)
	if err != nil || player.Id == uuid.Nil {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("not a player"))
		return
	}

	// Get all players' timelines, ordered with current player first
	currentPlayerId := uuid.Nil
	if game.CurrentPlayerId.Valid {
		currentPlayerId = game.CurrentPlayerId.UUID
	}

	allTimelines, err := database.GetAllPlayerTimelines(game.Id, currentPlayerId, player.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get timelines"))
		return
	}

	isMyTurn := game.CurrentPlayerId.Valid && game.CurrentPlayerId.UUID == player.Id

	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}

	tmpl, err := template.New("timeline.html").Funcs(funcMap).ParseFS(
		static.StaticFiles,
		"html/components/timeline-trivia/timeline.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse template: " + err.Error()))
		return
	}

	type data struct {
		AllTimelines []database.TimelineTriviaPlayerTimeline
		IsMyTurn     bool
		GameStatus   string
		LobbyId      uuid.UUID
	}

	_ = tmpl.Execute(w, data{
		AllTimelines: allTimelines,
		IsMyTurn:     isMyTurn,
		GameStatus:   game.GameStatus,
		LobbyId:      lobbyId,
	})
}

// GetCurrentCard returns the current card being played
func GetCurrentCard(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	game, err := ensureGameExists(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}

	currentCard, _ := database.GetTimelineTriviaCurrentCard(game.Id)

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/components/timeline-trivia/current-card.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse template"))
		return
	}

	_ = tmpl.Execute(w, currentCard)
}

// GetDrawPileCount returns the number of cards remaining in the draw pile
func GetDrawPileCount(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("0"))
		return
	}

	game, err := ensureGameExists(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("0"))
		return
	}

	count, err := database.GetTimelineTriviaDrawPileCount(game.Id)
	if err != nil {
		_, _ = w.Write([]byte("0"))
		return
	}

	_, _ = w.Write([]byte(strconv.Itoa(count)))
}

// GetPlayers returns the players list HTML
func GetPlayers(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	game, err := ensureGameExists(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}

	players, err := database.GetTimelineTriviaPlayers(game.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get players"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/components/timeline-trivia/players.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse template"))
		return
	}

	type data struct {
		Players         []database.TimelineTriviaPlayer
		CurrentPlayerId uuid.UUID
		CardsToWin      int
	}

	currentPlayerId := uuid.Nil
	if game.CurrentPlayerId.Valid {
		currentPlayerId = game.CurrentPlayerId.UUID
	}

	_ = tmpl.Execute(w, data{
		Players:         players,
		CurrentPlayerId: currentPlayerId,
		CardsToWin:      game.CardsToWin,
	})
}

// Search returns lobby search results
func Search(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	pageStr := r.FormValue("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	lobbies, err := database.SearchTimelineTriviaLobbies(name, page)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to search lobbies"))
		return
	}

	count, err := database.CountTimelineTriviaLobbies(name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to count lobbies"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/components/table-rows/timeline-trivia-lobby-rows.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse template"))
		return
	}

	type data struct {
		Lobbies     []database.TimelineTriviaLobbyDetails
		TotalCount  int
		CurrentPage int
		PageSize    int
	}

	_ = tmpl.Execute(w, data{
		Lobbies:     lobbies,
		TotalCount:  count,
		CurrentPage: page,
		PageSize:    10, // Same as database query LIMIT
	})
}
