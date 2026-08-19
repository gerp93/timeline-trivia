-- Which timeline a deck belongs to. At most one row per deck (DECK_ID is
-- the primary key); a deck with no row here is lazily treated as belonging
-- to the default "Real Life" timeline, the same convention CARD.CATEGORY_ID
-- and CARD.CARD_YEAR use for "unset means excluded/defaulted".
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_DECK_TIMELINE(
    DECK_ID UUID NOT NULL,
    TIMELINE_TRIVIA_TIMELINE_ID UUID NOT NULL,
    PRIMARY KEY(DECK_ID),
    FOREIGN KEY(DECK_ID) REFERENCES DECK(ID) ON DELETE CASCADE,
    FOREIGN KEY(TIMELINE_TRIVIA_TIMELINE_ID) REFERENCES TIMELINE_TRIVIA_TIMELINE(ID) ON DELETE CASCADE
);
