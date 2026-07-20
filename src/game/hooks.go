package game

import (
	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
)

// TimelineTrivia implements gameshell.Game — the game's lifecycle hooks.
// Room/player state is created lazily from the API layer (see api/timelinetrivia)
// and cascades away when the framework deletes a LOBBY, so those hooks are
// no-ops. OnDeckDeleting audits the deck's cards before the framework removes
// the DECK (FK cascade would not fire the card audit trigger).
type TimelineTrivia struct{}

func (TimelineTrivia) OnRoomCreated(lobbyId uuid.UUID) error     { return nil }
func (TimelineTrivia) OnPlayerJoined(playerId uuid.UUID) error   { return nil }
func (TimelineTrivia) OnPlayerActive(playerId uuid.UUID) error   { return nil }
func (TimelineTrivia) OnPlayerInactive(playerId uuid.UUID) error { return nil }
func (TimelineTrivia) OnRoomEmpty(lobbyId uuid.UUID) error       { return nil }

func (TimelineTrivia) OnDeckDeleting(deckId uuid.UUID) error {
	return database.AuditDeckCardsAsDeleted(deckId)
}
