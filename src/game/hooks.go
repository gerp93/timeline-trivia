package game

import (
	"errors"

	"github.com/google/uuid"

	"github.com/gerp93/timeline-trivia/database"
)

// TimelineTrivia implements gameshell.Game — the game's lifecycle hooks —
// and gameshell.DeckCreationHook. Room/player state is created lazily from
// the API layer (see api/timelinetrivia) and cascades away when the
// framework deletes a LOBBY, so those hooks are no-ops. OnDeckDeleting
// audits the deck's cards before the framework removes the DECK (FK
// cascade would not fire the card audit trigger). OnDeckCreated assigns
// the new deck to whichever timeline its own "deck-create-extra-fields"
// block posted (see static/html/pages/body/deck-list-timeline-fields.html).
type TimelineTrivia struct{}

func (TimelineTrivia) OnRoomCreated(lobbyId uuid.UUID) error     { return nil }
func (TimelineTrivia) OnPlayerJoined(playerId uuid.UUID) error   { return nil }
func (TimelineTrivia) OnPlayerActive(playerId uuid.UUID) error   { return nil }
func (TimelineTrivia) OnPlayerInactive(playerId uuid.UUID) error { return nil }
func (TimelineTrivia) OnRoomEmpty(lobbyId uuid.UUID) error       { return nil }

func (TimelineTrivia) OnDeckDeleting(deckId uuid.UUID) error {
	return database.AuditDeckCardsAsDeleted(deckId)
}

func (TimelineTrivia) OnDeckCreated(deckId uuid.UUID, extraFields map[string]string) error {
	timelineId, err := uuid.Parse(extraFields["timelineId"])
	if err != nil {
		return errors.New("a timeline is required")
	}

	if err := database.ValidateTimelineAssignable(timelineId); err != nil {
		return err
	}

	return database.SetDeckTimeline(deckId, timelineId)
}
