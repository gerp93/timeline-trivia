package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsApiPages "github.com/gerp93/gameshell-framework/api/pages"
	gsAuth "github.com/gerp93/gameshell-framework/auth"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"

	apiPages "github.com/gerp93/timeline-trivia/api/pages"
	"github.com/gerp93/timeline-trivia/database"
)

// Regression test for the shared-page-template migration. These handlers
// all discard ExecuteTemplate's error (`_ = tmpl.ExecuteTemplate(...)`), so
// a template referencing a field the Go data struct doesn't have fails
// *silently* — a 200 with a truncated body, not a 500. Asserting on real
// rendered content (not just status code) is the only way to catch that.
func TestSharedPageTemplatesRender(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("TIMELINE_TRIVIA_SQL_DATABASE"), "tt_e2e") {
		t.Skip("set TIMELINE_TRIVIA_SQL_DATABASE=tt_e2e")
	}
	gsDatabase.SetEnvVarPrefix("TIMELINE_TRIVIA")
	gsAuth.SetCookiePrefix("CARD-TIMELINE")
	if _, err := gsDatabase.CreateDatabaseConnection(); err != nil {
		t.Fatalf("db: %v", err)
	}
	setupSchema(t)
	gsApiPages.SetAccountPageFeatures(gsApiPages.AccountPageFeatures{WinCelebration: true})

	if err := gsDatabase.CreateUser("render_admin", "unused", true); err != nil {
		t.Logf("create user (may already exist): %v", err)
	}
	userId, err := gsDatabase.GetUserIdByName("render_admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if err := gsDatabase.SetUserIsAdmin(userId, true); err != nil {
		t.Fatalf("set admin: %v", err)
	}

	deckId, err := gsDatabase.CreateDeck("render deck", "", true)
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	categoryId, err := database.CreateCategory("Render Category")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	year := sql.NullInt64{Int64: 1969, Valid: true}
	cardId, err := database.CreateCard(deckId, "render event", year, uuid.NullUUID{UUID: categoryId, Valid: true})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	// Give the stats page something in every section to render: a guess (so
	// decades/categories are non-empty) and a timeout at a known setting.
	if err := database.LogGuess(userId, cardId, 1969, true); err != nil {
		t.Fatalf("log guess: %v", err)
	}
	if err := database.LogTimeout(userId, cardId, 45); err != nil {
		t.Fatalf("log timeout: %v", err)
	}
	if err := gsDatabase.AddUserDeckAccess(userId, deckId); err != nil {
		t.Fatalf("grant deck access: %v", err)
	}

	cookieRec := httptest.NewRecorder()
	gsAuth.SetUserId(cookieRec, userId)
	cookies := cookieRec.Result().Cookies()

	get := func(target string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		return r
	}

	cases := []struct {
		name      string
		handler   http.HandlerFunc
		path      string
		anonymous bool
		setPath   func(r *http.Request)
		want      []string
	}{
		{"Login", gsApiPages.Login, "/login", true, nil, []string{"User Login"}},
		{"Users", gsApiPages.Users, "/users", false, nil, []string{"render_admin"}},
		{"Decks", gsApiPages.Decks, "/decks", false, nil, []string{"render deck"}},
		{"Account", gsApiPages.Account, "/account", false, nil, []string{"Win Celebration", "render_admin"}},
		{
			"Deck", apiPages.Deck, "/deck/{deckId}", false,
			func(r *http.Request) { r.SetPathValue("deckId", deckId.String()) },
			[]string{"render deck", "render event", "1969", "Render Category", "Import Cards"},
		},
		{
			"StatsUser", apiPages.StatsUser, "/stats/user/{userId}", false,
			func(r *http.Request) { r.SetPathValue("userId", userId.String()) },
			[]string{
				"render_admin", "Decades Guessed Most Often", "1960s",
				"Ran Out of Time", "45 seconds", "Render Category",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.anonymous {
				req = httptest.NewRequest(http.MethodGet, tc.path, nil)
			} else {
				req = get(tc.path)
			}
			if tc.setPath != nil {
				tc.setPath(req)
			}
			rec := httptest.NewRecorder()
			gsApi.MiddlewareForPages(tc.handler).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Errorf("response missing %q; template may have failed silently. Full body:\n%s", want, body)
				}
			}
		})
	}
}
