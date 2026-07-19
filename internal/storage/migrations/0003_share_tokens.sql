-- Saved-list deep-link sharing was previously by row id (numeric, sequential,
-- trivially enumerable). Add a random opaque token per list and look up by it,
-- so a recipient can only clone a list whose owner actually shared the token.

ALTER TABLE saved_lists ADD COLUMN share_token TEXT NOT NULL DEFAULT '';

-- Backfill: every existing row gets a token. gen_random_uuid() is built in
-- (PostgreSQL 13+), no extension needed; stripping the dashes yields 32 hex
-- chars ≈ 128 bits of entropy. On a fresh deploy this touches zero rows.
UPDATE saved_lists
    SET share_token = replace(gen_random_uuid()::text, '-', '')
    WHERE share_token = '';

-- Unique index excluding the empty-string default (defensive — after the
-- backfill above, no row should be empty).
CREATE UNIQUE INDEX idx_saved_lists_share_token
    ON saved_lists(share_token)
    WHERE share_token <> '';
