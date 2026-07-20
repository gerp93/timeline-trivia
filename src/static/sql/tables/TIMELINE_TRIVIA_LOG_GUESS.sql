-- Append-only log of every card-placement guess. Deliberately has NO foreign
-- keys: game/lobby/player rows cascade away when a lobby's last websocket
-- client disconnects, but stats must survive that, so this outlives them and
-- is joined back to CARD by CARD_ID at query time (a deleted card simply drops
-- out of stats). CARD_YEAR is snapshotted (it's the answer for that guess, so
-- decade breakdowns stay stable even if the card is later edited).
CREATE TABLE IF NOT EXISTS TIMELINE_TRIVIA_LOG_GUESS(
    ID UUID NOT NULL DEFAULT UUID(),
    CREATED_ON_DATE DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP(),
    USER_ID UUID NOT NULL,
    CARD_ID UUID NOT NULL,
    CARD_YEAR INT NOT NULL,
    IS_CORRECT BOOLEAN NOT NULL,
    PRIMARY KEY(ID)
);
