-- name: PutProgramCache :exec
INSERT INTO program_cache (url, data, updated_at)
VALUES ($1, $2, FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
ON CONFLICT (url) DO UPDATE SET
    data = excluded.data,
    updated_at = FLOOR(EXTRACT(EPOCH FROM now()))::bigint;

-- name: GetProgramCache :one
SELECT data, updated_at FROM program_cache WHERE url = $1;

-- name: PutApplicantCache :exec
INSERT INTO applicant_cache (name, data, updated_at)
VALUES ($1, $2, FLOOR(EXTRACT(EPOCH FROM now()))::bigint)
ON CONFLICT (name) DO UPDATE SET
    data = excluded.data,
    updated_at = FLOOR(EXTRACT(EPOCH FROM now()))::bigint;

-- name: GetApplicantCache :one
SELECT data, updated_at FROM applicant_cache WHERE name = $1;

-- name: VacuumProgramCache :exec
DELETE FROM program_cache WHERE updated_at < $1;

-- name: VacuumApplicantCache :exec
DELETE FROM applicant_cache WHERE updated_at < $1;
