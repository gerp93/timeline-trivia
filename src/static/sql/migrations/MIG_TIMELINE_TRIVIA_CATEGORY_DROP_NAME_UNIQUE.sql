-- Categories used to be globally unique by name; now that they're scoped to
-- a timeline (see MIG_TIMELINE_TRIVIA_CATEGORY_ADD_TIMELINE_ID), two
-- different timelines must be able to each have their own "History"
-- category. Drops the old global constraint so
-- MIG_TIMELINE_TRIVIA_CATEGORY_ADD_TIMELINE_NAME_UNIQUE can add the
-- replacement composite one. No-op on a fresh database, which never had
-- NAME_UNIQUE. Single idempotent statement (the schema runner executes one
-- statement per file, no multiStatements).
ALTER TABLE TIMELINE_TRIVIA_CATEGORY DROP INDEX IF EXISTS NAME_UNIQUE;
