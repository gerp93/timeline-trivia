package game

import (
	"github.com/google/uuid"

	"github.com/gerp93/card-timeline/database"
)

// CardTimeline implements gameshell.Game — TimelineTrivia's lifecycle hooks.
// Room/player state is created lazily from the API layer (see api/timelinetrivia)
// and cascades away when the framework deletes a LOBBY, so those hooks are
// no-ops. OnDeckDeleting audits the deck's cards before the framework removes
// the DECK (FK cascade would not fire the card audit trigger).
type CardTimeline struct{}

func (CardTimeline) OnRoomCreated(lobbyId uuid.UUID) error     { return nil }
func (CardTimeline) OnPlayerJoined(playerId uuid.UUID) error   { return nil }
func (CardTimeline) OnPlayerActive(playerId uuid.UUID) error   { return nil }
func (CardTimeline) OnPlayerInactive(playerId uuid.UUID) error { return nil }
func (CardTimeline) OnRoomEmpty(lobbyId uuid.UUID) error       { return nil }

func (CardTimeline) OnDeckDeleting(deckId uuid.UUID) error {
	return database.AuditDeckCardsAsDeleted(deckId)
}
