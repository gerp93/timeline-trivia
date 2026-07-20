-- Predefined, admin-managed list of card categories. Every card is placed
-- into one of these (enforced in the Go layer, not a DB FK, so both fresh and
-- upgraded databases converge to the same shape). Seeded from the distinct
-- categories in the default deck's import JSON when this table is empty.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_CATEGORY(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    NAME VARCHAR(255) NOT NULL,
    PRIMARY KEY(ID),
    CONSTRAINT NAME_UNIQUE UNIQUE(NAME)
);
