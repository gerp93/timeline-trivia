package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/grantfbarnes/card-judge/api"
	apiAccess "github.com/grantfbarnes/card-judge/api/access"
	apiChronology "github.com/grantfbarnes/card-judge/api/chronology"
	apiPages "github.com/grantfbarnes/card-judge/api/pages"
	apiUser "github.com/grantfbarnes/card-judge/api/user"
	"github.com/grantfbarnes/card-judge/database"
	"github.com/grantfbarnes/card-judge/static"
	"github.com/grantfbarnes/card-judge/websocket"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
		}
	}()

	db, err := database.CreateDatabaseConnection()
	dbConnectAttemptCount := 0
	for err != nil && dbConnectAttemptCount < 6 {
		time.Sleep(10 * time.Second)
		dbConnectAttemptCount += 1
		db, err = database.CreateDatabaseConnection()
	}
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer db.Close()


	for _, sqlFile := range static.SQLFiles {
		err = database.RunFile(sqlFile)
		if err != nil {
			log.Fatalln(err)
			return
		}
	}

	// static files
	http.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.StaticFiles))))

	// pages
	http.Handle("GET /", api.MiddlewareForPages(http.HandlerFunc(apiPages.Home)))
	http.Handle("GET /about", api.MiddlewareForPages(http.HandlerFunc(apiPages.About)))
	http.Handle("GET /login", api.MiddlewareForPages(http.HandlerFunc(apiPages.Login)))
	http.Handle("GET /account", api.MiddlewareForPages(http.HandlerFunc(apiPages.Account)))
	http.Handle("GET /users", api.MiddlewareForPages(http.HandlerFunc(apiPages.Users)))
	http.Handle("GET /decks", api.MiddlewareForPages(http.HandlerFunc(apiPages.Decks)))
	http.Handle("GET /deck/{deckId}", api.MiddlewareForPages(http.HandlerFunc(apiPages.Deck)))
	http.Handle("GET /deck/{deckId}/access", api.MiddlewareForPages(http.HandlerFunc(apiPages.DeckAccess)))

	// chronology pages
	http.Handle("GET /chronology/lobbies", api.MiddlewareForPages(http.HandlerFunc(apiPages.ChronologyLobbies)))
	http.Handle("GET /chronology/{lobbyId}", api.MiddlewareForPages(http.HandlerFunc(apiPages.ChronologyLobby)))
	http.Handle("GET /chronology/{lobbyId}/access", api.MiddlewareForPages(http.HandlerFunc(apiPages.ChronologyLobbyAccess)))

	// user
	http.Handle("POST /api/user/create", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.Create)))
	http.Handle("POST /api/user/create/admin", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.CreateAdmin)))
	http.Handle("POST /api/user/login", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.Login)))
	http.Handle("POST /api/user/logout", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.Logout)))
	http.Handle("PUT /api/user/{userId}/name", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetName)))
	http.Handle("PUT /api/user/{userId}/password", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetPassword)))
	http.Handle("PUT /api/user/{userId}/password/reset", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.ResetPassword)))
	http.Handle("PUT /api/user/{userId}/color-theme", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetColorTheme)))
	http.Handle("PUT /api/user/{userId}/approve", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.Approve)))
	http.Handle("PUT /api/user/{userId}/is-admin", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.SetIsAdmin)))
	http.Handle("DELETE /api/user/{userId}", api.MiddlewareForAPIs(http.HandlerFunc(apiUser.Delete)))

	// chronology
	http.Handle("POST /api/chronology/create", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.Create)))
	http.Handle("POST /api/chronology/{lobbyId}/start", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.StartGame)))
	http.Handle("POST /api/chronology/{lobbyId}/reset", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.ResetGame)))
	http.Handle("POST /api/chronology/{lobbyId}/place-card", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.PlaceCard)))
	http.Handle("GET /api/chronology/{lobbyId}/state", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetGameState)))
	http.Handle("GET /api/chronology/{lobbyId}/timeline", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetTimeline)))
	http.Handle("GET /api/chronology/{lobbyId}/current-card", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetCurrentCard)))
	http.Handle("GET /api/chronology/{lobbyId}/players", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetPlayers)))
	http.Handle("GET /api/chronology/{lobbyId}/draw-pile-count", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.GetDrawPileCount)))
	http.Handle("POST /api/chronology/search", api.MiddlewareForAPIs(http.HandlerFunc(apiChronology.Search)))

	// access
	http.Handle("POST /api/access/lobby/{lobbyId}", api.MiddlewareForAPIs(http.HandlerFunc(apiAccess.Lobby)))
	http.Handle("POST /api/access/deck/{deckId}", api.MiddlewareForAPIs(http.HandlerFunc(apiAccess.Deck)))

	// websocket
	http.HandleFunc("GET /ws/lobby/{lobbyId}", websocket.ServeWs)

	if os.Getenv("CARD_JUDGE_LOG_FILE") != "" {
		logFile, err := os.OpenFile(os.Getenv("CARD_JUDGE_LOG_FILE"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalln(err)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	port := ":2016"
	if os.Getenv("CARD_JUDGE_PORT") != "" {
		port = ":" + os.Getenv("CARD_JUDGE_PORT")
	}

	log.Println("server is running...")
	if os.Getenv("CARD_JUDGE_CERT_FILE") != "" && os.Getenv("CARD_JUDGE_KEY_FILE") != "" {
		err = http.ListenAndServeTLS(port, os.Getenv("CARD_JUDGE_CERT_FILE"), os.Getenv("CARD_JUDGE_KEY_FILE"), nil)
	} else {
		err = http.ListenAndServe(port, nil)
	}
	if err != nil {
		log.Fatalln(err)
	}
}
