package apiCategory

import (
	"net/http"
	"strings"

	gsApi "github.com/gerp93/gameshell-framework/api"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
)

func Create(w http.ResponseWriter, r *http.Request) {
	// Category management is admin-only; the /categories page is gated by the
	// page policy, but these API endpoints go through MiddlewareForAPIs (login
	// only) so they must check admin themselves.
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
		_, _ = w.Write([]byte("A category name is required."))
		return
	}
	if len(name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Category name is too long (max 255 characters)."))
		return
	}

	exists, err := database.CategoryNameExists(name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if exists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("A category with that name already exists."))
		return
	}

	if _, err := database.CreateCategory(name); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// DeleteReassign deletes a category after moving every card in it to a target
// category chosen by the admin.
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
	if targetId == deleteId {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Choose a different category to move the cards to."))
		return
	}

	targetExists, err := database.CategoryExists(targetId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if !targetExists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("The chosen target category does not exist."))
		return
	}

	if err := database.DeleteCategoryReassigning(deleteId, targetId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
