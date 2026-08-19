-- Replacement for the dropped global NAME_UNIQUE (see
-- MIG_TIMELINE_TRIVIA_CATEGORY_DROP_NAME_UNIQUE): a category name only
-- needs to be unique within its own timeline. Must run after that drop and
-- after MIG_TIMELINE_TRIVIA_CATEGORY_ADD_TIMELINE_ID (the column has to
-- exist first). On a fresh database TIMELINE_TRIVIA_CATEGORY.sql already
-- defines this exact constraint, so this is a no-op there. Single
-- idempotent statement (the schema runner executes one statement per file,
-- no multiStatements).
ALTER TABLE TIMELINE_TRIVIA_CATEGORY ADD UNIQUE INDEX IF NOT EXISTS TIMELINE_TRIVIA_CATEGORY_NAME_UNIQUE(TIMELINE_TRIVIA_TIMELINE_ID, NAME);
