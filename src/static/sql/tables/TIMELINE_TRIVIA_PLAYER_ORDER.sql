-- The randomized turn order for one game. Rewritten every time the game is
-- started, so play order changes between games in the same lobby. Players who
-- join mid-game have no row and fall back to PLAYER.JOIN_ORDER at the end of
-- the rotation (see GetTimelineTriviaPlayers).
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_PLAYER_ORDER(
    ID UUID NOT NULL DEFAULT UUID(),
    TIMELINE_TRIVIA_GAME_ID UUID NOT NULL,
    PLAYER_ID UUID NOT NULL,
    TURN_ORDER INT NOT NULL,
    PRIMARY KEY(ID),
    FOREIGN KEY(TIMELINE_TRIVIA_GAME_ID) REFERENCES TIMELINE_TRIVIA_GAME(ID) ON DELETE CASCADE,
    FOREIGN KEY(PLAYER_ID) REFERENCES PLAYER(ID) ON DELETE CASCADE,
    CONSTRAINT GAME_PLAYER_UNIQUE UNIQUE(TIMELINE_TRIVIA_GAME_ID, PLAYER_ID)
);
