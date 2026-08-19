-- An era is a named, ordered year-range label within a timeline (e.g.
-- "Before Common Era"/"B.C.E" and "Common Era" for Real Life, or
-- "Before Battle of Yavin"/"BBY" and "After Battle of Yavin"/"ABY" for a
-- Star Wars timeline). Purely a display + admin-organization concern —
-- CARD_YEAR remains a single ordered integer scale per timeline regardless
-- of era boundaries, so gameplay comparisons never need to consult this
-- table. FROM_YEAR/TO_YEAR are nullable to mean an open-ended bound (the
-- earliest/latest era in a timeline usually leaves one side open).
--
-- EPOCH_OFFSET + DIRECTION let an era represent either half of a
-- split-epoch convention (B.C.E/C.E, BBY/ABY — both offset 0, opposite
-- directions) or one era in a sequence that resets its own counter at each
-- boundary (Tolkien's Ages, Elder Scrolls' 1E/2E/3E/4E — all FORWARD, each
-- with the prior era's ending absolute year as its offset). See
-- database.FormatYearInEras / database.AbsoluteYearFromEra.
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_ERA(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    TIMELINE_TRIVIA_TIMELINE_ID UUID NOT NULL,
    NAME VARCHAR(255) NOT NULL,
    ABBREVIATION VARCHAR(50) NOT NULL DEFAULT '',
    SORT_ORDER INT NOT NULL,
    FROM_YEAR INT NULL,
    TO_YEAR INT NULL,
    EPOCH_OFFSET INT NOT NULL DEFAULT 0,
    DIRECTION ENUM('FORWARD', 'BACKWARD') NOT NULL DEFAULT 'FORWARD',
    PRIMARY KEY(ID),
    FOREIGN KEY(TIMELINE_TRIVIA_TIMELINE_ID) REFERENCES TIMELINE_TRIVIA_TIMELINE(ID) ON DELETE CASCADE
);
