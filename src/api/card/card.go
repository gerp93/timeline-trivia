package apiCard

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
)

// maxImportUploadBytes bounds how much request body an /card-import request
// may send, independent of how many cards that JSON decodes to (which
// database.ParseCardImportJSON separately caps) — this stops a client from
// making the server buffer and parse an arbitrarily large body at all.
const maxImportUploadBytes = 2 << 20 // 2 MiB

// parseYear turns a form value into a nullable year. Empty = NULL.
func parseYear(value string) (sql.NullInt64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullInt64{}, true
	}
	year, err := strconv.Atoi(value)
	if err != nil {
		return sql.NullInt64{}, false
	}
	return sql.NullInt64{Int64: int64(year), Valid: true}, true
}

func hasDeckAccess(w http.ResponseWriter, r *http.Request, deckId uuid.UUID) bool {
	userId := gsApi.GetUserId(r)
	if userId == uuid.Nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id."))
		return false
	}
	ok, err := gsDatabase.UserHasDeckAccess(userId, deckId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Failed to check deck access."))
		return false
	}
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return false
	}
	return true
}

func Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	deckId, err := uuid.Parse(r.FormValue("deckId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get deck id."))
		return
	}

	if !hasDeckAccess(w, r, deckId) {
		return
	}

	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No text found."))
		return
	}

	year, ok := parseYear(r.FormValue("year"))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Year must be a whole number."))
		return
	}

	existingCardId, err := database.GetCardId(deckId, text)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if existingCardId != uuid.Nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Card text already exists in this deck."))
		return
	}

	if _, err := database.CreateCard(deckId, text, year); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func Update(w http.ResponseWriter, r *http.Request) {
	cardId, err := uuid.Parse(r.PathValue("cardId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get card id from path."))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	deckId, err := uuid.Parse(r.FormValue("deckId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get deck id."))
		return
	}

	if !hasDeckAccess(w, r, deckId) {
		return
	}

	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No text found."))
		return
	}

	year, ok := parseYear(r.FormValue("year"))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Year must be a whole number."))
		return
	}

	if err := database.UpdateCard(cardId, text, year); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func Delete(w http.ResponseWriter, r *http.Request) {
	cardId, err := uuid.Parse(r.PathValue("cardId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get card id from path."))
		return
	}

	card, err := database.GetCard(cardId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if !hasDeckAccess(w, r, card.DeckId) {
		return
	}

	if err := database.DeleteCard(cardId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func GetCardExport(w http.ResponseWriter, r *http.Request) {
	deckId, err := uuid.Parse(r.PathValue("deckId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get deck id from path."))
		return
	}

	if !hasDeckAccess(w, r, deckId) {
		return
	}

	cards, err := database.GetCardsInDeckExport(deckId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	writer := csv.NewWriter(w)
	defer writer.Flush()
	for _, card := range cards {
		year := ""
		if card.Year.Valid {
			year = strconv.FormatInt(card.Year.Int64, 10)
		}
		_ = writer.Write([]string{card.Text, year})
	}
}

// ImportJSON accepts an uploaded JSON file of
// [{"year": number, "event": string, "category": string}, ...] and inserts
// any cards from it that aren't already in the deck (matched by event
// text). See database.ParseCardImportJSON for the exact, strictly enforced
// schema — anything else is rejected.
func ImportJSON(w http.ResponseWriter, r *http.Request) {
	deckId, err := uuid.Parse(r.PathValue("deckId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get deck id from path."))
		return
	}

	if !hasDeckAccess(w, r, deckId) {
		return
	}

	// Cap the request body before doing any work with it, independent of
	// what the client claims Content-Length is.
	r.Body = http.MaxBytesReader(w, r.Body, maxImportUploadBytes)
	if err := r.ParseMultipartForm(maxImportUploadBytes); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(fmt.Sprintf("Upload too large or malformed (max %d MB).", maxImportUploadBytes/(1<<20))))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No file found in upload."))
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".json") {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("File must be a .json file."))
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to read uploaded file."))
		return
	}

	imported, skipped, err := database.ImportCardsIntoDeck(deckId, data)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("Imported %d card(s); skipped %d already in this deck.", imported, skipped)))
}
