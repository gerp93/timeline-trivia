CREATE TABLE IF NOT EXISTS CARD(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    CHANGED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    DECK_ID UUID NOT NULL,
    TEXT VARCHAR(510) NOT NULL,
    CARD_YEAR INT NULL,
    -- Category is a soft reference into TIMELINE_TRIVIA_CATEGORY; integrity is
    -- enforced in the Go layer (required on create/edit, reassigned before a
    -- category is deleted) rather than a DB FK, so the additive upgrade path
    -- (ALTER ... ADD COLUMN IF NOT EXISTS) stays a single idempotent statement.
    CATEGORY_ID UUID NULL,
    PRIMARY KEY(ID),
    FOREIGN KEY(DECK_ID) REFERENCES DECK(ID) ON DELETE CASCADE,
    CONSTRAINT DECK_TEXT_UNIQUE UNIQUE(DECK_ID, TEXT)
);
