package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsAuth "github.com/gerp93/gameshell-framework/auth"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsStatic "github.com/gerp93/gameshell-framework/static"
	gsWebsocket "github.com/gerp93/gameshell-framework/websocket"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	apiCard "github.com/gerp93/timeline-trivia/api/card"
	apiTimelineTrivia "github.com/gerp93/timeline-trivia/api/timelinetrivia"
	"github.com/gerp93/timeline-trivia/database"
	"github.com/gerp93/timeline-trivia/static"
)

// End-to-end exercise of the playtest-feedback changes against a real
// database, driving the real HTTP handlers and real websocket clients.
// Sessions are minted with auth.SetUserId rather than by logging in; the
// signing secret is per-process, so an in-process cookie is valid here.

type player struct {
	name     string
	userId   uuid.UUID
	playerId uuid.UUID
	conn     *websocket.Conn
	received chan string
}

func setupSchema(t *testing.T) {
	t.Helper()
	for _, f := range gsStatic.SQLFiles {
		if err := gsDatabase.RunFile(f); err != nil {
			t.Fatalf("framework schema %s: %v", f, err)
		}
	}
	// This game uses decks, so its schema needs the framework's deck tables
	// too — mirrors main.go's gsBootstrap.ApplyFeatureSchema(features) call.
	// Not calling ApplyFeatureSchema itself: it log.Fatalln's on error, which
	// would abort the whole test binary instead of just failing this test.
	for _, f := range gsStatic.DeckSQLFiles {
		if err := gsDatabase.RunFile(f); err != nil {
			t.Fatalf("framework deck schema %s: %v", f, err)
		}
	}
	for _, f := range static.SQLFiles {
		if err := runGameFile(f); err != nil {
			t.Fatalf("game schema %s: %v", f, err)
		}
	}
}

func runGameFile(path string) error {
	b, err := static.StaticFiles.ReadFile(path)
	if err != nil {
		return err
	}
	return gsDatabase.Execute(string(b))
}

func authedRequest(t *testing.T, method, target string, form url.Values, userId uuid.UUID) *http.Request {
	t.Helper()
	var r *http.Request
	if form != nil {
		r = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	gsAuth.SetUserId(rec, userId)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	// Handlers read {lobbyId}/{cardId} via PathValue, which only a ServeMux
	// populates — set it from the path directly since these bypass routing.
	if parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/"); len(parts) >= 3 {
		if parts[0] == "api" && parts[1] == "timeline-trivia" {
			r.SetPathValue("lobbyId", parts[2])
		}
	}
	return r
}

func serve(h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(h).ServeHTTP(rec, r)
	return rec
}

// drain collects everything a client received within a short settle window.
func drain(p *player) []string {
	var out []string
	deadline := time.After(700 * time.Millisecond)
	for {
		select {
		case m := <-p.received:
			out = append(out, m)
		case <-deadline:
			return out
		}
	}
}

func resultPayloads(msgs []string) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		if !strings.HasPrefix(m, "result:") {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(m[len("result:"):]), &p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func chatLines(msgs []string) []string {
	var out []string
	for _, m := range msgs {
		if strings.HasPrefix(m, "result:") || strings.HasPrefix(m, "turnTimer:") ||
			strings.HasPrefix(m, "lobbyMessage:") || m == "refresh" || m == "reload" {
			continue
		}
		out = append(out, m)
	}
	return out
}

func TestPlaytestFeedbackEndToEnd(t *testing.T) {
	// This test seeds and mutates freely, so it refuses to touch anything but
	// a purpose-made throwaway database.
	dbName := os.Getenv("TIMELINE_TRIVIA_SQL_DATABASE")
	if !strings.HasPrefix(dbName, "tt_e2e") {
		t.Skipf("refusing to run against %q; set TIMELINE_TRIVIA_SQL_DATABASE=tt_e2e", dbName)
	}
	gsDatabase.SetEnvVarPrefix("TIMELINE_TRIVIA")
	gsAuth.SetCookiePrefix("CARD-TIMELINE")
	if _, err := gsDatabase.CreateDatabaseConnection(); err != nil {
		t.Fatalf("db connect: %v", err)
	}
	setupSchema(t)

	// ---- seed users, deck, cards -------------------------------------------
	names := []string{"e2e_alice", "e2e_bob", "e2e_carol"}
	players := make([]*player, 0, len(names))
	for _, n := range names {
		if err := gsDatabase.CreateUser(n, "unused-not-a-login", true); err != nil {
			t.Fatalf("create user %s: %v", n, err)
		}
		id, err := gsDatabase.GetUserIdByName(n)
		if err != nil {
			t.Fatalf("get user %s: %v", n, err)
		}
		players = append(players, &player{name: n, userId: id, received: make(chan string, 256)})
	}
	// Alice gets a win celebration so the popup payload can be checked.
	if err := gsDatabase.SetUserWinMessage(players[0].userId, "GET REKT"); err != nil {
		t.Fatalf("set win message: %v", err)
	}
	if err := gsDatabase.SetUserWinGif(players[0].userId, []byte("GIF89a-fake-bytes"), "image/gif"); err != nil {
		t.Fatalf("set win gif: %v", err)
	}

	deckId, err := gsDatabase.CreateDeck("e2e deck", "", true)
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	categoryId, err := database.CreateCategory("E2E Category")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	for i := 0; i < 60; i++ {
		year := sql.NullInt64{Int64: int64(1000 + i*10), Valid: true}
		_, err := database.CreateCard(deckId, fmt.Sprintf("e2e event %d", i), year,
			uuid.NullUUID{UUID: categoryId, Valid: true})
		if err != nil {
			t.Fatalf("create card %d: %v", i, err)
		}
	}

	// ---- lobby, players, game ----------------------------------------------
	lobbyId, err := database.CreateTimelineTriviaLobby("e2e lobby", "", "")
	if err != nil {
		t.Fatalf("create lobby: %v", err)
	}
	for _, p := range players {
		if err := gsDatabase.AddUserLobbyAccess(p.userId, lobbyId); err != nil {
			t.Fatalf("grant access: %v", err)
		}
		pid, err := gsDatabase.AddUserToLobby(lobbyId, p.userId)
		if err != nil {
			t.Fatalf("join lobby: %v", err)
		}
		p.playerId = pid
	}
	gameId, err := database.CreateTimelineTriviaGame(lobbyId, 10)
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	if err := database.InitializeTimelineTriviaDrawPile(gameId, []uuid.UUID{deckId}, nil); err != nil {
		t.Fatalf("init draw pile: %v", err)
	}
	if err := gsDatabase.SetLobbyTurnTimerSeconds(lobbyId, 30); err != nil {
		t.Fatalf("set turn timer: %v", err)
	}

	// ---- real websocket clients, one per player ----------------------------
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/lobby/{lobbyId}", gsWebsocket.ServeWs)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, p := range players {
		rec := httptest.NewRecorder()
		gsAuth.SetUserId(rec, p.userId)
		hdr := http.Header{}
		for _, c := range rec.Result().Cookies() {
			hdr.Add("Cookie", c.Name+"="+c.Value)
		}
		conn, _, err := websocket.DefaultDialer.Dial(
			"ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/lobby/"+lobbyId.String(), hdr)
		if err != nil {
			t.Fatalf("ws dial %s: %v", p.name, err)
		}
		p.conn = conn
		go func(p *player) {
			for {
				_, msg, err := p.conn.ReadMessage()
				if err != nil {
					return
				}
				select {
				case p.received <- string(msg):
				default:
				}
			}
		}(p)
	}
	defer func() {
		for _, p := range players {
			_ = p.conn.Close()
		}
	}()
	time.Sleep(400 * time.Millisecond)
	for _, p := range players {
		drain(p) // discard join noise
	}

	byPlayerId := map[uuid.UUID]*player{}
	for _, p := range players {
		byPlayerId[p.playerId] = p
	}
	currentPlayer := func() *player {
		g, err := database.GetTimelineTriviaGameById(gameId)
		if err != nil || !g.CurrentPlayerId.Valid {
			t.Fatalf("no current player: %v", err)
		}
		return byPlayerId[g.CurrentPlayerId.UUID]
	}
	turnOrder := func() []string {
		ps, err := database.GetTimelineTriviaPlayers(gameId)
		if err != nil {
			t.Fatalf("get players: %v", err)
		}
		var out []string
		for _, p := range ps {
			if p.IsActive {
				out = append(out, p.UserName)
			}
		}
		return out
	}

	// ================= 1. start game, shuffled + stable order ===============
	rec := serve(apiTimelineTrivia.StartGame,
		authedRequest(t, "POST", "/api/timeline-trivia/"+lobbyId.String()+"/start", url.Values{}, players[0].userId))
	if rec.Code != http.StatusOK {
		t.Fatalf("start game: %d %s", rec.Code, rec.Body.String())
	}
	order1 := turnOrder()
	if len(order1) != 3 {
		t.Fatalf("expected 3 players in order, got %v", order1)
	}
	t.Logf("turn order after start: %v", order1)
	// The very first shuffle's "previous" baseline is JOIN_ORDER (no
	// TIMELINE_TRIVIA_PLAYER_ORDER rows exist yet), and
	// ShuffleTimelineTriviaPlayerOrder guarantees the new order differs from
	// that baseline — so the first game is provably shuffled, not merely
	// "probably" (it can never just fall back to join order by chance).
	joinOrder := []string{players[0].name, players[1].name, players[2].name}
	if strings.Join(order1, ",") == strings.Join(joinOrder, ",") {
		t.Errorf("first game's order equals plain join order %v — shuffle guarantee not holding", joinOrder)
	}

	startMsgs := drain(players[1])
	foundOrderChat := false
	for _, l := range chatLines(startMsgs) {
		if strings.Contains(l, "Game started") && strings.Contains(l, "turn order") {
			foundOrderChat = true
			t.Logf("chat: %s", l)
		}
	}
	if !foundOrderChat {
		t.Errorf("expected a 'Game started ... turn order' chat line, got %v", chatLines(startMsgs))
	}
	for _, p := range players[0:1] {
		drain(p)
	}
	drain(players[2])

	// Order must not change as turns advance.
	orderBefore := strings.Join(turnOrder(), ",")

	// ================= 2. wrong guess: everyone sees it =====================
	guesser := currentPlayer()
	card, err := database.GetTimelineTriviaCurrentCard(gameId)
	if err != nil {
		t.Fatalf("current card: %v", err)
	}
	timeline, err := database.GetPlayerTimeline(gameId, guesser.playerId)
	if err != nil || len(timeline) != 1 {
		t.Fatalf("expected 1 dealt card, got %d (%v)", len(timeline), err)
	}
	// position 0 is correct iff the dealt card is not older than the current
	// card; pick the opposite so this guess definitely misses.
	wrongPos := 0
	if timeline[0].CardYear >= card.CardYear {
		wrongPos = 1
	}
	rec = serve(apiTimelineTrivia.PlaceCard, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/place-card",
		url.Values{"position": {fmt.Sprint(wrongPos)}}, guesser.userId))
	if rec.Code != http.StatusOK {
		t.Fatalf("place card (wrong): %d %s", rec.Code, rec.Body.String())
	}

	// THE REPORTED BUG: every client, not just the guesser, must get the
	// status line for this guess.
	for _, p := range players {
		msgs := drain(p)
		payloads := resultPayloads(msgs)
		if len(payloads) == 0 {
			t.Errorf("%s received no result payload for another player's guess: %v", p.name, msgs)
			continue
		}
		bm, _ := payloads[0]["bottomMessage"].(string)
		if bm == "" {
			t.Errorf("%s got a result payload with no bottomMessage: %v", p.name, payloads[0])
		}
		if payloads[0]["type"] != "incorrect" {
			t.Errorf("%s expected type=incorrect, got %v", p.name, payloads[0]["type"])
		}
		if next, _ := payloads[0]["nextPlayerName"].(string); next == "" {
			t.Errorf("%s: wrong-guess result had no nextPlayerName", p.name)
		}
		t.Logf("%s bottomMessage: %q", p.name, bm)

		for _, l := range chatLines(msgs) {
			if strings.Contains(l, "guessed wrong") {
				t.Logf("%s chat: %s", p.name, l)
				if strings.Contains(l, database.FormatYear(card.CardYear)) {
					t.Errorf("wrong-guess chat leaked the year: %s", l)
				}
				if !strings.Contains(l, card.CardText) {
					t.Errorf("wrong-guess chat missing card text: %s", l)
				}
			}
		}
	}

	if got := strings.Join(turnOrder(), ","); got != orderBefore {
		t.Errorf("turn order changed after a guess: %q -> %q", orderBefore, got)
	}
	if currentPlayer().playerId == guesser.playerId {
		t.Errorf("card did not pass to the next guesser after a miss")
	}

	// ================= 2a. skip & remove is refused mid-steal ===============
	// The current guesser right now is a stealer (someone already missed this
	// exact card), not who the round opened with — Skip & Remove must be
	// off-limits so a bad guess can't be laundered into "the card was bad".
	stealer := currentPlayer()
	stealGame, err := database.GetTimelineTriviaGameById(gameId)
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if !stealGame.RoundStarterPlayerId.Valid || stealGame.CurrentPlayerId.UUID == stealGame.RoundStarterPlayerId.UUID {
		t.Fatalf("test setup assumption broken: expected a steal in progress (current != round starter)")
	}
	rec = serve(apiTimelineTrivia.SkipAndRemoveCard, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/skip-card", url.Values{}, stealer.userId))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("skip during a steal should be rejected, got %d %s", rec.Code, rec.Body.String())
	}
	// The card must still be there — a rejected skip must not have flagged it.
	stillCurrent, _ := database.GetTimelineTriviaCurrentCard(gameId)
	if stillCurrent.CardId != card.CardId {
		t.Errorf("card changed despite a rejected skip: had %s, now %s", card.CardId, stillCurrent.CardId)
	}
	// The HTML fragment must not render the button for this viewer either —
	// the server-side rejection above is a backstop, not the only guard.
	cardHTML := httptest.NewRecorder()
	cardReq := authedRequest(t, "GET", "/api/timeline-trivia/"+lobbyId.String()+"/current-card", nil, stealer.userId)
	gsApi.MiddlewareForAPIs(http.HandlerFunc(apiTimelineTrivia.GetCurrentCard)).ServeHTTP(cardHTML, cardReq)
	if strings.Contains(cardHTML.Body.String(), "Skip &amp; Remove") {
		t.Errorf("current-card fragment offered Skip & Remove to a stealer:\n%s", cardHTML.Body.String())
	}

	// ================= 3. timeout pass, no penalty ==========================
	timedOut := currentPlayer()
	preTimeline, _ := database.GetPlayerTimeline(gameId, timedOut.playerId)
	rec = serve(apiTimelineTrivia.TimeoutPass, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/timeout", url.Values{}, timedOut.userId))
	if rec.Code != http.StatusOK {
		t.Fatalf("timeout: %d %s", rec.Code, rec.Body.String())
	}
	postTimeline, _ := database.GetPlayerTimeline(gameId, timedOut.playerId)
	if len(preTimeline) != len(postTimeline) {
		t.Errorf("timeout changed the player's timeline: %d -> %d", len(preTimeline), len(postTimeline))
	}
	// No ghost "guessed here" marker for a timeout.
	all, err := database.GetAllPlayerTimelines(gameId, currentPlayer().playerId, timedOut.playerId)
	if err != nil {
		t.Fatalf("all timelines: %v", err)
	}
	for _, row := range all {
		if row.PlayerId == timedOut.playerId && row.HasAttempt {
			t.Errorf("timed-out player got a 'guessed here' marker (HasAttempt=true)")
		}
	}
	for _, p := range players {
		found := false
		for _, l := range chatLines(drain(p)) {
			if strings.Contains(l, "ran out of time") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s did not see the timeout announcement", p.name)
		}
	}

	// ================= 4. everyone missed -> discard + reveal ===============
	// One player is left who has not had this card.
	last := currentPlayer()
	lastCard, _ := database.GetTimelineTriviaCurrentCard(gameId)
	lastTimeline, _ := database.GetPlayerTimeline(gameId, last.playerId)
	wrongPos = 0
	if lastTimeline[0].CardYear >= lastCard.CardYear {
		wrongPos = 1
	}
	rec = serve(apiTimelineTrivia.PlaceCard, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/place-card",
		url.Values{"position": {fmt.Sprint(wrongPos)}}, last.userId))
	if rec.Code != http.StatusOK {
		t.Fatalf("place card (exhaust): %d %s", rec.Code, rec.Body.String())
	}
	sawReveal := false
	for _, p := range players {
		msgs := drain(p)
		for _, pl := range resultPayloads(msgs) {
			if pl["type"] == "revealed" {
				sawReveal = true
				if bm, _ := pl["bottomMessage"].(string); !strings.Contains(bm, database.FormatYear(lastCard.CardYear)) {
					t.Errorf("reveal bottomMessage missing the year: %q", bm)
				}
				if next, _ := pl["nextPlayerName"].(string); next == "" {
					t.Errorf("%s: revealed result had no nextPlayerName", p.name)
				}
			}
		}
		for _, l := range chatLines(msgs) {
			if strings.Contains(l, "Nobody got") {
				t.Logf("%s chat: %s", p.name, l)
				if !strings.Contains(l, database.FormatYear(lastCard.CardYear)) {
					t.Errorf("reveal chat missing the year: %s", l)
				}
			}
		}
	}
	if !sawReveal {
		t.Errorf("expected a 'revealed' result after every player missed")
	}

	// ================= 5. correct guess + win celebration ===================
	// Drive Alice specifically so her celebration is on the payload.
	alice := players[0]
	for currentPlayer().playerId != alice.playerId {
		g := currentPlayer()
		gCard, _ := database.GetTimelineTriviaCurrentCard(gameId)
		gTl, _ := database.GetPlayerTimeline(gameId, g.playerId)
		wp := 0
		if gTl[0].CardYear >= gCard.CardYear {
			wp = 1
		}
		serve(apiTimelineTrivia.PlaceCard, authedRequest(t, "POST",
			"/api/timeline-trivia/"+lobbyId.String()+"/place-card",
			url.Values{"position": {fmt.Sprint(wp)}}, g.userId))
		for _, p := range players {
			drain(p)
		}
	}
	aCard, _ := database.GetTimelineTriviaCurrentCard(gameId)
	aTl, _ := database.GetPlayerTimeline(gameId, alice.playerId)
	rightPos := 0
	if aTl[0].CardYear < aCard.CardYear {
		rightPos = len(aTl)
	}
	rec = serve(apiTimelineTrivia.PlaceCard, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/place-card",
		url.Values{"position": {fmt.Sprint(rightPos)}}, alice.userId))
	if rec.Code != http.StatusOK {
		t.Fatalf("place card (correct): %d %s", rec.Code, rec.Body.String())
	}
	sawCelebration := false
	for _, p := range players {
		msgs := drain(p)
		for _, pl := range resultPayloads(msgs) {
			if pl["type"] != "correct" {
				continue
			}
			if pl["celebration"] == "GET REKT" && pl["hasGif"] == true && pl["userId"] == alice.userId.String() {
				sawCelebration = true
			}
			if bm, _ := pl["bottomMessage"].(string); bm == "" {
				t.Errorf("%s: correct result had no bottomMessage", p.name)
			}
			if next, _ := pl["nextPlayerName"].(string); next == "" {
				t.Errorf("%s: non-winning correct result had no nextPlayerName", p.name)
			}
		}
		for _, l := range chatLines(msgs) {
			if strings.Contains(l, "correctly") {
				t.Logf("%s chat: %s", p.name, l)
				if !strings.Contains(l, database.FormatYear(aCard.CardYear)) {
					t.Errorf("correct-guess chat missing the year: %s", l)
				}
			}
		}
	}
	if !sawCelebration {
		t.Errorf("every client should have received Alice's win celebration on the payload")
	}

	// ================= 6. last-placed highlight, exactly one ================
	all, _ = database.GetAllPlayerTimelines(gameId, currentPlayer().playerId, alice.playerId)
	highlighted := 0
	for _, row := range all {
		for _, c := range row.Timeline {
			if c.IsLastPlaced {
				highlighted++
			}
		}
	}
	if highlighted != 1 {
		t.Errorf("expected exactly 1 last-placed card highlighted, got %d", highlighted)
	}

	// ================= 7. skip & remove -> purgatory ========================
	skipper := currentPlayer()
	badCard, _ := database.GetTimelineTriviaCurrentCard(gameId)
	rec = serve(apiTimelineTrivia.SkipAndRemoveCard, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/skip-card", url.Values{}, skipper.userId))
	if rec.Code != http.StatusOK {
		t.Fatalf("skip card: %d %s", rec.Code, rec.Body.String())
	}
	flagged, err := database.GetFlaggedCards()
	if err != nil {
		t.Fatalf("get flagged: %v", err)
	}
	foundFlag := false
	for _, f := range flagged {
		if f.Id == badCard.CardId {
			foundFlag = true
			if f.DeckName == "" || !f.FlaggedByName.Valid {
				t.Errorf("flagged card missing deck/flagger detail: %+v", f)
			}
			t.Logf("flagged: %q year=%v deck=%s by=%s", f.Text, f.Year.Int64, f.DeckName, f.FlaggedByName.String)
		}
	}
	if !foundFlag {
		t.Errorf("skipped card did not land in CARD_FLAGGED")
	}
	newCard, _ := database.GetTimelineTriviaCurrentCard(gameId)
	if newCard.CardId == badCard.CardId {
		t.Errorf("skip did not draw a replacement card")
	}
	if newCard.DeckName == "" {
		t.Errorf("current card is missing its deck name")
	}

	// A flagged card must not be eligible for a fresh draw pile.
	count, err := database.CountCardsInDecksForRanges([]uuid.UUID{deckId}, nil, nil)
	if err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if count != 59 {
		t.Errorf("expected the flagged card excluded from the pool (59), got %d", count)
	}

	// ================= 8. admin review: accept puts it back =================
	admin := players[1]
	if err := gsDatabase.SetUserIsAdmin(admin.userId, true); err != nil {
		t.Fatalf("set admin: %v", err)
	}
	r := authedRequest(t, "POST", "/api/card/"+badCard.CardId.String()+"/unflag", url.Values{}, admin.userId)
	r.SetPathValue("cardId", badCard.CardId.String())
	rec = serve(apiCard.Unflag, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("unflag: %d %s", rec.Code, rec.Body.String())
	}
	count, _ = database.CountCardsInDecksForRanges([]uuid.UUID{deckId}, nil, nil)
	if count != 60 {
		t.Errorf("accepted card should be back in the pool (60), got %d", count)
	}

	// Non-admin must not be able to review.
	r = authedRequest(t, "POST", "/api/card/"+badCard.CardId.String()+"/unflag", url.Values{}, players[2].userId)
	r.SetPathValue("cardId", badCard.CardId.String())
	if rec = serve(apiCard.Unflag, r); rec.Code != http.StatusUnauthorized {
		t.Errorf("non-admin unflag should be 401, got %d", rec.Code)
	}

	// ================= 9. only the current guesser may skip =================
	notGuesser := players[0]
	if currentPlayer().playerId == notGuesser.playerId {
		notGuesser = players[1]
	}
	rec = serve(apiTimelineTrivia.SkipAndRemoveCard, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/skip-card", url.Values{}, notGuesser.userId))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-guesser skip should be rejected, got %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(apiTimelineTrivia.TimeoutPass, authedRequest(t, "POST",
		"/api/timeline-trivia/"+lobbyId.String()+"/timeout", url.Values{}, notGuesser.userId))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-guesser timeout should be rejected, got %d %s", rec.Code, rec.Body.String())
	}

	// ================= 10. deck info + turn timer ===========================
	decks, err := database.GetTimelineTriviaGameDecks(gameId)
	if err != nil || len(decks) != 1 {
		t.Fatalf("expected 1 deck in play, got %d (%v)", len(decks), err)
	}
	if decks[0].Name != "e2e deck" || decks[0].TotalCount != 60 || decks[0].RemainingCount >= 60 {
		t.Errorf("unexpected deck info: %+v", decks[0])
	}
	t.Logf("deck info: %s %d/%d remaining", decks[0].Name, decks[0].RemainingCount, decks[0].TotalCount)

	secs, err := gsDatabase.GetLobbyTurnTimerSeconds(lobbyId)
	if err != nil || secs != 30 {
		t.Errorf("turn timer should be 30, got %d (%v)", secs, err)
	}

	// ================= 11. every restart's order differs from the last ======
	// Force a finish so ResetGame is legal, then confirm each restart
	// reorders. This is now a deterministic guarantee (see
	// ShuffleTimelineTriviaPlayerOrder), not just a likelihood — so unlike a
	// probabilistic check, every single iteration must differ, not just one
	// eventually.
	if err := gsDatabase.Execute(
		"UPDATE TIMELINE_TRIVIA_GAME SET GAME_STATUS = 'finished' WHERE ID = ?", gameId); err != nil {
		t.Fatalf("force finish: %v", err)
	}
	prev := strings.Join(turnOrder(), ",")
	for i := 0; i < 5; i++ {
		rec = serve(apiTimelineTrivia.ResetGame, authedRequest(t, "POST",
			"/api/timeline-trivia/"+lobbyId.String()+"/reset", url.Values{}, players[0].userId))
		if rec.Code != http.StatusOK {
			t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
		}
		rec = serve(apiTimelineTrivia.StartGame, authedRequest(t, "POST",
			"/api/timeline-trivia/"+lobbyId.String()+"/start", url.Values{}, players[0].userId))
		if rec.Code != http.StatusOK {
			t.Fatalf("restart: %d %s", rec.Code, rec.Body.String())
		}
		now := strings.Join(turnOrder(), ",")
		t.Logf("restart %d order: %s", i+1, now)
		if now == prev {
			t.Errorf("restart %d produced the same order as the previous game: %s", i+1, now)
		}
		prev = now
		if err := gsDatabase.Execute(
			"UPDATE TIMELINE_TRIVIA_GAME SET GAME_STATUS = 'finished' WHERE ID = ?", gameId); err != nil {
			t.Fatalf("force finish: %v", err)
		}
		for _, p := range players {
			drain(p)
		}
	}
}
