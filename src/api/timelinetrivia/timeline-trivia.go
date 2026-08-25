package apiTimelineTrivia

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsWebsocket "github.com/gerp93/gameshell-framework/websocket"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
	"github.com/gerp93/timeline-trivia/static"
)

// TimelineTrivia deck ID
var timelineTriviaDeckId = uuid.MustParse("88026803-d22a-11f0-b4d2-60cf84649547")

// defaultCardsToWin is the target a lobby gets when nothing else is specified.
const defaultCardsToWin = 10

// turnTimerGraceSeconds is added on top of the configured per-turn duration
// before a player can actually start losing real time. It exists because the
// result/win-celebration popup and the 3-2-1 turn countdown (see
// timeline-trivia.js) are shown independently on every client and can be
// clicked through at different speeds by different players — without a fixed
// grace window, whoever dismisses their popup fastest gets their own local
// clock started earliest, so the round could end while slower/spectating
// clients were still mid-animation. 8 seconds covers the longest any of
// those popups can naturally stay up on their own (5s "revealed" popup + 3s
// turn countdown), so a player who lets them run their full course — or
// clicks through instantly — always sees the same true remaining time; only
// someone who leaves a popup up longer than that (e.g. away from keyboard)
// actually starts losing seconds, and identically for every player.
const turnTimerGraceSeconds = 8

// turnSecondsRemaining computes how many seconds are left in the current
// turn from the server's own clock (TimelineTriviaGame.TurnElapsedSeconds,
// itself computed by the database — see that field's comment for why), so
// every client anchors its countdown to the same truth instead of each one
// starting a fresh local countdown whenever its own popup happens to clear.
// Returns 0 when there's no timer configured, no turn in progress, or time
// is already up.
func turnSecondsRemaining(timerSeconds int, turnElapsed sql.NullInt64) int {
	if timerSeconds <= 0 || !turnElapsed.Valid {
		return 0
	}
	elapsed := turnElapsed.Int64 - turnTimerGraceSeconds
	if elapsed < 0 {
		elapsed = 0
	}
	remaining := timerSeconds - int(elapsed)
	if remaining < 0 {
		remaining = 0
	}
	if remaining > timerSeconds {
		remaining = timerSeconds
	}
	return remaining
}

// resultPayload is the body of a "result:" websocket message — the popup and
// the bottom-of-screen status line every client shows after a guess resolves.
// It is JSON rather than the colon-delimited form it replaced: player names
// may contain colons, and the celebration fields have to ride along.
type resultPayload struct {
	PlayerName    string `json:"playerName"`
	Type          string `json:"type"` // correct | incorrect | revealed
	Message       string `json:"message"`
	BottomMessage string `json:"bottomMessage"`
	UserId        string `json:"userId,omitempty"`
	Celebration   string `json:"celebration,omitempty"`
	HasGif        bool   `json:"hasGif,omitempty"`
	// NextPlayerName is who the client should run its "next up" countdown
	// for once this popup clears. Omitted when there is no next turn — the
	// game just ended.
	NextPlayerName string `json:"nextPlayerName,omitempty"`
}

// announce posts a line to the lobby chat. Everything interpolated into msg
// must already be escaped (see esc) — the framework only escapes messages
// coming from players over the socket, and gsChat renders with innerHTML.
func announce(lobbyId uuid.UUID, msg string) {
	gsWebsocket.LobbyBroadcast(lobbyId, msg)
}

// esc makes player- or card-authored text safe to interpolate into a chat
// line. The <red>/<green>/<blue> color tokens are added by the caller after
// this, so they survive.
func esc(s string) string {
	return html.EscapeString(s)
}

// sendResult broadcasts a guess outcome to every client in the lobby. It is
// deliberately lobby-wide: players need to see what happened on other
// players' turns, not just their own.
func sendResult(lobbyId uuid.UUID, payload resultPayload) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		log.Println(err)
		return
	}
	gsWebsocket.LobbyBroadcast(lobbyId, "result:"+string(encoded))
}

// sendStatus updates the bottom-of-screen status line for every client
// without showing a popup, for game events that aren't a guess outcome (e.g.
// Skip & Remove). msg should read the same as the paired chat announcement
// (see announce), just without color tokens and already unescaped — the
// status line renders as plain text, not innerHTML.
func sendStatus(lobbyId uuid.UUID, msg string) {
	gsWebsocket.LobbyBroadcast(lobbyId, "status:"+msg)
}

// turnOrderNames renders a game's active players in turn order, for the
// "game started" chat line.
func turnOrderNames(gameId uuid.UUID) string {
	players, err := database.GetTimelineTriviaPlayers(gameId)
	if err != nil {
		log.Println(err)
		return ""
	}
	names := make([]string, 0, len(players))
	for _, p := range players {
		if p.IsActive {
			names = append(names, p.UserName)
		}
	}
	return strings.Join(names, ", ")
}

// currentPlayerName is whoever is on the hook to guess right now, for chat
// lines written after the turn has already advanced.
func currentPlayerName(gameId uuid.UUID) string {
	players, err := database.GetTimelineTriviaPlayers(gameId)
	if err != nil {
		log.Println(err)
		return "the next player"
	}
	for _, p := range players {
		if p.IsCurrent {
			return p.UserName
		}
	}
	return "the next player"
}

// winCelebrationFor loads a player's personalized win GIF/message, if they set
// one. Failures are non-fatal — the popup just falls back to its plain form.
func winCelebrationFor(userId uuid.UUID) (celebration string, hasGif bool) {
	c, err := gsDatabase.GetUserWinCelebration(userId)
	if err != nil {
		log.Println(err)
		return "", false
	}
	return c.Message.String, c.HasGif
}

// loseCelebrationFor is winCelebrationFor's counterpart, loaded for the
// player who just guessed wrong (not the "revealed" case, where every
// active player missed and there's no single "you" to show it to).
func loseCelebrationFor(userId uuid.UUID) (celebration string, hasGif bool) {
	c, err := gsDatabase.GetUserLoseCelebration(userId)
	if err != nil {
		log.Println(err)
		return "", false
	}
	return c.Message.String, c.HasGif
}

// ensureGameExists makes sure a TimelineTrivia game exists for a lobby, creating one if needed
func ensureGameExists(lobbyId uuid.UUID) (database.TimelineTriviaGame, error) {
	game, err := database.GetTimelineTriviaGame(lobbyId)
	if err == nil && game.Id != uuid.Nil {
		return game, nil
	}

	// Auto-create the game with default settings
	gameId, createErr := database.CreateTimelineTriviaGame(lobbyId, defaultCardsToWin, database.DefaultTimelineId)
	if createErr != nil {
		return game, createErr
	}

	// Initialize draw pile with the TimelineTrivia deck (cards use authored years)
	_ = database.InitializeTimelineTriviaDrawPile(gameId, []uuid.UUID{timelineTriviaDeckId}, nil)

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
	message := r.FormValue("message")

	cardsToWin := defaultCardsToWin
	if cardsToWinStr := r.FormValue("cardsToWin"); cardsToWinStr != "" {
		if val, err := strconv.Atoi(cardsToWinStr); err == nil && val > 0 {
			cardsToWin = val
		}
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

	timelineId, err := uuid.Parse(r.FormValue("timelineId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("a timeline is required"))
		return
	}
	timelineExists, err := database.TimelineExists(timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check timeline"))
		return
	}
	if !timelineExists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("the selected timeline does not exist"))
		return
	}

	// Defense in depth beyond the client-side filter: every selected deck
	// must actually belong to the chosen timeline.
	deckTimelineIds, err := database.GetDeckTimelineIds(deckIds)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check deck timelines"))
		return
	}
	for _, deckId := range deckIds {
		if deckTimelineIds[deckId] != timelineId {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("one or more selected decks do not belong to the chosen timeline"))
			return
		}
	}

	// Parse any year-range filters (parallel fromYear/toYear form arrays).
	// No ranges = no filter, matching ApplyYearRangeFilter's semantics.
	fromYears := r.Form["fromYear"]
	toYears := r.Form["toYear"]
	ranges := make([]database.TimelineTriviaYearRange, 0, len(fromYears))
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
		ranges = append(ranges, database.TimelineTriviaYearRange{FromYear: from, ToYear: to})
	}

	// Parse excluded category ids (all categories are included by default).
	excludedCategoryIdStrings := r.Form["excludedCategoryId"]
	excludedCategoryIds := make([]uuid.UUID, 0, len(excludedCategoryIdStrings))
	for _, idStr := range excludedCategoryIdStrings {
		if id, err := uuid.Parse(idStr); err == nil {
			excludedCategoryIds = append(excludedCategoryIds, id)
		}
	}

	// Safety check: cards to win must be realistic for what the selected
	// decks/year ranges/categories actually contain, before creating anything.
	totalCards, err := database.CountCardsInDecksForRanges(deckIds, ranges, excludedCategoryIds)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to count cards for the selected decks"))
		return
	}
	if err := database.ValidateCardsToWin(cardsToWin, totalCards); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	// Optional per-turn countdown (framework lobby setting; 0 = off)
	turnTimerSeconds := 0
	if turnTimerStr := r.FormValue("turnTimerSeconds"); turnTimerStr != "" {
		if val, err := strconv.Atoi(turnTimerStr); err == nil && val > 0 {
			turnTimerSeconds = val
		}
	}

	// Create the lobby with game_type = 'timeline-trivia'
	lobbyId, err := database.CreateTimelineTriviaLobby(name, message, password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to create lobby"))
		return
	}

	if turnTimerSeconds > 0 {
		if err := gsDatabase.SetLobbyTurnTimerSeconds(lobbyId, turnTimerSeconds); err != nil {
			log.Println(err)
		}
	}

	// Create the TimelineTrivia game
	gameId, err := database.CreateTimelineTriviaGame(lobbyId, cardsToWin, timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to create game"))
		return
	}

	// Initialize draw pile with cards from decks (cards use authored years)
	if err := database.InitializeTimelineTriviaDrawPile(gameId, deckIds, excludedCategoryIds); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to initialize draw pile"))
		return
	}

	for _, rg := range ranges {
		if err := database.AddYearRange(gameId, rg.FromYear, rg.ToYear); err != nil {
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

	// The order is freshly shuffled, so tell everyone what it came out as.
	announce(lobbyId, fmt.Sprintf("<blue>Game started</> — turn order: %s", esc(turnOrderNames(game.Id))))

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

	announce(lobbyId, "<blue>New game</> — press Start Game to deal and reshuffle the turn order.")

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

	// A game only ever draws from decks belonging to one timeline, so its
	// eras are fetched once and reused for every year formatted below.
	eras, erasErr := database.GetErasForTimeline(game.TimelineId)
	if erasErr != nil {
		log.Println(erasErr)
	}

	// Get position from form
	positionStr := r.FormValue("position")
	position, err := strconv.Atoi(positionStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid position"))
		return
	}

	// Capture the card being guessed before the attempt resolves and clears
	// the current card, so the guess can be logged for stats regardless of the
	// outcome.
	guessedCard, _ := database.GetTimelineTriviaCurrentCard(game.Id)

	// Attempt to place the card (the "steal" mechanic: a miss doesn't end the
	// round, it just passes the same card to the next player who hasn't
	// tried it yet)
	correct, roundExhausted, err := database.AttemptPlaceCardInTimeline(game.Id, player.Id, position)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	// Log the guess for stats (non-fatal to gameplay on failure).
	if guessedCard.CardId != uuid.Nil {
		if logErr := database.LogGuess(userId, guessedCard.CardId, guessedCard.CardYear, correct); logErr != nil {
			log.Println(logErr)
		}
	}

	guessedYear := database.FormatYearInEras(guessedCard.CardYear, eras)

	if correct {
		celebration, hasGif := winCelebrationFor(userId)

		// Check for winner
		winnerId, err := database.CheckTimelineTriviaWinner(game.Id)
		if err == nil && winnerId != uuid.Nil {
			// Game over! Record the win for stats (non-fatal on failure).
			if logErr := database.LogWin(winnerId, game.TimelineId); logErr != nil {
				log.Println(logErr)
			}
			announce(lobbyId, fmt.Sprintf(
				"<green>%s placed \"%s\" correctly — it was %s.</>",
				esc(player.Name), esc(guessedCard.CardText), esc(guessedYear),
			))
			announce(lobbyId, fmt.Sprintf("<green>%s wins the game!</>", esc(player.Name)))
			sendResult(lobbyId, resultPayload{
				PlayerName:    player.Name,
				Type:          "correct",
				Message:       "Wins the game!",
				BottomMessage: fmt.Sprintf("%s wins the game!", player.Name),
				UserId:        userId.String(),
				Celebration:   celebration,
				HasGif:        hasGif,
			})
			gsWebsocket.LobbyBroadcast(lobbyId, "reload")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("You win!"))
			return
		}

		announce(lobbyId, fmt.Sprintf(
			"<green>%s placed \"%s\" correctly — it was %s.</>",
			esc(player.Name), esc(guessedCard.CardText), esc(guessedYear),
		))

		// Round resolved: next round starts with the next active player
		// after this round's starter, and a fresh card is drawn.
		if err := database.ResolveCardRound(game.Id); err != nil {
			gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Correct! No more cards."))
			return
		}

		sendResult(lobbyId, resultPayload{
			PlayerName:     player.Name,
			Type:           "correct",
			Message:        fmt.Sprintf("Correct! It was %s.", guessedYear),
			BottomMessage:  fmt.Sprintf("%s placed \"%s\" correctly — it was %s.", player.Name, guessedCard.CardText, guessedYear),
			UserId:         userId.String(),
			Celebration:    celebration,
			HasGif:         hasGif,
			NextPlayerName: currentPlayerName(game.Id),
		})
		gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Correct! Next round begins."))
		return
	}

	if roundExhausted {
		// Every active player missed this card; it's discarded. Reveal the
		// year it actually was before ResolveCardRound clears the current
		// card and draws the next one.
		revealedCard, _ := database.GetTimelineTriviaCurrentCard(game.Id)
		revealedYear := database.FormatYearInEras(revealedCard.CardYear, eras)

		// Record the discard for stats (non-fatal on failure).
		if revealedCard.CardId != uuid.Nil {
			if logErr := database.LogCardDiscard(revealedCard.CardId); logErr != nil {
				log.Println(logErr)
			}
		}

		announce(lobbyId, fmt.Sprintf(
			"<red>Nobody got \"%s\" — it was %s. Card discarded.</>",
			esc(revealedCard.CardText), esc(revealedYear),
		))

		// Next round starts with the next active player after this round's
		// starter.
		if err := database.ResolveCardRound(game.Id); err != nil {
			gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Incorrect. No more cards."))
			return
		}

		sendResult(lobbyId, resultPayload{
			PlayerName:     player.Name,
			Type:           "revealed",
			Message:        fmt.Sprintf("Everyone missed! It was %s. Card discarded.", revealedYear),
			BottomMessage:  fmt.Sprintf("Nobody got \"%s\" — it was %s. Card discarded.", revealedCard.CardText, revealedYear),
			NextPlayerName: currentPlayerName(game.Id),
		})
		gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(
			"Incorrect. Everyone missed — card discarded.\n%s: %s",
			revealedYear, revealedCard.CardText,
		)))
		return
	}

	// Hand the same card to the next active player who hasn't tried it yet
	if err := database.AdvanceToNextGuesser(game.Id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to advance to next guesser"))
		return
	}

	// Deliberately no year here — the card is still in play and revealing it
	// would spoil the steal.
	nextName := currentPlayerName(game.Id)
	announce(lobbyId, fmt.Sprintf(
		"<red>%s guessed wrong on \"%s\" — %s can steal it.</>",
		esc(player.Name), esc(guessedCard.CardText), esc(nextName),
	))
	loseCelebration, loseHasGif := loseCelebrationFor(userId)
	sendResult(lobbyId, resultPayload{
		PlayerName:     player.Name,
		Type:           "incorrect",
		Message:        "Wrong! Next player can steal it.",
		BottomMessage:  fmt.Sprintf("%s guessed wrong on \"%s\" — %s can steal it.", player.Name, guessedCard.CardText, nextName),
		UserId:         userId.String(),
		Celebration:    loseCelebration,
		HasGif:         loseHasGif,
		NextPlayerName: nextName,
	})
	gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Incorrect. Next player can try to steal this card."))
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

	// Whose turn it is to guess right now (own turn or stealing alike),
	// shown once above the player list instead of per-row — see
	// database.TimelineTriviaPlayerTimeline.IsCurrent / HasAttempt.
	var currentPlayerName string
	var missedNames []string
	for _, p := range allTimelines {
		if p.IsCurrent {
			currentPlayerName = p.PlayerName
		}
		if p.HasAttempt {
			missedNames = append(missedNames, p.PlayerName)
		}
	}
	isSteal := game.CurrentPlayerId.Valid && game.RoundStarterPlayerId.Valid &&
		game.CurrentPlayerId.UUID != game.RoundStarterPlayerId.UUID

	timerSeconds, timerErr := gsDatabase.GetLobbyTurnTimerSeconds(lobbyId)
	if timerErr != nil {
		log.Println(timerErr)
	}
	secondsRemaining := turnSecondsRemaining(timerSeconds, game.TurnElapsedSeconds)

	eras, erasErr := database.GetErasForTimeline(game.TimelineId)
	if erasErr != nil {
		log.Println(erasErr)
	}
	funcMap := template.FuncMap{
		"add":        func(a, b int) int { return a + b },
		"formatYear": func(year int) string { return database.FormatYearInEras(year, eras) },
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
		AllTimelines      []database.TimelineTriviaPlayerTimeline
		IsMyTurn          bool
		GameStatus        string
		LobbyId           uuid.UUID
		CurrentPlayerName string
		IsSteal           bool
		MissedByNames     string
		SecondsRemaining  int
	}

	_ = tmpl.Execute(w, data{
		AllTimelines:      allTimelines,
		IsMyTurn:          isMyTurn,
		GameStatus:        game.GameStatus,
		LobbyId:           lobbyId,
		CurrentPlayerName: currentPlayerName,
		IsSteal:           isSteal,
		MissedByNames:     strings.Join(missedNames, ", "),
		SecondsRemaining:  secondsRemaining,
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
	attempts, _ := database.GetCardAttempts(game.Id)

	// Only the player currently on the hook gets the "skip and remove"
	// control, so the fragment has to know who is viewing it.
	isCurrentGuesser := false
	if game.CurrentPlayerId.Valid {
		viewer, err := gsDatabase.GetLobbyUserPlayer(lobbyId, gsApi.GetUserId(r))
		if err == nil {
			isCurrentGuesser = viewer.Id == game.CurrentPlayerId.UUID
		}
	}

	// A steal in progress (current guesser isn't who the round started with)
	// means someone already guessed wrong on this exact card — flagging it
	// now would erase evidence it might just be a hard card, not a bad one.
	// Skip & Remove is only offered to whoever the round opened with.
	isSteal := game.CurrentPlayerId.Valid && game.RoundStarterPlayerId.Valid &&
		game.CurrentPlayerId.UUID != game.RoundStarterPlayerId.UUID

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/components/timeline-trivia/current-card.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse template"))
		return
	}

	type data struct {
		database.TimelineTriviaCurrentCard
		Attempts         []database.TimelineTriviaCardAttempt
		LobbyId          uuid.UUID
		IsCurrentGuesser bool
		IsSteal          bool
	}

	_ = tmpl.Execute(w, data{
		TimelineTriviaCurrentCard: currentCard,
		Attempts:                  attempts,
		LobbyId:                   lobbyId,
		IsCurrentGuesser:          isCurrentGuesser,
		IsSteal:                   isSteal,
	})
}

// GetDecks returns the deck-breakdown tooltip shown next to the lobby's card
// counts.
func GetDecks(w http.ResponseWriter, r *http.Request) {
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

	decks, err := database.GetTimelineTriviaGameDecks(game.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get decks"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/components/timeline-trivia/deck-info.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse template"))
		return
	}

	_ = tmpl.Execute(w, decks)
}

// SkipAndRemoveCard takes a bad current card out of play and sends it to admin
// review. Only the player being asked to guess it can do this, and the round
// restarts cleanly with a replacement card rather than penalizing anyone.
func SkipAndRemoveCard(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	userId := gsApi.GetUserId(r)

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

	if !game.CurrentPlayerId.Valid || game.CurrentPlayerId.UUID != player.Id {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("only the current guesser can remove this card"))
		return
	}

	// A steal in progress means someone already guessed wrong on this exact
	// card; only whoever the round opened with may flag it (see GetCurrentCard).
	if game.RoundStarterPlayerId.Valid && game.CurrentPlayerId.UUID != game.RoundStarterPlayerId.UUID {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("this card already has a guess on it and can no longer be removed"))
		return
	}

	currentCard, err := database.GetTimelineTriviaCurrentCard(game.Id)
	if err != nil || currentCard.CardId == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no card to remove"))
		return
	}

	if err := database.FlagCardAndSkip(game.Id, currentCard.CardId, userId, lobbyId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to remove card: " + err.Error()))
		return
	}

	announce(lobbyId, fmt.Sprintf(
		"<red>%s</> flagged \"%s\" for review — removed from this game.",
		esc(player.Name), esc(currentCard.CardText),
	))
	sendStatus(lobbyId, fmt.Sprintf(
		"%s flagged \"%s\" for review — removed from this game.",
		player.Name, currentCard.CardText,
	))
	gsWebsocket.LobbyBroadcast(lobbyId, "refresh")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Card removed and sent for review."))
}

// TimeoutPass is posted by the current guesser's own browser when their turn
// timer runs out. They lose their shot at this card with no penalty — no
// guess is recorded against their timeline — and it passes on.
func TimeoutPass(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	userId := gsApi.GetUserId(r)

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

	// A stale client whose turn already passed must not skip anyone else.
	if game.GameStatus != "active" || !game.CurrentPlayerId.Valid || game.CurrentPlayerId.UUID != player.Id {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("not your turn"))
		return
	}

	// Capture the card and the timer setting in force before the pass
	// resolves, mirroring PlaceCard's guessedCard snapshot — the round may
	// end below, and the timer can be changed mid-game, so neither is
	// reliably recoverable afterwards.
	timedOutCard, _ := database.GetTimelineTriviaCurrentCard(game.Id)
	timerSeconds, timerErr := gsDatabase.GetLobbyTurnTimerSeconds(lobbyId)
	if timerErr != nil {
		log.Println(timerErr)
	}

	roundExhausted, err := database.RecordTimeoutPass(game.Id, player.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to record timeout"))
		return
	}

	// Log the timeout for stats (non-fatal to gameplay on failure).
	if timedOutCard.CardId != uuid.Nil {
		if logErr := database.LogTimeout(userId, timedOutCard.CardId, timerSeconds); logErr != nil {
			log.Println(logErr)
		}
	}

	if roundExhausted {
		// Nobody guessed this card in time; it's discarded like any other
		// exhausted round, with the answer revealed. Matches PlaceCard's
		// exhausted branch: the final miss (here, timeout) doesn't get its
		// own chat line, it just folds into "Nobody got it".
		revealedCard, _ := database.GetTimelineTriviaCurrentCard(game.Id)
		eras, erasErr := database.GetErasForTimeline(game.TimelineId)
		if erasErr != nil {
			log.Println(erasErr)
		}
		revealedYear := database.FormatYearInEras(revealedCard.CardYear, eras)

		if revealedCard.CardId != uuid.Nil {
			if logErr := database.LogCardDiscard(revealedCard.CardId); logErr != nil {
				log.Println(logErr)
			}
		}

		announce(lobbyId, fmt.Sprintf(
			"<red>Nobody got \"%s\" — it was %s. Card discarded.</>",
			esc(revealedCard.CardText), esc(revealedYear),
		))

		if err := database.ResolveCardRound(game.Id); err != nil {
			gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Time's up. No more cards."))
			return
		}

		sendResult(lobbyId, resultPayload{
			PlayerName:     player.Name,
			Type:           "revealed",
			Message:        fmt.Sprintf("Time ran out! It was %s. Card discarded.", revealedYear),
			BottomMessage:  fmt.Sprintf("Nobody got \"%s\" — it was %s. Card discarded.", revealedCard.CardText, revealedYear),
			NextPlayerName: currentPlayerName(game.Id),
		})
		gsWebsocket.LobbyBroadcast(lobbyId, "refresh")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Time's up. Card discarded."))
		return
	}

	if err := database.AdvanceToNextGuesser(game.Id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to advance to next guesser"))
		return
	}

	nextName := currentPlayerName(game.Id)
	announce(lobbyId, fmt.Sprintf(
		"<red>%s ran out of time — %s can steal it.</>",
		esc(player.Name), esc(nextName),
	))
	sendResult(lobbyId, resultPayload{
		PlayerName:     player.Name,
		Type:           "incorrect",
		Message:        "Out of time!",
		BottomMessage:  fmt.Sprintf("%s ran out of time — %s can steal it.", player.Name, nextName),
		NextPlayerName: nextName,
	})
	gsWebsocket.LobbyBroadcast(lobbyId, "refresh")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Time's up. Passing to the next player."))
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

// CardCount returns, as plain text, how many cards across the given decks
// would end up in the draw pile — restricted to the given year range(s) if
// any are provided, or every year otherwise, and excluding any given
// categories. Used by the lobby-creation form for a live estimate: called
// once with every currently-entered range for the "Select Decks" total, and
// once per range (with just that range) for each range row's own count.
func CardCount(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("0"))
		return
	}

	deckIdStrings := r.Form["deckId"]
	deckIds := make([]uuid.UUID, 0, len(deckIdStrings))
	for _, s := range deckIdStrings {
		if id, err := uuid.Parse(s); err == nil {
			deckIds = append(deckIds, id)
		}
	}
	if len(deckIds) == 0 {
		_, _ = w.Write([]byte("0"))
		return
	}

	fromYears := r.Form["fromYear"]
	toYears := r.Form["toYear"]
	ranges := make([]database.TimelineTriviaYearRange, 0, len(fromYears))
	for i := range fromYears {
		if i >= len(toYears) {
			break
		}
		if fromYears[i] == "" || toYears[i] == "" {
			continue
		}
		from, fromErr := strconv.Atoi(fromYears[i])
		to, toErr := strconv.Atoi(toYears[i])
		if fromErr != nil || toErr != nil {
			continue // ignore invalid rows for a live estimate rather than erroring
		}
		if from > to {
			from, to = to, from
		}
		ranges = append(ranges, database.TimelineTriviaYearRange{FromYear: from, ToYear: to})
	}

	excludedCategoryIdStrings := r.Form["excludedCategoryId"]
	excludedCategoryIds := make([]uuid.UUID, 0, len(excludedCategoryIdStrings))
	for _, s := range excludedCategoryIdStrings {
		if id, err := uuid.Parse(s); err == nil {
			excludedCategoryIds = append(excludedCategoryIds, id)
		}
	}

	count, err := database.CountCardsInDecksForRanges(deckIds, ranges, excludedCategoryIds)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("0"))
		return
	}

	_, _ = w.Write([]byte(strconv.Itoa(count)))
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

// SetLobbyMessage sets the lobby's welcome message, shown to players on entry
func SetLobbyMessage(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid lobby id"))
		return
	}

	userId := gsApi.GetUserId(r)

	player, err := gsDatabase.GetLobbyUserPlayer(lobbyId, userId)
	if err != nil || player.Id == uuid.Nil {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("not a player in this lobby"))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("failed to parse form"))
		return
	}

	message := r.FormValue("message")

	if err := gsDatabase.SetLobbyMessage(lobbyId, message); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to set lobby message"))
		return
	}

	gsWebsocket.LobbyBroadcast(lobbyId, fmt.Sprintf("<green>%s</>: Lobby message updated", player.Name))
	gsWebsocket.LobbyBroadcast(lobbyId, "lobbyMessage:"+message)
}
