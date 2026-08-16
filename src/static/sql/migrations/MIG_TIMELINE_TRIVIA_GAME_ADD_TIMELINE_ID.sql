-- Additive upgrade for databases created before games were tied to a
-- timeline. On a fresh database TIMELINE_TRIVIA_GAME.sql already includes
-- TIMELINE_TRIVIA_TIMELINE_ID, so this is a no-op; on an existing database
-- it adds the column, backfilling every existing game to the seeded "Real
-- Life" timeline (database.DefaultTimelineId). Single idempotent statement
-- (the schema runner executes one statement per file, no multiStatements).
ALTER TABLE TIMELINE_TRIVIA_GAME ADD COLUMN IF NOT EXISTS TIMELINE_TRIVIA_TIMELINE_ID UUID NOT NULL DEFAULT '9f9a1a00-d22a-11f0-b4d2-60cf84649547';
