-- A timeline is an alternate history/universe a lobby can be created
-- against (e.g. "Real Life", "Star Wars"). Decks belong to a timeline (see
-- TIMELINE_TRIVIA_DECK_TIMELINE); eras (TIMELINE_TRIVIA_ERA) are ordered,
-- named year-range labels within one timeline.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_TIMELINE(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    NAME VARCHAR(255) NOT NULL,
    PRIMARY KEY(ID),
    CONSTRAINT TIMELINE_TRIVIA_TIMELINE_NAME_UNIQUE UNIQUE(NAME)
);
