package apiPages

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
	"github.com/gerp93/timeline-trivia/static"
)

func Home(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Home"

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/home.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	_ = tmpl.ExecuteTemplate(w, "base", basePageData)
}

func About(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - About"

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/about.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	_ = tmpl.ExecuteTemplate(w, "base", basePageData)
}

func Login(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Login"

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/login.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	_ = tmpl.ExecuteTemplate(w, "base", basePageData)
}

func Account(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Account"

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/account.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		ThemeGroups []gsApi.ThemeGroup
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		ThemeGroups:  gsApi.ThemeGroups,
	})
}

func Categories(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Categories"

	categories, err := database.GetCategoriesWithCounts()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get categories"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/categories.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Categories []database.CategoryWithCount
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Categories:   categories,
	})
}

func Users(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Users"

	var name string
	var page int
	params := r.URL.Query()
	for key, val := range params {
		switch key {
		case "name":
			name = val[0]
		case "page":
			page, _ = strconv.Atoi(val[0])
		}
	}

	totalRowCount, err := gsDatabase.CountUsers(name)
	if err != nil {
		totalRowCount = 0
	}
	totalPageCount := max((totalRowCount+9)/10, 1)

	if page < 1 {
		page = 1
	}

	if page > totalPageCount {
		page = totalPageCount
	}

	users, err := gsDatabase.SearchUsers(name, page)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get table rows"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/users.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Name     string
		Page     int
		LastPage int
		RowCount int
		Users    []gsDatabase.User
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Name:         name,
		Page:         page,
		LastPage:     totalPageCount,
		RowCount:     totalRowCount,
		Users:        users,
	})
}

func Decks(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Decks"

	var name string
	var page int
	params := r.URL.Query()
	for key, val := range params {
		switch key {
		case "name":
			name = val[0]
		case "page":
			page, _ = strconv.Atoi(val[0])
		}
	}

	totalRowCount, err := gsDatabase.CountDecks(name)
	if err != nil {
		totalRowCount = 0
	}
	totalPageCount := max((totalRowCount+9)/10, 1)

	if page < 1 {
		page = 1
	}

	if page > totalPageCount {
		page = totalPageCount
	}

	decks, err := gsDatabase.SearchDecks(name, page)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get table rows"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/decks.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Name     string
		Page     int
		LastPage int
		RowCount int
		Decks    []gsDatabase.DeckDetails
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Name:         name,
		Page:         page,
		LastPage:     totalPageCount,
		RowCount:     totalRowCount,
		Decks:        decks,
	})
}

func Deck(w http.ResponseWriter, r *http.Request) {
	deckIdString := r.PathValue("deckId")
	deckId, err := uuid.Parse(deckIdString)
	if err != nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	deck, err := gsDatabase.GetDeck(deckId)
	if err != nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	if deck.Id == uuid.Nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Deck"

	hasDeckAccess, err := gsDatabase.UserHasDeckAccess(basePageData.User.Id, deckId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check deck access"))
		return
	}

	if !hasDeckAccess {
		http.Redirect(w, r, fmt.Sprintf("/deck/%s/access", deckId), http.StatusSeeOther)
		return
	}

	var text string
	var page int
	params := r.URL.Query()
	for key, val := range params {
		switch key {
		case "text":
			text = val[0]
		case "page":
			page, _ = strconv.Atoi(val[0])
		}
	}

	totalRowCount, err := database.CountCardsInDeck(deckId, text)
	if err != nil {
		totalRowCount = 0
	}
	totalPageCount := max((totalRowCount+9)/10, 1)

	if page < 1 {
		page = 1
	}

	if page > totalPageCount {
		page = totalPageCount
	}

	cards, err := database.SearchCardsInDeck(deckId, text, page)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get table rows"))
		return
	}

	categories, err := database.GetCategories()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get categories"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/deck.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Deck       gsDatabase.Deck
		Text       string
		Page       int
		LastPage   int
		RowCount   int
		Cards      []database.Card
		Categories []database.Category
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Deck:         deck,
		Text:         text,
		Page:         page,
		LastPage:     totalPageCount,
		RowCount:     totalRowCount,
		Cards:        cards,
		Categories:   categories,
	})
}

func DeckAccess(w http.ResponseWriter, r *http.Request) {
	deckIdString := r.PathValue("deckId")
	deckId, err := uuid.Parse(deckIdString)
	if err != nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	deck, err := gsDatabase.GetDeck(deckId)
	if err != nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	if deck.Id == uuid.Nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Deck"

	hasDeckAccess, err := gsDatabase.UserHasDeckAccess(basePageData.User.Id, deckId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check deck access"))
		return
	}

	if hasDeckAccess {
		http.Redirect(w, r, fmt.Sprintf("/deck/%s", deckId), http.StatusSeeOther)
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/deck-access.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Deck gsDatabase.Deck
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Deck:         deck,
	})
}

// TimelineTriviaLobbies displays the list of TimelineTrivia games
func TimelineTriviaLobbies(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Games"

	// Get readable decks for the current user
	decks, err := gsDatabase.GetReadableDecks(basePageData.User.Id)
	if err != nil {
		decks = make([]gsDatabase.Deck, 0)
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/timeline-trivia-lobbies.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Decks               []gsDatabase.Deck
		MinCardsPerWinRatio int
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:        basePageData,
		Decks:               decks,
		MinCardsPerWinRatio: database.MinCardsPerWinRatio,
	})
}

// TimelineTriviaLobby displays a specific TimelineTrivia game
func TimelineTriviaLobby(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
		return
	}

	lobby, err := database.GetLobby(lobbyId)
	if err != nil {
		http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
		return
	}

	if lobby.Id == uuid.Nil {
		http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Game"

	hasLobbyAccess, err := gsDatabase.UserHasLobbyAccess(basePageData.User.Id, lobbyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check lobby access"))
		return
	}

	if !hasLobbyAccess {
		http.Redirect(w, r, fmt.Sprintf("/timeline-trivia/%s/access", lobbyId), http.StatusSeeOther)
		return
	}

	// Get or create player for this user
	playerId, err := gsDatabase.AddUserToLobby(lobbyId, basePageData.User.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to join lobby"))
		return
	}

	// Get the TimelineTrivia game - auto-create if it doesn't exist
	game, err := database.GetTimelineTriviaGame(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		log.Printf("[INFO TimelineTriviaLobby] Game not found for lobby %s, auto-creating...", lobbyId)
		// Auto-create the game with default settings
		gameId, createErr := database.CreateTimelineTriviaGame(lobbyId, 5) // 5 cards to win default
		if createErr != nil {
			log.Printf("[ERROR TimelineTriviaLobby] Failed to auto-create game for lobby %s: %v", lobbyId, createErr)
			http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
			return
		}
		// Initialize draw pile with the TimelineTrivia deck (cards use authored years)
		timelineTriviaDeckId, _ := uuid.Parse("88026803-d22a-11f0-b4d2-60cf84649547")
		if initErr := database.InitializeTimelineTriviaDrawPile(gameId, []uuid.UUID{timelineTriviaDeckId}); initErr != nil {
			log.Printf("[ERROR TimelineTriviaLobby] Failed to initialize draw pile for lobby %s: %v", lobbyId, initErr)
		}
		// Re-fetch the game
		game, err = database.GetTimelineTriviaGame(lobbyId)
		if err != nil || game.Id == uuid.Nil {
			log.Printf("[ERROR TimelineTriviaLobby] Still no game after auto-create for lobby %s", lobbyId)
			http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
			return
		}
		log.Printf("[INFO TimelineTriviaLobby] Auto-created game %s for lobby %s", game.Id, lobbyId)
	}

	// Get current player name if game is active
	var currentPlayerName string
	var isMyTurn bool
	if game.CurrentPlayerId.Valid {
		player, _ := gsDatabase.GetPlayer(game.CurrentPlayerId.UUID)
		currentPlayerName = player.Name
		isMyTurn = game.CurrentPlayerId.UUID == playerId
	}

	// Get winner name if game is finished
	var winnerName string
	if game.WinnerId.Valid {
		user, _ := gsDatabase.GetUser(game.WinnerId.UUID)
		winnerName = user.Name
	}

	yearRanges, err := database.GetYearRanges(game.Id)
	if err != nil {
		log.Printf("[ERROR TimelineTriviaLobby] Failed to get year ranges for game %s: %v", game.Id, err)
	}

	funcMap := template.FuncMap{
		"formatYear": database.FormatYear,
	}

	tmpl, err := template.New("base.html").Funcs(funcMap).ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/timeline-trivia.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Lobby             database.Lobby
		Game              database.TimelineTriviaGame
		PlayerId          uuid.UUID
		CurrentPlayerName string
		IsMyTurn          bool
		WinnerName        string
		YearRanges        []database.TimelineTriviaYearRange
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:      basePageData,
		Lobby:             lobby,
		Game:              game,
		PlayerId:          playerId,
		CurrentPlayerName: currentPlayerName,
		IsMyTurn:          isMyTurn,
		WinnerName:        winnerName,
		YearRanges:        yearRanges,
	})
}

// TimelineTriviaLobbyAccess displays the access page for a TimelineTrivia game
func TimelineTriviaLobbyAccess(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
		return
	}

	lobby, err := database.GetLobby(lobbyId)
	if err != nil {
		http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
		return
	}

	if lobby.Id == uuid.Nil {
		http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Access"

	hasLobbyAccess, err := gsDatabase.UserHasLobbyAccess(basePageData.User.Id, lobbyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check lobby access"))
		return
	}

	if hasLobbyAccess {
		http.Redirect(w, r, fmt.Sprintf("/timeline-trivia/%s", lobbyId), http.StatusSeeOther)
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/lobby-access.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Lobby database.Lobby
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Lobby:        lobby,
	})
}
