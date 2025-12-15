package static

import "embed"

//go:embed *
var StaticFiles embed.FS

// SQLFiles is the ordered list of SQL files to execute for database setup.
// Order matters: settings -> tables -> functions -> procedures -> events -> triggers
// Tables must be in dependency order (e.g., DECK before CARD).
var SQLFiles = []string{
	// database
	"sql/settings.sql",

	// tables
	"sql/tables/USER.sql",
	"sql/tables/DECK.sql",
	"sql/tables/CARD.sql",
	"sql/tables/LOBBY.sql",
	"sql/tables/PLAYER.sql",
	"sql/tables/DRAW_PILE.sql",
	"sql/tables/USER_ACCESS_DECK.sql",
	"sql/tables/LOGIN_ATTEMPT.sql",
	"sql/tables/AUDIT_CARD.sql",
	"sql/tables/AUDIT_DECK.sql",
	"sql/tables/AUDIT_USER.sql",
	"sql/tables/CHRONOLOGY_GAME.sql",
	"sql/tables/CHRONOLOGY_CURRENT_CARD.sql",
	"sql/tables/CHRONOLOGY_DRAW_PILE.sql",
	"sql/tables/CHRONOLOGY_PLAYER_TIMELINE.sql",

	// functions
	"sql/functions/FN_GET_LOGIN_ATTEMPT_IS_ALLOWED.sql",
	"sql/functions/FN_USER_HAS_DECK_ACCESS.sql",

	// procedures
	"sql/procedures/SP_GET_READABLE_DECKS.sql",
	"sql/procedures/SP_ALERT_LOBBY.sql",

	// events
	"sql/events/EVT_CLEAN_LOGIN_ATTEMPTS.sql",

	// triggers
	"sql/triggers/TR_AUDIT_CARD_DELETE.sql",
	"sql/triggers/TR_AUDIT_CARD_UPDATE.sql",
	"sql/triggers/TR_AUDIT_DECK_DELETE.sql",
	"sql/triggers/TR_AUDIT_DECK_UPDATE.sql",
	"sql/triggers/TR_AUDIT_USER_DELETE.sql",
	"sql/triggers/TR_AUDIT_USER_UPDATE.sql",
	"sql/triggers/TR_REVOKE_ACCESS_AF_UP_DECK.sql",
	"sql/triggers/TR_SET_CHANGED_ON_DATE_BF_UP_CARD.sql",
	"sql/triggers/TR_SET_CHANGED_ON_DATE_BF_UP_DECK.sql",
	"sql/triggers/TR_SET_CHANGED_ON_DATE_BF_UP_USER.sql",
}
