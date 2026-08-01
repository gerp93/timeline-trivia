package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsApiUser "github.com/gerp93/gameshell-framework/api/user"
	gsAuth "github.com/gerp93/gameshell-framework/auth"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	gsStatic "github.com/gerp93/gameshell-framework/static"
)

// Lose Celebration mirrors Win Celebration's storage and handlers exactly
// (see TestWinCelebrationUpload for the size-limit/connection-reset
// regression coverage, which exercises the same shared code this handler
// reuses). This just confirms the lose-gif/lose-message wiring itself works
// end to end: upload, PNG accepted alongside GIF, and the message cap.
func TestLoseCelebrationUpload(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("TIMELINE_TRIVIA_SQL_DATABASE"), "tt_e2e") {
		t.Skip("set TIMELINE_TRIVIA_SQL_DATABASE=tt_e2e")
	}
	gsDatabase.SetEnvVarPrefix("TIMELINE_TRIVIA")
	gsAuth.SetCookiePrefix("CARD-TIMELINE")
	gsApiUser.SetMaxWinGifBytes(1000 * 1024) // matches main.go's startup config
	if _, err := gsDatabase.CreateDatabaseConnection(); err != nil {
		t.Fatalf("db: %v", err)
	}
	for _, f := range gsStatic.SQLFiles {
		if err := gsDatabase.RunFile(f); err != nil {
			t.Fatalf("schema %s: %v", f, err)
		}
	}
	if err := gsDatabase.CreateUser("lose_celebration_user", "unused", true); err != nil {
		t.Logf("create user (may already exist): %v", err)
	}
	userId, err := gsDatabase.GetUserIdByName("lose_celebration_user")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	gif := append([]byte("GIF89a"), bytes.Repeat([]byte{0x42}, 40*1024)...)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("loseGif", "oof.gif")
	_, _ = fw.Write(gif)
	_ = mw.Close()

	r := httptest.NewRequest("PUT", "/api/user/"+userId.String()+"/lose-gif", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.SetPathValue("userId", userId.String())
	cookieRec := httptest.NewRecorder()
	gsAuth.SetUserId(cookieRec, userId)
	for _, c := range cookieRec.Result().Cookies() {
		r.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.SetLoseGif)).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Errorf("upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Refresh") != "" {
		t.Errorf("HX-Refresh should not be set, got %q", rec.Header().Get("HX-Refresh"))
	}

	celebration, err := gsDatabase.GetUserLoseCelebration(userId)
	if err != nil || !celebration.HasGif {
		t.Errorf("gif was not stored: HasGif=%v err=%v", celebration.HasGif, err)
	}

	// PNG must also be accepted.
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x11}, 20*1024)...)
	var pngBody bytes.Buffer
	pmw := multipart.NewWriter(&pngBody)
	pfw, _ := pmw.CreateFormFile("loseGif", "oof.png")
	_, _ = pfw.Write(png)
	_ = pmw.Close()

	pr := httptest.NewRequest("PUT", "/api/user/"+userId.String()+"/lose-gif", &pngBody)
	pr.Header.Set("Content-Type", pmw.FormDataContentType())
	pr.SetPathValue("userId", userId.String())
	for _, c := range cookieRec.Result().Cookies() {
		pr.AddCookie(c)
	}
	prec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.SetLoseGif)).ServeHTTP(prec, pr)
	if prec.Code != http.StatusOK {
		t.Errorf("PNG upload failed: %d %s", prec.Code, prec.Body.String())
	}
	_, pmime, _ := gsDatabase.GetUserLoseGif(userId)
	if pmime != "image/png" {
		t.Errorf("expected image/png stored, got %q", pmime)
	}

	// Clearing removes the gif but leaves the message alone.
	if err := gsDatabase.SetUserLoseMessage(userId, "OOF"); err != nil {
		t.Fatalf("set lose message: %v", err)
	}
	dr := httptest.NewRequest("DELETE", "/api/user/"+userId.String()+"/lose-gif", nil)
	dr.SetPathValue("userId", userId.String())
	for _, c := range cookieRec.Result().Cookies() {
		dr.AddCookie(c)
	}
	drec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.ClearLoseGif)).ServeHTTP(drec, dr)
	if drec.Code != http.StatusOK {
		t.Errorf("clear gif failed: %d %s", drec.Code, drec.Body.String())
	}
	cleared, err := gsDatabase.GetUserLoseCelebration(userId)
	if err != nil || cleared.HasGif {
		t.Errorf("gif should be cleared: HasGif=%v err=%v", cleared.HasGif, err)
	}
	if cleared.Message.String != "OOF" {
		t.Errorf("clearing the gif should not touch the message, got %q", cleared.Message.String)
	}

	// A lose message over 140 runes must be rejected.
	longMsg := strings.Repeat("x", 141)
	mr := httptest.NewRequest("PUT", "/api/user/"+userId.String()+"/lose-message",
		strings.NewReader("loseMessage="+longMsg))
	mr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mr.SetPathValue("userId", userId.String())
	for _, c := range cookieRec.Result().Cookies() {
		mr.AddCookie(c)
	}
	mrec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.SetLoseMessage)).ServeHTTP(mrec, mr)
	if mrec.Code != http.StatusBadRequest {
		t.Errorf("expected 141-char message rejected, got %d", mrec.Code)
	}
}
