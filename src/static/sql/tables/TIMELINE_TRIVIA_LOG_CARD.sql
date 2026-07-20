-- Append-only log of card lifecycle events during play. DRAW is written each
-- time a card becomes the event to guess; DISCARD when every active player
-- missed it and it was thrown away. No foreign keys (see LOG_GUESS): survives
-- lobby deletion, joined back to CARD by CARD_ID for card/deck/decade stats.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_LOG_CARD(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    CARD_ID UUID NOT NULL,
    EVENT_TYPE ENUM('DRAW', 'DISCARD') NOT NULL,
    PRIMARY KEY(ID)
);
