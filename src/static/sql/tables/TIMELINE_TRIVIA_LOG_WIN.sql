-- Append-only log of games won, one row per win. No foreign keys (see
-- LOG_GUESS): survives lobby deletion so per-user games-won counts persist.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_LOG_WIN(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    USER_ID UUID NOT NULL,
    PRIMARY KEY(ID)
);
