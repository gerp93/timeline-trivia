package apiPages

import (
	"html/template"
	"net/http"
	"sort"
	"strconv"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
	"github.com/gerp93/timeline-trivia/static"
)

// Stats is the statistics hub: global top decades plus links into the other
// stats views.
func Stats(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics"

	topDecades, err := database.GetTopDecades(basePageData.User.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get top decades"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/stats.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		SelfUserId uuid.UUID
		TopDecades []database.TopDecade
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		SelfUserId:   basePageData.User.Id,
		TopDecades:   topDecades,
	})
}

// StatsLeaderboard shows the cross-user leaderboard (public decks only).
func StatsLeaderboard(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics - Leaderboard"

	entries, err := database.GetLeaderboard()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get leaderboard"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/stats-leaderboard.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Entries []database.LeaderboardEntry
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Entries:      entries,
	})
}

// StatsUsers lists users to pick whose detailed stats to view.
func StatsUsers(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics - Users"

	var name string
	var page int
	params := r.URL.Query()
	for key, val := range params {
		switch key {
		case "name":
			name = val[0]
		case "page":
			page, _ = strconv.Atoi(val[0])
		}
	}

	totalRowCount, err := gsDatabase.CountUsers(name)
	if err != nil {
		totalRowCount = 0
	}
	totalPageCount := max((totalRowCount+9)/10, 1)

	if page < 1 {
		page = 1
	}
	if page > totalPageCount {
		page = totalPageCount
	}

	users, err := gsDatabase.SearchUsers(name, page)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get table rows"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/stats-users.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Name     string
		Page     int
		LastPage int
		RowCount int
		Users    []gsDatabase.User
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Name:         name,
		Page:         page,
		LastPage:     totalPageCount,
		RowCount:     totalRowCount,
		Users:        users,
	})
}

// StatsUser shows one user's detailed stats: overall totals, decade rankings
// (most often, most/least successful past the significance floor), and
// per-category success. Everything is scoped to the viewer's readable decks.
func StatsUser(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics - User"

	targetId, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		http.Redirect(w, r, "/stats/users", http.StatusSeeOther)
		return
	}
	viewerId := basePageData.User.Id

	totals, err := database.GetUserStatTotals(viewerId, targetId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get user stats"))
		return
	}

	targetUser, err := gsDatabase.GetUser(targetId)
	if err != nil || targetUser.Id == uuid.Nil {
		http.Redirect(w, r, "/stats/users", http.StatusSeeOther)
		return
	}
	totals.Name = targetUser.Name

	decades, err := database.GetUserDecadeStats(viewerId, targetId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get decade stats"))
		return
	}

	categories, err := database.GetUserCategoryStats(viewerId, targetId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get category stats"))
		return
	}

	// Rank decades three ways. "Most often" uses every decade; "most/least
	// successful" only decades past the significance floor so a lucky handful
	// of guesses can't top the list.
	mostOften := append([]database.DecadeStat(nil), decades...)
	sort.SliceStable(mostOften, func(i, j int) bool {
		return mostOften[i].Attempts > mostOften[j].Attempts
	})
	mostOften = topDecades(mostOften, 5)

	qualified := make([]database.DecadeStat, 0, len(decades))
	for _, d := range decades {
		if d.Qualified() {
			qualified = append(qualified, d)
		}
	}
	mostSuccessful := append([]database.DecadeStat(nil), qualified...)
	sort.SliceStable(mostSuccessful, func(i, j int) bool {
		return mostSuccessful[i].Rate() > mostSuccessful[j].Rate()
	})
	mostSuccessful = topDecades(mostSuccessful, 5)

	leastSuccessful := append([]database.DecadeStat(nil), qualified...)
	sort.SliceStable(leastSuccessful, func(i, j int) bool {
		return leastSuccessful[i].Rate() < leastSuccessful[j].Rate()
	})
	leastSuccessful = topDecades(leastSuccessful, 5)

	// Categories ranked by success rate.
	categoriesRanked := append([]database.CategoryStat(nil), categories...)
	sort.SliceStable(categoriesRanked, func(i, j int) bool {
		return categoriesRanked[i].Rate() > categoriesRanked[j].Rate()
	})

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/stats-user.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Stat             database.StatUser
		MinDecadeGuesses int
		MostOften        []database.DecadeStat
		MostSuccessful   []database.DecadeStat
		LeastSuccessful  []database.DecadeStat
		Categories       []database.CategoryStat
		HasQualified     bool
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:     basePageData,
		Stat:             totals,
		MinDecadeGuesses: database.MinDecadeGuesses,
		MostOften:        mostOften,
		MostSuccessful:   mostSuccessful,
		LeastSuccessful:  leastSuccessful,
		Categories:       categoriesRanked,
		HasQualified:     len(qualified) > 0,
	})
}

// topDecades returns at most n entries from the front of the slice.
func topDecades(decades []database.DecadeStat, n int) []database.DecadeStat {
	if len(decades) > n {
		return decades[:n]
	}
	return decades
}

// StatsCards is the card picker for per-card stats, scoped to readable decks.
func StatsCards(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics - Cards"

	var text string
	var page int
	params := r.URL.Query()
	for key, val := range params {
		switch key {
		case "text":
			text = val[0]
		case "page":
			page, _ = strconv.Atoi(val[0])
		}
	}

	viewerId := basePageData.User.Id

	totalRowCount, err := database.CountStatCardsWithAccess(viewerId, text)
	if err != nil {
		totalRowCount = 0
	}
	totalPageCount := max((totalRowCount+9)/10, 1)

	if page < 1 {
		page = 1
	}
	if page > totalPageCount {
		page = totalPageCount
	}

	cards, err := database.SearchStatCardsWithAccess(viewerId, text, page)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get table rows"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/stats-cards.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Text     string
		Page     int
		LastPage int
		RowCount int
		Cards    []database.StatCardRow
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Text:         text,
		Page:         page,
		LastPage:     totalPageCount,
		RowCount:     totalRowCount,
		Cards:        cards,
	})
}

// StatsCard shows the play record for a single card, gated on the viewer being
// able to read the card's deck.
func StatsCard(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics - Card"

	cardId, err := uuid.Parse(r.PathValue("cardId"))
	if err != nil {
		http.Redirect(w, r, "/stats/cards", http.StatusSeeOther)
		return
	}

	deckId, err := database.GetCardDeckId(cardId)
	if err != nil || deckId == uuid.Nil {
		http.Redirect(w, r, "/stats/cards", http.StatusSeeOther)
		return
	}

	canRead, err := database.UserCanReadDeck(basePageData.User.Id, deckId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to check deck access"))
		return
	}
	if !canRead {
		http.Redirect(w, r, "/stats/cards", http.StatusSeeOther)
		return
	}

	stat, err := database.GetCardStats(cardId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get card stats"))
		return
	}

	tmpl, err := template.ParseFS(
		static.StaticFiles,
		"html/pages/base.html",
		"html/pages/body/stats-card.html",
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Stat database.StatCard
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData: basePageData,
		Stat:         stat,
	})
}
