-- name: SaveList :one
INSERT INTO saved_lists (user_tg_id, name, url, data, share_token)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetSavedListByToken :one
SELECT * FROM saved_lists WHERE share_token = $1;

-- name: ListSavedLists :many
SELECT * FROM saved_lists
WHERE user_tg_id = $1
ORDER BY created_at DESC;

-- name: GetSavedList :one
SELECT * FROM saved_lists WHERE id = $1;

-- name: UpdateSavedListData :exec
UPDATE saved_lists SET data = $2 WHERE id = $1;

-- name: DeleteSavedList :exec
DELETE FROM saved_lists WHERE id = $1;
