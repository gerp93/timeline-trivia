package apiTimeline

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
)

// parseOptionalYear turns a form value into a nullable year, mirroring
// apiCard's parseYear. Empty = NULL (open-ended bound).
func parseOptionalYear(value string) (sql.NullInt64, bool) {
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

// hasDeckAccess mirrors apiCard's helper of the same shape — any user who
// can edit a deck's cards can also set which timeline it belongs to.
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

// SetDeckTimeline assigns which timeline a deck belongs to.
func SetDeckTimeline(w http.ResponseWriter, r *http.Request) {
	deckId, err := uuid.Parse(r.PathValue("deckId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get deck id from path."))
		return
	}

	if !hasDeckAccess(w, r, deckId) {
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	timelineId, err := uuid.Parse(r.FormValue("timelineId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Invalid timeline."))
		return
	}

	exists, err := database.TimelineExists(timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if !exists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Selected timeline does not exist."))
		return
	}

	// A deck can only be assigned to a timeline that has at least one era —
	// otherwise card create/edit would have nothing to offer in the Era
	// dropdown, and there'd be no way to author a card for it at all.
	eras, err := database.GetErasForTimeline(timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Failed to check timeline eras."))
		return
	}
	if len(eras) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("This timeline has no eras yet. Add at least one era before assigning decks to it."))
		return
	}

	// Same invariant as eras, but with higher stakes: category is a
	// required field on every card, so a deck with nothing to offer in the
	// Category dropdown couldn't author any card at all.
	categories, err := database.GetCategoriesForTimeline(timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Failed to check timeline categories."))
		return
	}
	if len(categories) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("This timeline has no categories yet. Add at least one category before assigning decks to it."))
		return
	}

	if err := database.SetDeckTimeline(deckId, timelineId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// Create creates a new timeline. Admin-only; the /timelines page is gated
// by the page policy, but this API endpoint goes through
// MiddlewareForAPIs (login only) so it must check admin itself.
func Create(w http.ResponseWriter, r *http.Request) {
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

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("A timeline name is required."))
		return
	}
	if len(name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Timeline name is too long (max 255 characters)."))
		return
	}

	exists, err := database.TimelineNameExists(name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if exists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("A timeline with that name already exists."))
		return
	}

	if _, err := database.CreateTimeline(name); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// DeleteReassign deletes a timeline after moving every deck belonging to it
// to a target timeline chosen by the admin. The default "Real Life"
// timeline can never be deleted.
func DeleteReassign(w http.ResponseWriter, r *http.Request) {
	if !gsApi.UserIsAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	deleteId, err := uuid.Parse(r.PathValue("timelineId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get timeline id from path."))
		return
	}

	if deleteId == database.DefaultTimelineId {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("The default Real Life timeline cannot be deleted."))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	targetId, err := uuid.Parse(r.FormValue("targetTimelineId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Please choose a timeline to move the decks to."))
		return
	}
	if targetId == deleteId {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Choose a different timeline to move the decks to."))
		return
	}

	targetExists, err := database.TimelineExists(targetId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if !targetExists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("The chosen target timeline does not exist."))
		return
	}

	if err := database.DeleteTimelineReassigning(deleteId, targetId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// CreateEra adds an era to a timeline. Admin-only.
func CreateEra(w http.ResponseWriter, r *http.Request) {
	if !gsApi.UserIsAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	timelineId, err := uuid.Parse(r.PathValue("timelineId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get timeline id from path."))
		return
	}

	timelineExists, err := database.TimelineExists(timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if !timelineExists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Timeline does not exist."))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("An era name is required."))
		return
	}
	if len(name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Era name is too long (max 255 characters)."))
		return
	}

	abbreviation := strings.TrimSpace(r.FormValue("abbreviation"))
	if len(abbreviation) > 50 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Abbreviation is too long (max 50 characters)."))
		return
	}

	sortOrder, err := strconv.Atoi(strings.TrimSpace(r.FormValue("sortOrder")))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Order must be a whole number."))
		return
	}

	fromYear, ok := parseOptionalYear(r.FormValue("fromYear"))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("From year must be a whole number."))
		return
	}
	toYear, ok := parseOptionalYear(r.FormValue("toYear"))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("To year must be a whole number."))
		return
	}
	if fromYear.Valid && toYear.Valid && fromYear.Int64 > toYear.Int64 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("From year must not be after to year."))
		return
	}

	epochOffsetStr := strings.TrimSpace(r.FormValue("epochOffset"))
	epochOffset := 0
	if epochOffsetStr != "" {
		var offsetErr error
		epochOffset, offsetErr = strconv.Atoi(epochOffsetStr)
		if offsetErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Epoch offset must be a whole number."))
			return
		}
	}

	direction := strings.TrimSpace(r.FormValue("direction"))
	if direction != database.EraDirectionForward && direction != database.EraDirectionBackward {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Direction must be either forward or backward."))
		return
	}

	if _, err := database.CreateEra(timelineId, name, abbreviation, sortOrder, fromYear, toYear, epochOffset, direction); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// DeleteEra removes an era. Admin-only. No reassignment needed — see
// database.DeleteEra.
func DeleteEra(w http.ResponseWriter, r *http.Request) {
	if !gsApi.UserIsAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	eraId, err := uuid.Parse(r.PathValue("eraId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get era id from path."))
		return
	}

	if err := database.DeleteEra(eraId); err != nil {
		switch {
		case errors.Is(err, database.ErrEraNotFound):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, database.ErrEraInUse):
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
