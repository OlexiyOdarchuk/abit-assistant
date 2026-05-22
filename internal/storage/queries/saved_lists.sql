-- name: SaveList :one
INSERT INTO saved_lists (user_tg_id, name, url, data, share_token)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: GetSavedListByToken :one
SELECT * FROM saved_lists WHERE share_token = ?;

-- name: ListSavedLists :many
SELECT * FROM saved_lists
WHERE user_tg_id = ?
ORDER BY created_at DESC;

-- name: GetSavedList :one
SELECT * FROM saved_lists WHERE id = ?;

-- name: UpdateSavedListData :exec
UPDATE saved_lists SET data = ?2 WHERE id = ?1;

-- name: DeleteSavedList :exec
DELETE FROM saved_lists WHERE id = ?;
