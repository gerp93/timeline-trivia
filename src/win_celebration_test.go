package main

import (
	"bytes"
	"io"
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

// Regression test for the reported "nothing happens on a valid GIF upload".
// The upload actually succeeded — SetWinGif used to set HX-Refresh: true,
// which reloads the page and collapses the <details> the form lives in
// before the new preview is visible, indistinguishable from a silent
// failure. Also covers the follow-up asks: PNG accepted alongside GIF, and
// the win message capped at 140 characters.
func TestWinCelebrationUpload(t *testing.T) {
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
	if err := gsDatabase.CreateUser("celebration_user", "unused", true); err != nil {
		t.Logf("create user (may already exist): %v", err)
	}
	userId, err := gsDatabase.GetUserIdByName("celebration_user")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	// A minimal but real GIF89a, the size an actual upload would be.
	gif := append([]byte("GIF89a"), bytes.Repeat([]byte{0x42}, 40*1024)...)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("winGif", "party.gif")
	_, _ = fw.Write(gif)
	_ = mw.Close()

	r := httptest.NewRequest("PUT", "/api/user/"+userId.String()+"/win-gif", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.SetPathValue("userId", userId.String())
	cookieRec := httptest.NewRecorder()
	gsAuth.SetUserId(cookieRec, userId)
	for _, c := range cookieRec.Result().Cookies() {
		r.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.SetWinGif)).ServeHTTP(rec, r)

	t.Logf("status=%d hx-refresh=%q body=%q", rec.Code, rec.Header().Get("HX-Refresh"), rec.Body.String())

	celebration, err := gsDatabase.GetUserWinCelebration(userId)
	t.Logf("stored HasGif=%v err=%v", celebration.HasGif, err)

	data, mime, err := gsDatabase.GetUserWinGif(userId)
	t.Logf("stored bytes=%d mime=%q err=%v", len(data), mime, err)

	if rec.Code != http.StatusOK {
		t.Errorf("upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if !celebration.HasGif {
		t.Errorf("gif was not stored")
	}
	if rec.Header().Get("HX-Refresh") != "" {
		t.Errorf("HX-Refresh should not be set (it collapses the <details>), got %q", rec.Header().Get("HX-Refresh"))
	}

	// PNG must also be accepted now.
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x11}, 20*1024)...)
	var pngBody bytes.Buffer
	pmw := multipart.NewWriter(&pngBody)
	pfw, _ := pmw.CreateFormFile("winGif", "party.png")
	_, _ = pfw.Write(png)
	_ = pmw.Close()

	pr := httptest.NewRequest("PUT", "/api/user/"+userId.String()+"/win-gif", &pngBody)
	pr.Header.Set("Content-Type", pmw.FormDataContentType())
	pr.SetPathValue("userId", userId.String())
	for _, c := range cookieRec.Result().Cookies() {
		pr.AddCookie(c)
	}
	prec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.SetWinGif)).ServeHTTP(prec, pr)
	t.Logf("PNG upload status=%d body=%q", prec.Code, prec.Body.String())
	if prec.Code != http.StatusOK {
		t.Errorf("PNG upload failed: %d %s", prec.Code, prec.Body.String())
	}
	_, pmime, _ := gsDatabase.GetUserWinGif(userId)
	if pmime != "image/png" {
		t.Errorf("expected image/png stored, got %q", pmime)
	}

	// A win message over 140 runes must be rejected.
	longMsg := strings.Repeat("x", 141)
	mr := httptest.NewRequest("PUT", "/api/user/"+userId.String()+"/win-message",
		strings.NewReader("winMessage="+longMsg))
	mr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mr.SetPathValue("userId", userId.String())
	for _, c := range cookieRec.Result().Cookies() {
		mr.AddCookie(c)
	}
	mrec := httptest.NewRecorder()
	gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.SetWinMessage)).ServeHTTP(mrec, mr)
	t.Logf("141-char message status=%d body=%q", mrec.Code, mrec.Body.String())
	if mrec.Code != http.StatusBadRequest {
		t.Errorf("expected 141-char message rejected, got %d", mrec.Code)
	}
}

// Regression test for a real GIF appearing to silently fail on upload.
// httptest.NewRecorder (used above) never touches a real socket, so it can't
// reproduce this: capping http.MaxBytesReader/ParseMultipartForm at the same
// size as the business rule meant any legitimately larger file tripped that
// cap mid-read and Go aborted the connection while the client was still
// writing it — a TCP reset the browser reports as net::ERR_CONNECTION_RESET,
// not the intended 400 response. The fix rejects on the client's declared
// Content-Length before ever reading the body, so the connection is never
// cut mid-stream. This drives the real handler over a real
// httptest.NewServer loopback connection with a file well over the
// configured 1000 KB limit and asserts a clean HTTP response comes back
// rather than a transport error.
func TestWinCelebrationOversizedUploadOverRealConnection(t *testing.T) {
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
	if err := gsDatabase.CreateUser("oversized_upload_user", "unused", true); err != nil {
		t.Logf("create user (may already exist): %v", err)
	}
	userId, err := gsDatabase.GetUserIdByName("oversized_upload_user")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("PUT /api/user/{userId}/win-gif", gsApi.MiddlewareForAPIs(http.HandlerFunc(gsApiUser.SetWinGif)))
	server := httptest.NewServer(mux)
	defer server.Close()

	cookieRec := httptest.NewRecorder()
	gsAuth.SetUserId(cookieRec, userId)

	// 1200 KB: comfortably over the configured 1000 KB limit — must come
	// back as a clean 400, not a broken connection.
	oversized := append([]byte("GIF89a"), bytes.Repeat([]byte{0x42}, 1200*1024)...)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("winGif", "big.gif")
	_, _ = fw.Write(oversized)
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPut, server.URL+"/api/user/"+userId.String()+"/win-gif", &body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, c := range cookieRec.Result().Cookies() {
		req.AddCookie(c)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("oversized upload should get a clean HTTP response, got a transport error instead: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	t.Logf("status=%d body=%q", resp.StatusCode, string(respBody))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for a 1200 KB file, got %d: %s", resp.StatusCode, respBody)
	}
}
