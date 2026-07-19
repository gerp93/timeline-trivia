package static

import "embed"

//go:embed *
var StaticFiles embed.FS

// SQLFiles is the ordered list of game SQL files to execute for database
// setup, run after the framework schema has been applied (the framework owns
// DECK/USER_ACCESS_DECK and deck management; CARD is game-owned and FKs to it).
// Order matters: tables -> triggers. Tables must be in dependency order.
var SQLFiles = []string{
	// tables
	"sql/tables/CARD.sql",
	"sql/tables/AUDIT_CARD.sql",
	"sql/tables/TIMELINE_TRIVIA_GAME.sql",
	"sql/tables/TIMELINE_TRIVIA_YEAR_RANGE.sql",
	"sql/tables/TIMELINE_TRIVIA_CARD_ATTEMPT.sql",
	"sql/tables/TIMELINE_TRIVIA_CURRENT_CARD.sql",
	"sql/tables/TIMELINE_TRIVIA_DRAW_PILE.sql",
	"sql/tables/TIMELINE_TRIVIA_PLAYER_TIMELINE.sql",

	// triggers
	"sql/triggers/TR_AUDIT_CARD_DELETE.sql",
	"sql/triggers/TR_AUDIT_CARD_UPDATE.sql",
	"sql/triggers/TR_SET_CHANGED_ON_DATE_BF_UP_CARD.sql",
}
