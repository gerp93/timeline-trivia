package main

import (
	"log"
	"net/http"
	"os"
	"time"

	gameshell "github.com/gerp93/gameshell-framework"
	gsApi "github.com/gerp93/gameshell-framework/api"
	gsApiDeck "github.com/gerp93/gameshell-framework/api/deck"
	gsAuth "github.com/gerp93/gameshell-framework/auth"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsStatic "github.com/gerp93/gameshell-framework/static"
	gsWebsocket "github.com/gerp93/gameshell-framework/websocket"

	apiAccess "github.com/gerp93/card-timeline/api/access"
	apiCard "github.com/gerp93/card-timeline/api/card"
	apiPages "github.com/gerp93/card-timeline/api/pages"
	apiTimelineTrivia "github.com/gerp93/card-timeline/api/timelinetrivia"
	apiUser "github.com/gerp93/card-timeline/api/user"
	"github.com/gerp93/card-timeline/database"
	"github.com/gerp93/card-timeline/game"
	"github.com/gerp93/card-timeline/static"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
		}
	}()

	gameshell.Register(game.CardTimeline{})
	gsApi.SetBrandName("Timeline Trivia")
	gsAuth.SetCookiePrefix("CARD-TIMELINE")
	gsApi.SetPagePolicy(gsApi.PagePolicy{
		LoginPaths: []string{"/account", "/users"},
		LoginPathPrefixes: []string{
			"/deck/",
			"/timeline-trivia/",
		},
		AdminPaths: []string{"/users"},
	})
	gsDatabase.SetEnvPrefix("CARD_TIMELINE")

	db, err := gsDatabase.CreateDatabaseConnection()
	dbConnectAttemptCount := 0
	for err != nil && dbConnectAttemptCount < 6 {
		time.Sleep(10 * time.Second)
		dbConnectAttemptCount += 1
		db, err = gsDatabase.CreateDatabaseConnection()
	}
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer db.Close()

	// framework schema first, game schema depends on it
	for _, sqlFile := range gsStatic.SQLFiles {
		err = gsDatabase.RunFile(sqlFile)
		if err != nil {
			log.Fatalln(err)
			return
		}
	}

	for _, sqlFile := range static.SQLFiles {
		bytes, err := static.StaticFiles.ReadFile(sqlFile)
		if err != nil {
			log.Fatalln(err)
			return
		}
		err = gsDatabase.Execute(string(bytes))
		if err != nil {
			log.Fatalln(err)
			return
		}
	}

	// Seed a default deck from the embedded starter data, but only if the
	// database has no decks yet.
	defaultDeckJSON, err := static.StaticFiles.ReadFile("data/default-deck.json")
	if err != nil {
		log.Fatalln(err)
		return
	}
	if err := database.SeedDefaultDeckIfEmpty(defaultDeckJSON); err != nil {
		log.Fatalln(err)
		return
	}

	// static files (game's own, plus shared framework assets under /gs/)
	http.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.StaticFiles))))
	http.Handle("GET /gs/", http.StripPrefix("/gs/", http.FileServer(http.FS(gsStatic.StaticFiles))))

	// pages
	http.Handle("GET /", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Home)))
	http.Handle("GET /about", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.About)))
	http.Handle("GET /login", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Login)))
	http.Handle("GET /account", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Account)))
	http.Handle("GET /users", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Users)))
	http.Handle("GET /decks", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Decks)))
	http.Handle("GET /deck/{deckId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Deck)))
	http.Handle("GET /deck/{deckId}/access", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.DeckAccess)))

	// timeline-trivia pages
	http.Handle("GET /timeline-trivia/lobbies", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.TimelineTriviaLobbies)))
	http.Handle("GET /timeline-trivia/{lobbyId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.TimelineTriviaLobby)))
	http.Handle("GET /timeline-trivia/{lobbyId}/access", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.TimelineTriviaLobbyAccess)))

	// user
	http.Handle("POST /api/user/create", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.Create)))
	http.Handle("POST /api/user/create/admin", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.CreateAdmin)))
	http.Handle("POST /api/user/login", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.Login)))
	http.Handle("POST /api/user/logout", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.Logout)))
	http.Handle("PUT /api/user/{userId}/name", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetName)))
	http.Handle("PUT /api/user/{userId}/password", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetPassword)))
	http.Handle("PUT /api/user/{userId}/password/reset", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.ResetPassword)))
	http.Handle("PUT /api/user/{userId}/color-theme", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetColorTheme)))
	http.Handle("PUT /api/user/{userId}/approve", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.Approve)))
	http.Handle("PUT /api/user/{userId}/is-admin", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetIsAdmin)))
	http.Handle("DELETE /api/user/{userId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiUser.Delete)))

	// deck (framework-owned deck management)
	http.Handle("POST /api/deck/create", gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiDeck.Create)))
	http.Handle("PUT /api/deck/{deckId}/name", gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiDeck.SetName)))
	http.Handle("PUT /api/deck/{deckId}/password", gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiDeck.SetPassword)))
	http.Handle("PUT /api/deck/{deckId}/is-public-read-only", gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiDeck.SetIsPublicReadOnly)))
	http.Handle("DELETE /api/deck/{deckId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiDeck.Delete)))

	// card (game-owned; text + year)
	http.Handle("GET /api/deck/{deckId}/card-export", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.GetCardExport)))
	http.Handle("POST /api/card/create", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.Create)))
	http.Handle("PUT /api/card/{cardId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.Update)))
	http.Handle("DELETE /api/card/{cardId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.Delete)))

	// timeline-trivia
	http.Handle("POST /api/timeline-trivia/create", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.Create)))
	http.Handle("POST /api/timeline-trivia/{lobbyId}/start", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.StartGame)))
	http.Handle("POST /api/timeline-trivia/{lobbyId}/reset", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.ResetGame)))
	http.Handle("POST /api/timeline-trivia/{lobbyId}/place-card", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.PlaceCard)))
	http.Handle("GET /api/timeline-trivia/{lobbyId}/state", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.GetGameState)))
	http.Handle("GET /api/timeline-trivia/{lobbyId}/timeline", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.GetTimeline)))
	http.Handle("GET /api/timeline-trivia/{lobbyId}/current-card", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.GetCurrentCard)))
	http.Handle("GET /api/timeline-trivia/{lobbyId}/players", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.GetPlayers)))
	http.Handle("GET /api/timeline-trivia/{lobbyId}/draw-pile-count", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.GetDrawPileCount)))
	http.Handle("POST /api/timeline-trivia/search", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.Search)))

	// access
	http.Handle("POST /api/access/lobby/{lobbyId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiAccess.Lobby)))
	http.Handle("POST /api/access/deck/{deckId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiAccess.Deck)))

	// websocket
	http.HandleFunc("GET /ws/lobby/{lobbyId}", gsWebsocket.ServeWs)

	if os.Getenv("CARD_TIMELINE_LOG_FILE") != "" {
		logFile, err := os.OpenFile(os.Getenv("CARD_TIMELINE_LOG_FILE"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalln(err)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	port := ":2016"
	if os.Getenv("CARD_TIMELINE_PORT") != "" {
		port = ":" + os.Getenv("CARD_TIMELINE_PORT")
	}

	log.Println("server is running...")
	if os.Getenv("CARD_TIMELINE_CERT_FILE") != "" && os.Getenv("CARD_TIMELINE_KEY_FILE") != "" {
		err = http.ListenAndServeTLS(port, os.Getenv("CARD_TIMELINE_CERT_FILE"), os.Getenv("CARD_TIMELINE_KEY_FILE"), nil)
	} else {
		err = http.ListenAndServe(port, nil)
	}
	if err != nil {
		log.Fatalln(err)
	}
}
