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
)

// parseUUIDQueryParam parses a query-string UUID, returning uuid.Nil for an
// empty or malformed value rather than erroring — every stats page treats
// uuid.Nil as "no filter" (Overall/whole-timeline), so a missing or garbled
// param degrades to that instead of failing the request.
func parseUUIDQueryParam(value string) uuid.UUID {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// timelineFuncMap is shared by the stats pages that render a timeline (and,
// on stats-user.html, era) selector — "isNilUUID" flags the "Overall"/
// "whole timeline" option as selected, which text/template's built-in `not`
// can't do for a fixed-size array type like uuid.UUID (its zero value has
// the same length as any other, so `not` always sees it as non-empty).
var timelineFuncMap = template.FuncMap{
	"isNilUUID": func(id uuid.UUID) bool { return id == uuid.Nil },
}

// Stats is the statistics hub: top decades (or, with a timeline selected,
// top eras) plus links into the other stats views.
func Stats(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics"

	timelineId := parseUUIDQueryParam(r.URL.Query().Get("timeline"))

	timelines, err := database.GetTimelines()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get timelines"))
		return
	}

	var topDecadesResult []database.TopDecade
	var topErasResult []database.TopEra
	if timelineId == uuid.Nil {
		topDecadesResult, err = database.GetTopDecades(basePageData.User.Id)
	} else {
		topErasResult, err = database.GetTopErasForTimeline(basePageData.User.Id, timelineId)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get top decades"))
		return
	}

	tmpl, err := parseChrome("html/pages/body/stats.html", timelineFuncMap)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		SelfUserId       uuid.UUID
		Timelines        []database.Timeline
		SelectedTimeline uuid.UUID
		TopDecades       []database.TopDecade
		TopEras          []database.TopEra
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:     basePageData,
		SelfUserId:       basePageData.User.Id,
		Timelines:        timelines,
		SelectedTimeline: timelineId,
		TopDecades:       topDecadesResult,
		TopEras:          topErasResult,
	})
}

// StatsLeaderboard shows the cross-user leaderboard (public decks only),
// optionally scoped to one timeline.
func StatsLeaderboard(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics - Leaderboard"

	timelineId := parseUUIDQueryParam(r.URL.Query().Get("timeline"))

	timelines, err := database.GetTimelines()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get timelines"))
		return
	}

	entries, err := database.GetLeaderboard(timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get leaderboard"))
		return
	}

	tmpl, err := parseChrome("html/pages/body/stats-leaderboard.html", timelineFuncMap)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Timelines        []database.Timeline
		SelectedTimeline uuid.UUID
		Entries          []database.LeaderboardEntry
	}

	_ = tmpl.ExecuteTemplate(w, "base", data{
		BasePageData:     basePageData,
		Timelines:        timelines,
		SelectedTimeline: timelineId,
		Entries:          entries,
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

	tmpl, err := parseChrome("html/pages/body/stats-users.html", nil)
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

// StatsUser shows one user's detailed stats: overall totals, decade/era
// rankings (most often, most/least successful past the significance
// floor), and per-category success. Everything is scoped to the viewer's
// readable decks, and optionally to one timeline (in which case the decade
// breakdown is replaced by an era breakdown for that timeline) and, within
// a timeline, further to one era.
func StatsUser(w http.ResponseWriter, r *http.Request) {
	basePageData := gsApi.GetBasePageData(r)
	basePageData.PageTitle = "Timeline Trivia - Statistics - User"

	targetId, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		http.Redirect(w, r, "/stats/users", http.StatusSeeOther)
		return
	}
	viewerId := basePageData.User.Id

	params := r.URL.Query()
	timelineId := parseUUIDQueryParam(params.Get("timeline"))
	eraId := parseUUIDQueryParam(params.Get("era"))
	// An era filter only makes sense alongside its own timeline; a stale or
	// manually-edited era param with no timeline selected is ignored.
	if timelineId == uuid.Nil {
		eraId = uuid.Nil
	}

	timelines, err := database.GetTimelines()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get timelines"))
		return
	}

	var eras []database.Era
	if timelineId != uuid.Nil {
		eras, err = database.GetErasForTimeline(timelineId)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("failed to get eras"))
			return
		}
	}

	totals, err := database.GetUserStatTotals(viewerId, targetId, timelineId, eraId)
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

	categories, err := database.GetUserCategoryStats(viewerId, targetId, timelineId, eraId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get category stats"))
		return
	}

	timeouts, err := database.GetUserTimeoutStats(viewerId, targetId, timelineId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to get timeout stats"))
		return
	}

	// Categories ranked by success rate.
	categoriesRanked := append([]database.CategoryStat(nil), categories...)
	sort.SliceStable(categoriesRanked, func(i, j int) bool {
		return categoriesRanked[i].Rate() > categoriesRanked[j].Rate()
	})

	tmpl, err := parseChrome("html/pages/body/stats-user.html", timelineFuncMap)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse HTML"))
		return
	}

	type data struct {
		gsApi.BasePageData
		Stat               database.StatUser
		MinBucketGuesses   int
		Timelines          []database.Timeline
		SelectedTimeline   uuid.UUID
		Eras               []database.Era
		SelectedEra        uuid.UUID
		MostOften          []database.DecadeStat
		MostSuccessful     []database.DecadeStat
		LeastSuccessful    []database.DecadeStat
		HasQualified       bool
		EraMostOften       []database.EraStat
		EraMostSuccessful  []database.EraStat
		EraLeastSuccessful []database.EraStat
		HasEraQualified    bool
		Categories         []database.CategoryStat
		Timeouts           database.UserTimeoutStats
	}

	d := data{
		BasePageData:     basePageData,
		Stat:             totals,
		MinBucketGuesses: database.MinBucketGuesses,
		Timelines:        timelines,
		SelectedTimeline: timelineId,
		Eras:             eras,
		SelectedEra:      eraId,
		Categories:       categoriesRanked,
		Timeouts:         timeouts,
	}

	if timelineId == uuid.Nil {
		// "Overall": rank decades three ways, same as before this feature.
		// "Most often" uses every decade; "most/least successful" only
		// decades past the significance floor so a lucky handful of
		// guesses can't top the list.
		decades, err := database.GetUserDecadeStats(viewerId, targetId)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("failed to get decade stats"))
			return
		}

		mostOften := append([]database.DecadeStat(nil), decades...)
		sort.SliceStable(mostOften, func(i, j int) bool {
			return mostOften[i].Attempts > mostOften[j].Attempts
		})
		d.MostOften = topDecades(mostOften, 5)

		qualified := make([]database.DecadeStat, 0, len(decades))
		for _, dd := range decades {
			if dd.Qualified() {
				qualified = append(qualified, dd)
			}
		}
		d.MostSuccessful, d.LeastSuccessful = splitSuccessfulDecades(qualified)
		d.HasQualified = len(d.MostSuccessful) > 0
	} else {
		// One timeline selected: same three-way ranking, but bucketed by
		// that timeline's own eras instead of decades.
		eraStats, err := database.GetUserEraStats(viewerId, targetId, timelineId)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("failed to get era stats"))
			return
		}

		mostOften := append([]database.EraStat(nil), eraStats...)
		sort.SliceStable(mostOften, func(i, j int) bool {
			return mostOften[i].Attempts > mostOften[j].Attempts
		})
		d.EraMostOften = topEras(mostOften, 5)

		qualified := make([]database.EraStat, 0, len(eraStats))
		for _, e := range eraStats {
			if e.Qualified() {
				qualified = append(qualified, e)
			}
		}
		d.EraMostSuccessful, d.EraLeastSuccessful = splitSuccessfulEras(qualified)
		d.HasEraQualified = len(d.EraMostSuccessful) > 0
	}

	_ = tmpl.ExecuteTemplate(w, "base", d)
}

// topDecades returns at most n entries from the front of the slice.
func topDecades(decades []database.DecadeStat, n int) []database.DecadeStat {
	if len(decades) > n {
		return decades[:n]
	}
	return decades
}

// topEras is topDecades's era counterpart.
func topEras(eras []database.EraStat, n int) []database.EraStat {
	if len(eras) > n {
		return eras[:n]
	}
	return eras
}

// splitSuccessfulDecades ranks the qualified decades by accuracy and returns
// the best and worst ends of that ranking, best-first and worst-first
// respectively.
//
// Both lists come from ONE descending ranking, taking opposite ends, so they
// are disjoint by construction. Ranking twice (once each way) and capping
// each by count is NOT enough: with tied rates a stable sort leaves the same
// decade first in both directions, so it showed up as both most and least
// successful. Real data hit this — four qualified decades at 83/80/80/70%
// put the first 80% decade at the top of both lists.
func splitSuccessfulDecades(qualified []database.DecadeStat) (most, least []database.DecadeStat) {
	ranked := append([]database.DecadeStat(nil), qualified...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Rate() > ranked[j].Rate()
	})

	n := successfulSplitSize(len(ranked))

	most = ranked[:n]
	least = make([]database.DecadeStat, 0, n)
	for i := len(ranked) - 1; i >= len(ranked)-n; i-- {
		least = append(least, ranked[i])
	}
	return most, least
}

// splitSuccessfulEras is splitSuccessfulDecades's era counterpart — same
// tie-breaking rationale applies.
func splitSuccessfulEras(qualified []database.EraStat) (most, least []database.EraStat) {
	ranked := append([]database.EraStat(nil), qualified...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Rate() > ranked[j].Rate()
	})

	n := successfulSplitSize(len(ranked))

	most = ranked[:n]
	least = make([]database.EraStat, 0, n)
	for i := len(ranked) - 1; i >= len(ranked)-n; i-- {
		least = append(least, ranked[i])
	}
	return most, least
}

// successfulSplitSize returns how many decades (or eras) each of "most
// successful" and "least successful" should show. Capped at half of
// qualifiedCount so the two lists can never share an entry — a flat cap of
// 5 would, with fewer than 10 qualified entries, put the same entries in
// both lists (just reordered), which reads as a broken/duplicated page
// rather than as "not enough data yet". Also capped at 5 so the lists
// don't grow unbounded.
func successfulSplitSize(qualifiedCount int) int {
	n := qualifiedCount / 2
	if n > 5 {
		n = 5
	}
	return n
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

	tmpl, err := parseChrome("html/pages/body/stats-cards.html", nil)
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

	tmpl, err := parseChrome("html/pages/body/stats-card.html", nil)
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
