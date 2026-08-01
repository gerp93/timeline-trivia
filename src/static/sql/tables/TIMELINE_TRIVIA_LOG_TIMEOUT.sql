-- Append-only log of every turn lost to the turn timer running out. Same
-- no-foreign-keys reasoning as TIMELINE_TRIVIA_LOG_GUESS: this outlives the
-- game/lobby/player rows that cascade away on disconnect. Deliberately NOT
-- folded into TIMELINE_TRIVIA_LOG_GUESS — a timeout is not a guess, and
-- counting it as a wrong one would silently deflate every accuracy figure.
-- TIMER_SECONDS is snapshotted because a lobby's timer can be changed
-- mid-game, so the setting in force at the moment of the timeout is the only
-- one that explains it.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_LOG_TIMEOUT(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    USER_ID UUID NOT NULL,
    CARD_ID UUID NOT NULL,
    TIMER_SECONDS INT NOT NULL,
    PRIMARY KEY(ID)
);
