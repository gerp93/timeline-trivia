-- Additive upgrade for databases created before wins were tied to a
-- timeline. On a fresh database TIMELINE_TRIVIA_LOG_WIN.sql already includes
-- TIMELINE_TRIVIA_TIMELINE_ID, so this is a no-op; on an existing database it
-- adds the column, backfilling every existing win to the seeded "Real Life"
-- timeline (database.DefaultTimelineId) — accurate, not a guess, since Real
-- Life was the only timeline that existed before this feature. No row is
-- deleted or otherwise altered. Single idempotent statement (the schema
-- runner executes one statement per file, no multiStatements).
ALTER TABLE TIMELINE_TRIVIA_LOG_WIN ADD COLUMN IF NOT EXISTS TIMELINE_TRIVIA_TIMELINE_ID UUID NOT NULL DEFAULT '9f9a1a00-d22a-11f0-b4d2-60cf84649547';
