-- Cards a player flagged mid-game as unusable ("skip and remove"). A flagged
-- card is in purgatory: it is excluded from every draw pile until an admin
-- reviews it on /flagged-cards and either accepts it back, edits and accepts
-- it, or deletes it outright. LOBBY_ID is nullable because lobbies are deleted
-- when their last client disconnects, and the flag has to outlive that.
CREATE TABLE IF NOT EXISTS CARD_FLAGGED(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    CARD_ID UUID NOT NULL,
    FLAGGED_BY_USER_ID UUID NULL,
    LOBBY_ID UUID NULL,
    REASON VARCHAR(255) NULL,
    PRIMARY KEY(ID),
    FOREIGN KEY(CARD_ID) REFERENCES CARD(ID) ON DELETE CASCADE,
    FOREIGN KEY(FLAGGED_BY_USER_ID) REFERENCES USER(ID) ON DELETE SET NULL,
    FOREIGN KEY(LOBBY_ID) REFERENCES LOBBY(ID) ON DELETE SET NULL,
    CONSTRAINT CARD_UNIQUE UNIQUE(CARD_ID)
);
