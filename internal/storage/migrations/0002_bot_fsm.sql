-- Bot FSM persistence: one row per user holds the current conversation
-- state name and an opaque JSON blob with state-specific data
-- (e.g. {"current_subject":"Математика","url":"https://..."}).
--
-- Storing FSM in the database (instead of telebot's in-memory map) means
-- conversations survive bot restarts — a user mid-way through entering
-- НМТ scores doesn't lose progress when we redeploy.

CREATE TABLE bot_fsm (
    tg_id      BIGINT PRIMARY KEY REFERENCES users(tg_id) ON DELETE CASCADE,
    state      TEXT   NOT NULL DEFAULT '',  -- empty = no active conversation
    data       TEXT   NOT NULL DEFAULT '{}',
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
);
