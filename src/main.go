package main

import (
	"log"
	"net/http"
	"time"

	gameshell "github.com/gerp93/gameshell-framework"
	gsApi "github.com/gerp93/gameshell-framework/api"
	gsApiUser "github.com/gerp93/gameshell-framework/api/user"
	gsAuth "github.com/gerp93/gameshell-framework/auth"
	gsBootstrap "github.com/gerp93/gameshell-framework/bootstrap"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsStatic "github.com/gerp93/gameshell-framework/static"
	gsWebsocket "github.com/gerp93/gameshell-framework/websocket"

	apiAccess "github.com/gerp93/timeline-trivia/api/access"
	apiCard "github.com/gerp93/timeline-trivia/api/card"
	apiCategory "github.com/gerp93/timeline-trivia/api/category"
	apiPages "github.com/gerp93/timeline-trivia/api/pages"
	apiTimelineTrivia "github.com/gerp93/timeline-trivia/api/timelinetrivia"
	"github.com/gerp93/timeline-trivia/database"
	"github.com/gerp93/timeline-trivia/game"
	"github.com/gerp93/timeline-trivia/static"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
		}
	}()

	gameshell.Register(game.TimelineTrivia{})
	gsApi.SetBrandName("Timeline Trivia")
	gsAuth.SetCookiePrefix("CARD-TIMELINE")
	gsApi.SetPagePolicy(gsApi.PagePolicy{
		LoginPaths: []string{"/account", "/users", "/categories", "/flagged-cards", "/stats"},
		LoginPathPrefixes: []string{
			"/deck",
			"/timeline-trivia",
			"/stats",
		},
		AdminPaths: []string{"/users", "/categories", "/flagged-cards"},
	})
	gsDatabase.SetEnvVarPrefix("TIMELINE_TRIVIA")
	gsApiUser.SetMaxWinGifBytes(1000 * 1024)
	features := gsBootstrap.Features{
		Decks:           true,
		WinCelebration:  true,
		LoseCelebration: true,
		LobbyTurnTimer:  true,
	}
	gsBootstrap.MountFeatures(features)

	db := gsBootstrap.ConnectWithRetry(6, 10*time.Second)
	defer db.Close()

	// framework schema first, game schema depends on it
	gsBootstrap.ApplySchema(gsStatic.StaticFiles, gsStatic.SQLFiles)
	gsBootstrap.ApplyFeatureSchema(features)
	gsBootstrap.ApplySchema(static.StaticFiles, static.SQLFiles)

	// Seed a default deck from the embedded starter data, but only if the
	// database has no decks yet. Categories are seeded first (independently of
	// deck seeding, so an existing database still gets its base category list),
	// then any pre-category cards in the default deck get backfilled by text.
	defaultDeckJSON, err := static.StaticFiles.ReadFile("data/default-deck.json")
	if err != nil {
		log.Fatalln(err)
		return
	}
	if err := database.SeedCategoriesIfEmpty(defaultDeckJSON); err != nil {
		log.Fatalln(err)
		return
	}
	if err := database.SeedDefaultDeckIfEmpty(defaultDeckJSON); err != nil {
		log.Fatalln(err)
		return
	}
	if err := database.BackfillDefaultDeckCategories(defaultDeckJSON); err != nil {
		log.Fatalln(err)
		return
	}
	if err := database.SeedDefaultUserIfEmpty(); err != nil {
		log.Fatalln(err)
		return
	}

	// static files (game's own at /static/, shared framework assets at /gs/)
	gsBootstrap.MountStaticAssets(static.StaticFiles)

	// pages (game-owned; framework's core + Features-gated pages are wired by MountFeatures)
	// "/{$}" (not "/"): a bare "/" is a Go 1.22+ subtree wildcard matching
	// every unmatched path, silently serving Home for any bad URL instead of
	// a real 404. "{$}" restricts the match to the literal root only.
	http.Handle("GET /{$}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Home)))
	http.Handle("GET /about", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.About)))
	http.Handle("GET /categories", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Categories)))
	http.Handle("GET /flagged-cards", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.FlaggedCards)))
	http.Handle("GET /deck/{deckId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Deck)))

	// stats pages
	http.Handle("GET /stats", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.Stats)))
	http.Handle("GET /stats/leaderboard", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.StatsLeaderboard)))
	http.Handle("GET /stats/users", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.StatsUsers)))
	http.Handle("GET /stats/user/{userId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.StatsUser)))
	http.Handle("GET /stats/cards", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.StatsCards)))
	http.Handle("GET /stats/card/{cardId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.StatsCard)))

	// timeline-trivia pages
	http.Handle("GET /timeline-trivia/lobbies", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.TimelineTriviaLobbies)))
	http.Handle("GET /timeline-trivia/{lobbyId}", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.TimelineTriviaLobby)))
	http.Handle("GET /timeline-trivia/{lobbyId}/access", gsApi.MiddlewareForPages(http.HandlerFunc(apiPages.TimelineTriviaLobbyAccess)))

	// card (game-owned; text + year)
	http.Handle("GET /api/deck/{deckId}/card-export", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.GetCardExport)))
	http.Handle("POST /api/deck/{deckId}/card-import", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.ImportJSON)))
	http.Handle("POST /api/card/create", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.Create)))
	http.Handle("PUT /api/card/{cardId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.Update)))
	http.Handle("DELETE /api/card/{cardId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.Delete)))

	// flagged-card review (admin-only, checked in-handler). Accept is a POST
	// rather than DELETE because it removes the flag, not the card.
	http.Handle("POST /api/card/{cardId}/unflag", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.Unflag)))
	http.Handle("PUT /api/card/{cardId}/flagged", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCard.UpdateFlagged)))

	// category (game-owned; admin-managed predefined list, checked in-handler).
	// Delete-with-reassign is a POST (not DELETE) because it carries a form
	// body — Go's ParseForm only reads the body for POST/PUT/PATCH.
	http.Handle("POST /api/category/create", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCategory.Create)))
	http.Handle("POST /api/category/{categoryId}/delete", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiCategory.DeleteReassign)))

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
	http.Handle("GET /api/timeline-trivia/{lobbyId}/decks", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.GetDecks)))
	http.Handle("POST /api/timeline-trivia/{lobbyId}/skip-card", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.SkipAndRemoveCard)))
	http.Handle("POST /api/timeline-trivia/{lobbyId}/timeout", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.TimeoutPass)))
	http.Handle("PUT /api/timeline-trivia/{lobbyId}/message", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.SetLobbyMessage)))
	http.Handle("POST /api/timeline-trivia/search", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.Search)))
	http.Handle("POST /api/timeline-trivia/card-count", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.CardCount)))

	// access
	http.Handle("POST /api/access/lobby/{lobbyId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiAccess.Lobby)))
	http.Handle("POST /api/access/deck/{deckId}", gsApi.MiddlewareForAPIs(http.HandlerFunc(apiAccess.Deck)))

	// websocket
	http.HandleFunc("GET /ws/lobby/{lobbyId}", gsWebsocket.ServeWs)

	gsBootstrap.Serve("TIMELINE_TRIVIA")
}
