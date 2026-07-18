package main

import (
	"log"
	"net/http"
	"os"
	"time"

	gameshell "github.com/gerp93/gameshell-framework"
	gsApi "github.com/gerp93/gameshell-framework/api"
	gsAuth "github.com/gerp93/gameshell-framework/auth"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsStatic "github.com/gerp93/gameshell-framework/static"
	gsWebsocket "github.com/gerp93/gameshell-framework/websocket"

	apiAccess "github.com/gerp93/card-timeline/api/access"
	apiChronology "github.com/gerp93/card-timeline/api/chronology"
	apiPages "github.com/gerp93/card-timeline/api/pages"
	apiUser "github.com/gerp93/card-timeline/api/user"
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
	gsApi.SetBrandName("Card Timeline")
	gsAuth.SetCookiePrefix("CARD-TIMELINE")
	gsApi.SetPagePolicy(gsApi.PagePolicy{
		LoginPaths: []string{"/account", "/users"},
		LoginPathPrefixes: []string{
			"/deck/",
			"/chronology/",
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

	// static files
	http.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.StaticFiles))))

	// pages
	http.Handle("GET /", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Home)))
	http.Handle("GET /about", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.About)))
	http.Handle("GET /login", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Login)))
	http.Handle("GET /account", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Account)))
	http.Handle("GET /users", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Users)))
	http.Handle("GET /decks", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Decks)))
	http.Handle("GET /deck/{deckId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Deck)))
	http.Handle("GET /deck/{deckId}/access", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.DeckAccess)))

	// chronology pages
	http.Handle("GET /chronology/lobbies", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.ChronologyLobbies)))
	http.Handle("GET /chronology/{lobbyId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.ChronologyLobby)))
	http.Handle("GET /chronology/{lobbyId}/access", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.ChronologyLobbyAccess)))

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

	// chronology
	http.Handle("POST /api/chronology/create", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.Create)))
	http.Handle("POST /api/chronology/{lobbyId}/start", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.StartGame)))
	http.Handle("POST /api/chronology/{lobbyId}/reset", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.ResetGame)))
	http.Handle("POST /api/chronology/{lobbyId}/place-card", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.PlaceCard)))
	http.Handle("GET /api/chronology/{lobbyId}/state", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetGameState)))
	http.Handle("GET /api/chronology/{lobbyId}/timeline", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetTimeline)))
	http.Handle("GET /api/chronology/{lobbyId}/current-card", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetCurrentCard)))
	http.Handle("GET /api/chronology/{lobbyId}/players", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetPlayers)))
	http.Handle("GET /api/chronology/{lobbyId}/draw-pile-count", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetDrawPileCount)))
	http.Handle("POST /api/chronology/search", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiChronology.Search)))

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
