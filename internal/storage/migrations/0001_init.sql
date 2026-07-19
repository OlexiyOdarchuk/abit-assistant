-- AbitAssistant initial schema (PostgreSQL).
--
-- Notes:
--   * tg_id is the natural primary key.
--   * Timestamps are Unix-epoch BIGINT (seconds), not timestamptz — the Go
--     code stores int64 and compares against time.Now().Unix(), so we keep
--     the same representation the SQLite schema used.
--   * JSON blobs stay TEXT (the app marshals/unmarshals them itself).

CREATE TABLE users (
    tg_id           BIGINT PRIMARY KEY,
    nmt_scores      TEXT   NOT NULL DEFAULT '{}',  -- JSON: {"Українська мова": 170, ...}
    settings        TEXT   NOT NULL DEFAULT '{}',  -- JSON: UserSettings struct
    activates       BIGINT NOT NULL DEFAULT 0,     -- total /start invocations
    right_activates BIGINT NOT NULL DEFAULT 0,     -- successful flows completed
    created_at      BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM now()))::bigint),
    updated_at      BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
);

CREATE TABLE saved_lists (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_tg_id  BIGINT NOT NULL REFERENCES users(tg_id) ON DELETE CASCADE,
    name        TEXT   NOT NULL,
    url         TEXT   NOT NULL,
    data        TEXT   NOT NULL,  -- JSON snapshot of decoded program
    created_at  BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
);

CREATE INDEX idx_saved_lists_user ON saved_lists(user_tg_id, created_at DESC);

CREATE TABLE program_cache (
    url        TEXT   PRIMARY KEY,
    data       TEXT   NOT NULL,  -- JSON abit.Program
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
);

CREATE TABLE applicant_cache (
    name       TEXT   PRIMARY KEY,
    data       TEXT   NOT NULL,  -- JSON []abit.ApplicantEntry
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
);
