-- Raises the cards-to-win default from 5 to 10. On a fresh database
-- TIMELINE_TRIVIA_GAME.sql already declares the new default, so this is a
-- no-op; on an existing database CREATE TABLE IF NOT EXISTS never runs, so
-- the default has to be altered explicitly. Single idempotent statement (the
-- schema runner executes one statement per file, no multiStatements).
ALTER TABLE TIMELINE_TRIVIA_GAME ALTER COLUMN CARDS_TO_WIN SET DEFAULT 10;
