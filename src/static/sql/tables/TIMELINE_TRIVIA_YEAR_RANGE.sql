-- Optional per-game year-range filters; a draw-pile card is kept only if its
-- year falls within at least one of a game's ranges. No rows = no filter.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_YEAR_RANGE(
    ID UUID NOT NULL DEFAULT UUID(),
    TIMELINE_TRIVIA_GAME_ID UUID NOT NULL,
    FROM_YEAR INT NOT NULL,
    TO_YEAR INT NOT NULL,
    PRIMARY KEY(ID),
    FOREIGN KEY(TIMELINE_TRIVIA_GAME_ID) REFERENCES TIMELINE_TRIVIA_GAME(ID) ON DELETE CASCADE
);
