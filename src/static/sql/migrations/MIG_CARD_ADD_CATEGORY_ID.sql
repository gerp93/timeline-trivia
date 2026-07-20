-- Additive upgrade for databases created before cards had categories. On a
-- fresh database CARD.sql already includes CATEGORY_ID, so this is a no-op;
-- on an existing database it adds the column. Single idempotent statement
-- (the schema runner executes one statement per file, no multiStatements).
ALTER TABLE CARD ADD COLUMN IF NOT EXISTS CATEGORY_ID UUID NULL;
