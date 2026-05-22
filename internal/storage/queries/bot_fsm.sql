-- name: GetFSM :one
SELECT state, data FROM bot_fsm WHERE tg_id = ?;

-- name: SetFSM :exec
INSERT INTO bot_fsm (tg_id, state, data, updated_at)
VALUES (?1, ?2, ?3, unixepoch())
ON CONFLICT(tg_id) DO UPDATE SET
    state = excluded.state,
    data = excluded.data,
    updated_at = unixepoch();

-- name: ClearFSM :exec
DELETE FROM bot_fsm WHERE tg_id = ?;
