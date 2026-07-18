package game

import "github.com/google/uuid"

// CardTimeline implements gameshell.Game — Chronology's room/player
// lifecycle hooks. The game creates/reads its own state lazily from the
// API layer (see api/chronology), so these hooks are no-ops: CHRONOLOGY_GAME
// rows are created on first access, and CHRONOLOGY_GAME cascades away
// automatically when the framework deletes its LOBBY row (FK ON DELETE
// CASCADE).
type CardTimeline struct{}

func (CardTimeline) OnRoomCreated(lobbyId uuid.UUID) error     { return nil }
func (CardTimeline) OnPlayerJoined(playerId uuid.UUID) error   { return nil }
func (CardTimeline) OnPlayerActive(playerId uuid.UUID) error   { return nil }
func (CardTimeline) OnPlayerInactive(playerId uuid.UUID) error { return nil }
func (CardTimeline) OnRoomEmpty(lobbyId uuid.UUID) error       { return nil }
