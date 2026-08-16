package apiPages

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsApiPages "github.com/gerp93/gameshell-framework/api/pages"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsStatic "github.com/gerp93/gameshell-framework/static"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
	"github.com/gerp93/timeline-trivia/static"
)

// parseChrome composes the framework's shared base.html with one of this
// game's own body files. Two ParseFS calls, not one — base.html lives in
// the framework's embed.FS, the body file in this game's own.
func parseChrome(bodyPattern string, funcMap template.FuncMap) (*template.Template, error) {
	t := template.New("base.html")
	if funcMap != nil {
		t = t.Funcs(funcMap)
	}
	t, err := t.ParseFS(gsStatic.StaticFiles, "html/pages/base.html")
	if err != nil {
		return nil, err
	}
	return t.ParseFS(static.StaticFiles, bodyPattern)
}

func Home(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Home"

	tmpl, err := parseChrome("html/pages/body/home.html", nil)
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

	tmpl, err := parseChrome("html/pages/body/about.html", nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	_ = tmpl.ExecuteTemplate(w, "base", basePageData)
}

// FlaggedCards is the admin review screen for cards players pulled out of
// play mid-game ("skip and remove"). Flagged cards are excluded from every
// draw pile until they are accepted, edited and accepted, or deleted here.
func FlaggedCards(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Flagged Cards"

	cards, err := database.GetFlaggedCards()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get flagged cards"))
		return
	}

	// Flagged cards can come from decks belonging to different timelines,
	// so era and category resolution has to happen per-card rather than
	// with one shared list — batch-resolve each distinct deck's timeline,
	// then each distinct timeline's eras and categories.
	deckIds := make([]uuid.UUID, 0, len(cards))
	seenDeck := make(map[uuid.UUID]bool, len(cards))
	for _, c := range cards {
		if !seenDeck[c.DeckId] {
			seenDeck[c.DeckId] = true
			deckIds = append(deckIds, c.DeckId)
		}
	}
	deckTimelineIds, err := database.GetDeckTimelineIds(deckIds)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get deck timelines"))
		return
	}
	erasByTimeline := make(map[uuid.UUID][]database.Era, len(deckTimelineIds))
	categoriesByTimeline := make(map[uuid.UUID][]database.Category, len(deckTimelineIds))
	for _, timelineId := range deckTimelineIds {
		if _, ok := erasByTimeline[timelineId]; !ok {
			eras, err := database.GetErasForTimeline(timelineId)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("failed to get eras"))
				return
			}
			erasByTimeline[timelineId] = eras
		}
		if _, ok := categoriesByTimeline[timelineId]; !ok {
			categories, err := database.GetCategoriesForTimeline(timelineId)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("failed to get categories"))
				return
			}
			categoriesByTimeline[timelineId] = categories
		}
	}

	funcMap := template.FuncMap{
		// Cards awaiting review may still be missing a year — that's often
		// exactly why they were flagged.
		"formatNullYear": func(deckId uuid.UUID, year sql.NullInt64) string {
			if !year.Valid {
				return "—"
			}
			return database.FormatYearInEras(int(year.Int64), erasByTimeline[deckTimelineIds[deckId]])
		},
		"erasForDeck": func(deckId uuid.UUID) []database.Era {
			return erasByTimeline[deckTimelineIds[deckId]]
		},
		"categoriesForDeck": func(deckId uuid.UUID) []database.Category {
			return categoriesByTimeline[deckTimelineIds[deckId]]
		},
		"eraIdForCard": func(deckId uuid.UUID, year sql.NullInt64) string {
			if !year.Valid {
				return ""
			}
			if era, ok := database.FindEraForYear(int(year.Int64), erasByTimeline[deckTimelineIds[deckId]]); ok {
				return era.Id.String()
			}
			return ""
		},
		"relativeYearForCard": func(deckId uuid.UUID, year sql.NullInt64) string {
			if !year.Valid {
				return ""
			}
			eras := erasByTimeline[deckTimelineIds[deckId]]
			if era, ok := database.FindEraForYear(int(year.Int64), eras); ok {
				return strconv.Itoa(database.RelativeYearInEra(int(year.Int64), era))
			}
			return strconv.FormatInt(year.Int64, 10)
		},
	}

	tmpl, err := parseChrome("html/pages/body/flagged-cards.html", funcMap)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Cards []database.FlaggedCard
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Cards:        cards,
	})
}

// Timelines is the admin management page for timelines, their eras, and
// their categories.
func Timelines(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Timelines"

	timelines, err := database.GetTimelinesWithEras()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get timelines"))
		return
	}

	tmpl, err := parseChrome("html/pages/body/timelines.html", nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Timelines         []database.TimelineWithEras
		DefaultTimelineId uuid.UUID
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:      basePageData,
		Timelines:         timelines,
		DefaultTimelineId: database.DefaultTimelineId,
	})
}

// Deck displays a single deck's cards. The shared header/export/edit-deck
// dialog/danger-zone/pagination chrome comes from the framework's
// deck-detail-chrome.html; this game's own card table, search field, and
// create/edit-card dialogs (Year + Category, plus the Import Cards dialog —
// both genuinely game-specific) come from the two local fragment files
// composed in via gsApiPages.ParseGameFragment.
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

	// Only timelines with at least one era are offered here — SetDeckTimeline
	// rejects assigning a deck to an eras-less timeline server-side too, this
	// just keeps the dropdown from offering a choice that would fail. The
	// deck's current timeline is always included even if (unexpectedly) it
	// has none, so the select still shows where the deck actually is.
	timelinesWithEras, err := database.GetTimelinesWithEras()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get timelines"))
		return
	}

	deckTimelineId, err := database.GetDeckTimelineId(deckId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get deck timeline"))
		return
	}

	timelines := make([]database.Timeline, 0, len(timelinesWithEras))
	for _, t := range timelinesWithEras {
		if (len(t.Eras) > 0 && len(t.Categories) > 0) || t.Id == deckTimelineId {
			timelines = append(timelines, t.Timeline)
		}
	}

	eras, err := database.GetErasForTimeline(deckTimelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get eras"))
		return
	}

	categories, err := database.GetCategoriesForTimeline(deckTimelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get categories"))
		return
	}

	// Prefill helpers for the Edit Card dialog: given an existing card's
	// (possibly NULL) year, find which of the deck's timeline's eras it
	// falls into and preselect that era + its in-era year, falling back to
	// the "Other" (raw absolute) option when no era matches.
	funcMap := template.FuncMap{
		"eraIdForYear": func(year sql.NullInt64) string {
			if !year.Valid {
				return ""
			}
			if era, ok := database.FindEraForYear(int(year.Int64), eras); ok {
				return era.Id.String()
			}
			return ""
		},
		"relativeYearForYear": func(year sql.NullInt64) string {
			if !year.Valid {
				return ""
			}
			if era, ok := database.FindEraForYear(int(year.Int64), eras); ok {
				return strconv.Itoa(database.RelativeYearInEra(int(year.Int64), era))
			}
			return strconv.FormatInt(year.Int64, 10)
		},
	}

	tmpl, err := gsApiPages.ParseGameFragmentWithFuncs(
		static.StaticFiles,
		funcMap,
		"html/pages/body/deck-detail-chrome.html",
		"html/pages/body/deck-card-management.html",
		"html/pages/body/deck-search-controls.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Deck           gsDatabase.Deck
		Text           string
		Page           int
		LastPage       int
		RowCount       int
		Cards          []database.Card
		Categories     []database.Category
		Timelines      []database.Timeline
		DeckTimelineId uuid.UUID
		Eras           []database.Era
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:   basePageData,
		Deck:           deck,
		Text:           text,
		Page:           page,
		LastPage:       totalPageCount,
		RowCount:       totalRowCount,
		Cards:          cards,
		Categories:     categories,
		Timelines:      timelines,
		DeckTimelineId: deckTimelineId,
		Eras:           eras,
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

	deckIds := make([]uuid.UUID, 0, len(decks))
	for _, d := range decks {
		deckIds = append(deckIds, d.Id)
	}
	cardCounts, err := database.GetDeckCardCounts(deckIds)
	if err != nil {
		cardCounts = make(map[uuid.UUID]int)
	}

	// All categories across every timeline — each one's own TimelineId (see
	// database.Category) lets the create-lobby form restrict the exclusion
	// list to whichever timeline is chosen, the same way deck selection is
	// restricted.
	categories, err := database.GetAllCategories()
	if err != nil {
		categories = make([]database.Category, 0)
	}

	// Which timeline each readable deck belongs to, so the create-lobby
	// form can restrict deck selection to whichever timeline is chosen.
	deckTimelineIds, err := database.GetDeckTimelineIds(deckIds)
	if err != nil {
		deckTimelineIds = make(map[uuid.UUID]uuid.UUID)
	}

	timelines, err := database.GetTimelines()
	if err != nil {
		timelines = make([]database.Timeline, 0)
	}

	type deckWithCardCount struct {
		gsDatabase.Deck
		CardCount  int
		TimelineId uuid.UUID
	}
	decksWithCounts := make([]deckWithCardCount, 0, len(decks))
	for _, d := range decks {
		decksWithCounts = append(decksWithCounts, deckWithCardCount{
			Deck:       d,
			CardCount:  cardCounts[d.Id],
			TimelineId: deckTimelineIds[d.Id],
		})
	}

	tmpl, err := parseChrome("html/pages/body/timeline-trivia-lobbies.html", nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Decks               []deckWithCardCount
		Categories          []database.Category
		Timelines           []database.Timeline
		DefaultTimelineId   uuid.UUID
		MinCardsPerWinRatio int
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:        basePageData,
		Decks:               decksWithCounts,
		Categories:          categories,
		Timelines:           timelines,
		DefaultTimelineId:   database.DefaultTimelineId,
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
		gameId, createErr := database.CreateTimelineTriviaGame(lobbyId, 10, database.DefaultTimelineId) // cards to win default
		if createErr != nil {
			log.Printf("[ERROR TimelineTriviaLobby] Failed to auto-create game for lobby %s: %v", lobbyId, createErr)
			http.Redirect(w, r, "/timeline-trivia/lobbies", http.StatusSeeOther)
			return
		}
		// Initialize draw pile with the TimelineTrivia deck (cards use authored years)
		timelineTriviaDeckId, _ := uuid.Parse("88026803-d22a-11f0-b4d2-60cf84649547")
		if initErr := database.InitializeTimelineTriviaDrawPile(gameId, []uuid.UUID{timelineTriviaDeckId}, nil); initErr != nil {
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

	// Which decks the draw pile was built from, for the header tooltip
	decks, err := database.GetTimelineTriviaGameDecks(game.Id)
	if err != nil {
		log.Printf("[ERROR TimelineTriviaLobby] Failed to get decks for game %s: %v", game.Id, err)
	}

	// Per-turn countdown; 0 = off (framework lobby setting)
	turnTimerSeconds, err := gsDatabase.GetLobbyTurnTimerSeconds(lobbyId)
	if err != nil {
		log.Printf("[ERROR TimelineTriviaLobby] Failed to get turn timer for lobby %s: %v", lobbyId, err)
	}

	eras, err := database.GetErasForTimeline(game.TimelineId)
	if err != nil {
		log.Printf("[ERROR TimelineTriviaLobby] Failed to get eras for timeline %s: %v", game.TimelineId, err)
	}
	funcMap := template.FuncMap{
		"formatYear": func(year int) string { return database.FormatYearInEras(year, eras) },
	}

	tmpl, err := parseChrome("html/pages/body/timeline-trivia.html", funcMap)
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
		Decks             []database.TimelineTriviaDeckInfo
		TurnTimerSeconds  int
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
		Decks:             decks,
		TurnTimerSeconds:  turnTimerSeconds,
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

	tmpl, err := parseChrome("html/pages/body/lobby-access.html", nil)
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
