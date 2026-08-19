package apiCategory

import (
	"errors"
	"net/http"
	"strings"

	gsApi "github.com/gerp93/gameshell-framework/api"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
)

// Create adds a category to a timeline. Admin-only; the /timelines page is
// gated by the page policy, but this API endpoint goes through
// MiddlewareForAPIs (login only) so it must check admin itself.
func Create(w http.ResponseWriter, r *http.Request) {
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
		_, _ = w.Write([]byte("A category name is required."))
		return
	}
	if len(name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Category name is too long (max 255 characters)."))
		return
	}

	exists, err := database.CategoryNameExistsInTimeline(timelineId, name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if exists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("A category with that name already exists for this timeline."))
		return
	}

	if _, err := database.CreateCategory(timelineId, name); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// DeleteReassign deletes a category after moving every card in it to a
// target category (which must belong to the same timeline) chosen by the
// admin. Admin-only.
func DeleteReassign(w http.ResponseWriter, r *http.Request) {
	if !gsApi.UserIsAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	deleteId, err := uuid.Parse(r.PathValue("categoryId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get category id from path."))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	targetId, err := uuid.Parse(r.FormValue("targetCategoryId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Please choose a category to move the cards to."))
		return
	}

	if err := database.DeleteCategoryReassigning(deleteId, targetId); err != nil {
		switch {
		case errors.Is(err, database.ErrCategoryNotFound), errors.Is(err, database.ErrCategoryTargetNotFound):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, database.ErrCategorySelfReassign), errors.Is(err, database.ErrCategoryTimelineMismatch):
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
