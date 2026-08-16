-- Predefined, admin-managed list of card categories, scoped to one timeline
-- (a category belongs to exactly one timeline, just like an era). Every
-- card is placed into one of its own timeline's categories (enforced in
-- the Go layer, not a DB FK, so both fresh and upgraded databases converge
-- to the same shape). Seeded from the distinct categories in the default
-- deck's import JSON, under the default "Real Life" timeline, when this
-- table is empty.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_CATEGORY(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    TIMELINE_TRIVIA_TIMELINE_ID UUID NOT NULL DEFAULT '9f9a1a00-d22a-11f0-b4d2-60cf84649547',
    NAME VARCHAR(255) NOT NULL,
    PRIMARY KEY(ID),
    CONSTRAINT TIMELINE_TRIVIA_CATEGORY_NAME_UNIQUE UNIQUE(TIMELINE_TRIVIA_TIMELINE_ID, NAME)
);
