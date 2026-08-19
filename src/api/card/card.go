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

// resolveYear turns the posted eraId+relativeYear into a nullable absolute
// CARD_YEAR, or a non-empty error message. A blank relativeYear means unset
// (NULL — a card can still be authored-but-incomplete, same as before). A
// non-blank relativeYear requires a real era belonging to the deck's own
// timeline — decks can only ever be assigned to timelines with at least
// one era (see apiTimeline.SetDeckTimeline), so an era is always available
// to pick. The computed absolute year is rejected if it falls outside the
// selected era's own range — otherwise a card tagged "Third Age" could
// silently land under whatever era its year actually resolves to at
// display time, quietly contradicting what was picked at entry time.
func resolveYear(deckId uuid.UUID, eraIdStr string, relativeYearStr string) (sql.NullInt64, string) {
	relativeYearStr = strings.TrimSpace(relativeYearStr)
	if relativeYearStr == "" {
		return sql.NullInt64{}, ""
	}
	relativeYear, err := strconv.Atoi(relativeYearStr)
	if err != nil {
		return sql.NullInt64{}, "Year must be a whole number."
	}

	eraIdStr = strings.TrimSpace(eraIdStr)
	if eraIdStr == "" {
		return sql.NullInt64{}, "An era is required."
	}

	eraId, err := uuid.Parse(eraIdStr)
	if err != nil {
		return sql.NullInt64{}, "Invalid era."
	}
	era, err := database.GetEra(eraId)
	if err != nil {
		return sql.NullInt64{}, "Failed to check era."
	}
	if era.Id == uuid.Nil {
		return sql.NullInt64{}, "Selected era does not exist."
	}

	timelineId, err := database.GetDeckTimelineId(deckId)
	if err != nil {
		return sql.NullInt64{}, "Failed to check deck timeline."
	}
	if era.TimelineId != timelineId {
		return sql.NullInt64{}, "Selected era does not belong to this deck's timeline."
	}

	absolute := database.AbsoluteYearFromEra(era, relativeYear)
	if !database.EraContainsAbsoluteYear(absolute, era) {
		// Direction-aware: whichever bound was actually violated is
		// translated back into "year within era" terms (the units the
		// caller typed in) by comparing the typed value against that
		// bound's own translated value, rather than assuming which
		// physical bound (From/To) is the min vs the max — that flips for
		// a Backward era (see database.RelativeYearInEra).
		if era.FromYear.Valid && int64(absolute) < era.FromYear.Int64 {
			limit := database.RelativeYearInEra(int(era.FromYear.Int64), era)
			if relativeYear < limit {
				return sql.NullInt64{}, fmt.Sprintf("Year within %s must be at least %d.", era.Name, limit)
			}
			return sql.NullInt64{}, fmt.Sprintf("Year within %s must be at most %d.", era.Name, limit)
		}
		limit := database.RelativeYearInEra(int(era.ToYear.Int64), era)
		if relativeYear > limit {
			return sql.NullInt64{}, fmt.Sprintf("Year within %s must be at most %d.", era.Name, limit)
		}
		return sql.NullInt64{}, fmt.Sprintf("Year within %s must be at least %d.", era.Name, limit)
	}

	return sql.NullInt64{Int64: int64(absolute), Valid: true}, ""
}

// parseCategoryId parses the required categoryId form value and confirms it
// belongs to the deck's own timeline (categories are scoped to a timeline,
// the same way eras are — see resolveYear). Returns a non-empty error
// message on any problem (missing, malformed, unknown, or wrong-timeline
// category).
func parseCategoryId(deckId uuid.UUID, value string) (uuid.NullUUID, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.NullUUID{}, "A category is required."
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.NullUUID{}, "Invalid category."
	}
	category, err := database.GetCategory(id)
	if err != nil {
		return uuid.NullUUID{}, "Failed to check category."
	}
	if category.Id == uuid.Nil {
		return uuid.NullUUID{}, "Selected category does not exist."
	}

	timelineId, err := database.GetDeckTimelineId(deckId)
	if err != nil {
		return uuid.NullUUID{}, "Failed to check deck timeline."
	}
	if category.TimelineId != timelineId {
		return uuid.NullUUID{}, "Selected category does not belong to this deck's timeline."
	}

	return uuid.NullUUID{UUID: id, Valid: true}, ""
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

	year, yearErr := resolveYear(deckId, r.FormValue("eraId"), r.FormValue("relativeYear"))
	if yearErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(yearErr))
		return
	}

	categoryId, categoryErr := parseCategoryId(deckId, r.FormValue("categoryId"))
	if categoryErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(categoryErr))
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

	if _, err := database.CreateCard(deckId, text, year, categoryId); err != nil {
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

	year, yearErr := resolveYear(deckId, r.FormValue("eraId"), r.FormValue("relativeYear"))
	if yearErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(yearErr))
		return
	}

	categoryId, categoryErr := parseCategoryId(deckId, r.FormValue("categoryId"))
	if categoryErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(categoryErr))
		return
	}

	if err := database.UpdateCard(cardId, text, year, categoryId); err != nil {
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

// Unflag accepts a card back out of purgatory unchanged, making it eligible
// for draw piles again. Admin-only: the review screen is the only caller.
func Unflag(w http.ResponseWriter, r *http.Request) {
	cardId, err := uuid.Parse(r.PathValue("cardId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get card id from path."))
		return
	}

	if !gsApi.UserIsAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	if err := database.UnflagCard(cardId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// UpdateFlagged is "edit and accept" from the review screen: fix the card's
// text/year/category and take it out of purgatory in one step, so a reviewer
// can't accidentally accept a card they meant to correct first. Admin-only.
func UpdateFlagged(w http.ResponseWriter, r *http.Request) {
	cardId, err := uuid.Parse(r.PathValue("cardId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get card id from path."))
		return
	}

	if !gsApi.UserIsAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	// UpdateFlagged's form has no hidden deckId field (it authorizes purely
	// via admin check, not deck access) — the card's own deck is needed here
	// to resolve which timeline's eras an eraId must belong to.
	card, err := database.GetCard(cardId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if card.Id == uuid.Nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Card not found."))
		return
	}

	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No text found."))
		return
	}

	year, yearErr := resolveYear(card.DeckId, r.FormValue("eraId"), r.FormValue("relativeYear"))
	if yearErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(yearErr))
		return
	}

	categoryId, categoryErr := parseCategoryId(card.DeckId, r.FormValue("categoryId"))
	if categoryErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(categoryErr))
		return
	}

	if err := database.UpdateCard(cardId, text, year, categoryId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if err := database.UnflagCard(cardId); err != nil {
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
		category := ""
		if card.CategoryName.Valid {
			category = card.CategoryName.String
		}
		_ = writer.Write([]string{card.Text, year, category})
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
