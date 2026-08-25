-- Additive upgrade for databases created before the per-turn timer was
-- anchored to a server-side clock. On a fresh database TIMELINE_TRIVIA_GAME.sql
-- already includes CURRENT_TURN_STARTED_ON_DATE, so this is a no-op; on an
-- existing database it adds the column, leaving existing in-progress turns
-- with a NULL start (treated as "no server timer truth yet" until the next
-- turn change stamps it). Single idempotent statement (the schema runner
-- executes one statement per file, no multiStatements).
ALTER TABLE TIMELINE_TRIVIA_GAME ADD COLUMN IF NOT EXISTS CURRENT_TURN_STARTED_ON_DATE DATETIME NULL;
