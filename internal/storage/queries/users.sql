-- name: UpsertUser :exec
-- Creates the user row if missing; safe to call on every interaction.
INSERT INTO users (tg_id) VALUES ($1)
ON CONFLICT (tg_id) DO NOTHING;

-- name: GetUser :one
SELECT * FROM users WHERE tg_id = $1;

-- name: ListUserIDs :many
SELECT tg_id FROM users ORDER BY tg_id;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: IncrementActivates :exec
-- Race-safe counter increment (the Python version had a read-modify-write race).
INSERT INTO users (tg_id, activates, updated_at)
VALUES ($1, 1, FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
ON CONFLICT (tg_id) DO UPDATE SET
    activates = users.activates + 1,
    updated_at = FLOOR(EXTRACT(EPOCH FROM now()))::bigint;

-- name: AddActivates :exec
-- Adds a batched delta accumulated in memory (the hot path buffers the
-- per-update +1s and a periodic flush applies them in one write).
INSERT INTO users (tg_id, activates, updated_at)
VALUES ($1, $2, FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
ON CONFLICT (tg_id) DO UPDATE SET
    activates = users.activates + excluded.activates,
    updated_at = FLOOR(EXTRACT(EPOCH FROM now()))::bigint;

-- name: IncrementRightActivates :exec
INSERT INTO users (tg_id, right_activates, updated_at)
VALUES ($1, 1, FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
ON CONFLICT (tg_id) DO UPDATE SET
    right_activates = users.right_activates + 1,
    updated_at = FLOOR(EXTRACT(EPOCH FROM now()))::bigint;

-- name: TotalActivates :one
SELECT
    COALESCE(SUM(activates), 0)::bigint       AS total_activates,
    COALESCE(SUM(right_activates), 0)::bigint AS total_right_activates
FROM users;

-- name: TopUserByActivates :one
SELECT tg_id, activates FROM users
ORDER BY activates DESC, tg_id ASC
LIMIT 1;

-- name: SetUserNMT :exec
INSERT INTO users (tg_id, nmt_scores, updated_at)
VALUES ($1, $2, FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
ON CONFLICT (tg_id) DO UPDATE SET
    nmt_scores = excluded.nmt_scores,
    updated_at = FLOOR(EXTRACT(EPOCH FROM now()))::bigint;

-- name: GetUserNMT :one
SELECT nmt_scores FROM users WHERE tg_id = $1;

-- name: SetUserSettings :exec
INSERT INTO users (tg_id, settings, updated_at)
VALUES ($1, $2, FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
ON CONFLICT (tg_id) DO UPDATE SET
    settings = excluded.settings,
    updated_at = FLOOR(EXTRACT(EPOCH FROM now()))::bigint;

-- name: GetUserSettings :one
SELECT settings FROM users WHERE tg_id = $1;
