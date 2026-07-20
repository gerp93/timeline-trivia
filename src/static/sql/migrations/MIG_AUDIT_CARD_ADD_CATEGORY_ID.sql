-- Additive upgrade: keep AUDIT_CARD in step with CARD's CATEGORY_ID column so
-- the card audit triggers (which snapshot CATEGORY_ID) work on databases
-- created before categories existed. No-op on a fresh database.
ALTER TABLE AUDIT_CARD ADD COLUMN IF NOT EXISTS CATEGORY_ID UUID NULL;
