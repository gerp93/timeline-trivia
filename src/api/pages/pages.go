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

	"github.com/gerp93/card-timeline/database"
	"github.com/gerp93/card-timeline/static"
)

func Home(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Chronology - Home"

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
	basePageData.PageTitle = "Chronology - About"

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
	basePageData.PageTitle = "Chronology - Login"

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
	basePageData.PageTitle = "Chronology - Account"

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

	_ = tmpl.ExecuteTemplate(w, "base", basePageData)
}

func Users(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Chronology - Users"

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
	basePageData.PageTitle = "Chronology - Decks"

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

	totalRowCount, err := database.CountDecks(name)
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

	decks, err := database.SearchDecks(name, page)
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
		Decks    []database.DeckDetails
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

	deck, err := database.GetDeck(deckId)
	if err != nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	if deck.Id == uuid.Nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Chronology - Deck"

	hasDeckAccess, err := database.UserHasDeckAccess(basePageData.User.Id, deckId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check deck access"))
		return
	}

	if !hasDeckAccess {
		http.Redirect(w, r, fmt.Sprintf("/deck/%s/access", deckId), http.StatusSeeOther)
		return
	}

	var category string
	var text string
	var page int
	params := r.URL.Query()
	for key, val := range params {
		switch key {
		case "category":
			category = val[0]
		case "text":
			text = val[0]
		case "page":
			page, _ = strconv.Atoi(val[0])
		}
	}

	totalRowCount, err := database.CountCardsInDeck(deckId, category, text)
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

	cards, err := database.SearchCardsInDeck(deckId, category, text, page)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get table rows"))
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
		Deck     database.Deck
		Category string
		Text     string
		Page     int
		LastPage int
		RowCount int
		Cards    []database.Card
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Deck:         deck,
		Category:     category,
		Text:         text,
		Page:         page,
		LastPage:     totalPageCount,
		RowCount:     totalRowCount,
		Cards:        cards,
	})
}

func DeckAccess(w http.ResponseWriter, r *http.Request) {
	deckIdString := r.PathValue("deckId")
	deckId, err := uuid.Parse(deckIdString)
	if err != nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	deck, err := database.GetDeck(deckId)
	if err != nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	if deck.Id == uuid.Nil {
		http.Redirect(w, r, "/decks", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Chronology - Deck"

	hasDeckAccess, err := database.UserHasDeckAccess(basePageData.User.Id, deckId)
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
		Deck database.Deck
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Deck:         deck,
	})
}

// ChronologyLobbies displays the list of Chronology games
func ChronologyLobbies(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Chronology - Games"

	// Get readable decks for the current user
	decks, err := database.GetReadableDecks(basePageData.User.Id)
	if err != nil {
		decks = make([]database.Deck, 0)
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/chronology-lobbies.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Decks []database.Deck
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Decks:        decks,
	})
}

// ChronologyLobby displays a specific Chronology game
func ChronologyLobby(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
		return
	}

	lobby, err := database.GetLobby(lobbyId)
	if err != nil {
		http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
		return
	}

	if lobby.Id == uuid.Nil {
		http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Chronology - Game"

	hasLobbyAccess, err := gsDatabase.UserHasLobbyAccess(basePageData.User.Id, lobbyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check lobby access"))
		return
	}

	if !hasLobbyAccess {
		http.Redirect(w, r, fmt.Sprintf("/chronology/%s/access", lobbyId), http.StatusSeeOther)
		return
	}

	// Get or create player for this user
	playerId, err := gsDatabase.AddUserToLobby(lobbyId, basePageData.User.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to join lobby"))
		return
	}

	// Get the Chronology game - auto-create if it doesn't exist
	game, err := database.GetChronologyGame(lobbyId)
	if err != nil || game.Id == uuid.Nil {
		log.Printf("[INFO ChronologyLobby] Game not found for lobby %s, auto-creating...", lobbyId)
		// Auto-create the game with default settings
		gameId, createErr := database.CreateChronologyGame(lobbyId, 5) // 5 cards to win default
		if createErr != nil {
			log.Printf("[ERROR ChronologyLobby] Failed to auto-create game for lobby %s: %v", lobbyId, createErr)
			http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
			return
		}
		// Initialize draw pile with the Chronology deck
		chronologyDeckId, _ := uuid.Parse("88026803-d22a-11f0-b4d2-60cf84649547")
		if initErr := database.InitializeChronologyDrawPile(gameId, []uuid.UUID{chronologyDeckId}); initErr != nil {
			log.Printf("[ERROR ChronologyLobby] Failed to initialize draw pile for lobby %s: %v", lobbyId, initErr)
		}
		// Parse years from card text
		if yearErr := database.UpdateDrawPileYears(gameId); yearErr != nil {
			log.Printf("[ERROR ChronologyLobby] Failed to update draw pile years for lobby %s: %v", lobbyId, yearErr)
		}
		// Re-fetch the game
		game, err = database.GetChronologyGame(lobbyId)
		if err != nil || game.Id == uuid.Nil {
			log.Printf("[ERROR ChronologyLobby] Still no game after auto-create for lobby %s", lobbyId)
			http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
			return
		}
		log.Printf("[INFO ChronologyLobby] Auto-created game %s for lobby %s", game.Id, lobbyId)
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

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/chronology.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Lobby             database.Lobby
		Game              database.ChronologyGame
		PlayerId          uuid.UUID
		CurrentPlayerName string
		IsMyTurn          bool
		WinnerName        string
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:      basePageData,
		Lobby:             lobby,
		Game:              game,
		PlayerId:          playerId,
		CurrentPlayerName: currentPlayerName,
		IsMyTurn:          isMyTurn,
		WinnerName:        winnerName,
	})
}

// ChronologyLobbyAccess displays the access page for a Chronology game
func ChronologyLobbyAccess(w http.ResponseWriter, r *http.Request) {
	lobbyIdString := r.PathValue("lobbyId")
	lobbyId, err := uuid.Parse(lobbyIdString)
	if err != nil {
		http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
		return
	}

	lobby, err := database.GetLobby(lobbyId)
	if err != nil {
		http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
		return
	}

	if lobby.Id == uuid.Nil {
		http.Redirect(w, r, "/chronology/lobbies", http.StatusSeeOther)
		return
	}

	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Chronology - Access"

	hasLobbyAccess, err := gsDatabase.UserHasLobbyAccess(basePageData.User.Id, lobbyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check lobby access"))
		return
	}

	if hasLobbyAccess {
		http.Redirect(w, r, fmt.Sprintf("/chronology/%s", lobbyId), http.StatusSeeOther)
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
