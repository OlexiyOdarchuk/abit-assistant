-- name: PutProgramCache :exec
INSERT INTO program_cache (url, data, updated_at)
VALUES (?1, ?2, unixepoch())
ON CONFLICT(url) DO UPDATE SET
    data = excluded.data,
    updated_at = unixepoch();

-- name: GetProgramCache :one
SELECT data, updated_at FROM program_cache WHERE url = ?;

-- name: PutApplicantCache :exec
INSERT INTO applicant_cache (name, data, updated_at)
VALUES (?1, ?2, unixepoch())
ON CONFLICT(name) DO UPDATE SET
    data = excluded.data,
    updated_at = unixepoch();

-- name: GetApplicantCache :one
SELECT data, updated_at FROM applicant_cache WHERE name = ?;

-- name: VacuumProgramCache :exec
DELETE FROM program_cache WHERE updated_at < ?;

-- name: VacuumApplicantCache :exec
DELETE FROM applicant_cache WHERE updated_at < ?;
