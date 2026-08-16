-- Append-only log of games won, one row per win. No foreign keys (see
-- LOG_GUESS): survives lobby deletion so per-user games-won counts persist.
-- TIMELINE_TRIVIA_TIMELINE_ID is a soft reference (see CARD.CATEGORY_ID for
-- the same rationale) snapshotted at win time — unlike LOG_GUESS/LOG_CARD/
-- LOG_TIMEOUT, this table has no CARD_ID to derive a timeline from later via
-- join, so it has to be captured directly.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_LOG_WIN(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    USER_ID UUID NOT NULL,
    TIMELINE_TRIVIA_TIMELINE_ID UUID NOT NULL DEFAULT '9f9a1a00-d22a-11f0-b4d2-60cf84649547',
    PRIMARY KEY(ID)
);
