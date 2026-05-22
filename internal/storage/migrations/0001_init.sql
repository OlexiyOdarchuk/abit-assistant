-- AbitAssistant initial schema.
--
-- Departures from the Python original:
--   * tg_id is the natural primary key (the Python schema kept a
--     surrogate `id` plus a UNIQUE on tg_id — that's redundant).
--   * Timestamps are Unix epoch INTEGER, not stringy DATETIME — easier
--     to compare against time.Now().Unix() in Go, no timezone games.
--   * `updated_at` columns exist on users too (Python only tracked
--     created_at), so we can detect inactive accounts later.
--   * STRICT tables enforce declared column types (SQLite >= 3.37).
--   * Renamed url_cache → program_cache and competitor_cache →
--     applicant_cache so the names match the domain types
--     (abit.Program and abit.ApplicantEntry).

CREATE TABLE users (
    tg_id           INTEGER PRIMARY KEY,
    nmt_scores      TEXT    NOT NULL DEFAULT '{}',  -- JSON: {"Українська мова": 170, ...}
    settings        TEXT    NOT NULL DEFAULT '{}',  -- JSON: UserSettings struct
    activates       INTEGER NOT NULL DEFAULT 0,     -- total /start invocations
    right_activates INTEGER NOT NULL DEFAULT 0,     -- successful flows completed
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE TABLE saved_lists (
    id          INTEGER PRIMARY KEY,
    user_tg_id  INTEGER NOT NULL REFERENCES users(tg_id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    url         TEXT    NOT NULL,
    data        TEXT    NOT NULL,  -- JSON snapshot of decoded program
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX idx_saved_lists_user ON saved_lists(user_tg_id, created_at DESC);

CREATE TABLE program_cache (
    url        TEXT    PRIMARY KEY,
    data       TEXT    NOT NULL,  -- JSON abit.Program
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE TABLE applicant_cache (
    name       TEXT    PRIMARY KEY,
    data       TEXT    NOT NULL,  -- JSON []abit.ApplicantEntry
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;
