-- name: UpsertUser :exec
-- Creates the user row if missing; safe to call on every interaction.
INSERT INTO users (tg_id) VALUES (?)
ON CONFLICT(tg_id) DO NOTHING;

-- name: GetUser :one
SELECT * FROM users WHERE tg_id = ?;

-- name: ListUserIDs :many
SELECT tg_id FROM users ORDER BY tg_id;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: IncrementActivates :exec
-- Race-safe counter increment (the Python version had a read-modify-write race).
INSERT INTO users (tg_id, activates, updated_at)
VALUES (?1, 1, unixepoch())
ON CONFLICT(tg_id) DO UPDATE SET
    activates = activates + 1,
    updated_at = unixepoch();

-- name: AddActivates :exec
-- Adds a batched delta accumulated in memory (the hot path buffers the
-- per-update +1s and a periodic flush applies them in one write, so the
-- single SQLite connection isn't hit on every update).
INSERT INTO users (tg_id, activates, updated_at)
VALUES (?1, ?2, unixepoch())
ON CONFLICT(tg_id) DO UPDATE SET
    activates = activates + excluded.activates,
    updated_at = unixepoch();

-- name: IncrementRightActivates :exec
INSERT INTO users (tg_id, right_activates, updated_at)
VALUES (?1, 1, unixepoch())
ON CONFLICT(tg_id) DO UPDATE SET
    right_activates = right_activates + 1,
    updated_at = unixepoch();

-- name: TotalActivates :one
-- CASTs needed because COALESCE(SUM(...), 0) returns an unbound NUMERIC in
-- SQLite, which sqlc maps to interface{}. The CAST forces INTEGER.
SELECT
    CAST(COALESCE(SUM(activates), 0)       AS INTEGER) AS total_activates,
    CAST(COALESCE(SUM(right_activates), 0) AS INTEGER) AS total_right_activates
FROM users;

-- name: TopUserByActivates :one
SELECT tg_id, activates FROM users
ORDER BY activates DESC, tg_id ASC
LIMIT 1;

-- name: SetUserNMT :exec
INSERT INTO users (tg_id, nmt_scores, updated_at)
VALUES (?1, ?2, unixepoch())
ON CONFLICT(tg_id) DO UPDATE SET
    nmt_scores = excluded.nmt_scores,
    updated_at = unixepoch();

-- name: GetUserNMT :one
SELECT nmt_scores FROM users WHERE tg_id = ?;

-- name: SetUserSettings :exec
INSERT INTO users (tg_id, settings, updated_at)
VALUES (?1, ?2, unixepoch())
ON CONFLICT(tg_id) DO UPDATE SET
    settings = excluded.settings,
    updated_at = unixepoch();

-- name: GetUserSettings :one
SELECT settings FROM users WHERE tg_id = ?;
