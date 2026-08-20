-- name: InsertRun :one
INSERT INTO runs (
  at, repo, ref, digest, exit_code, verdict,
  num_comments, num_errors, num_advisories, num_skipped,
  tool_version, schema_version
) VALUES (
  sqlc.arg(at), sqlc.arg(repo), sqlc.narg(ref), sqlc.arg(digest), sqlc.arg(exit_code), sqlc.narg(verdict),
  sqlc.narg(num_comments), sqlc.narg(num_errors), sqlc.narg(num_advisories), sqlc.narg(num_skipped),
  sqlc.arg(tool_version), sqlc.arg(schema_version)
)
RETURNING id, at, repo, ref, digest, exit_code, verdict,
  num_comments, num_errors, num_advisories, num_skipped,
  tool_version, schema_version;

-- name: ListReviews :many
-- Newest first, limited. LIMIT accepts -1 for "no limit" (config.md
-- section 6, --limit=0).
SELECT * FROM runs
WHERE repo = sqlc.arg(repo)
  AND exit_code = 0
  AND (sqlc.narg(ref) IS NULL OR ref = sqlc.narg(ref))
ORDER BY at DESC
LIMIT sqlc.arg(limit);

-- name: CountReviews :one
-- Rows matching ListReviews before its LIMIT, so a caller can tell a
-- truncated answer from a complete one (config.md section 6.1).
SELECT count(*) FROM runs
WHERE repo = sqlc.arg(repo)
  AND exit_code = 0
  AND (sqlc.narg(ref) IS NULL OR ref = sqlc.narg(ref));

-- name: ListFailedRuns :many
-- The other half of the log: runs that stored no review (--failed).
SELECT * FROM runs
WHERE repo = sqlc.arg(repo)
  AND exit_code != 0
  AND (sqlc.narg(ref) IS NULL OR ref = sqlc.narg(ref))
ORDER BY at DESC
LIMIT sqlc.arg(limit);

-- name: CountFailedRuns :one
-- Rows matching ListFailedRuns before its LIMIT.
SELECT count(*) FROM runs
WHERE repo = sqlc.arg(repo)
  AND exit_code != 0
  AND (sqlc.narg(ref) IS NULL OR ref = sqlc.narg(ref));

-- name: ListRepoCounts :many
-- Per-repository counts of stored reviews and failed runs, for --list.
-- Ordered by repo so the output is stable between runs.
SELECT
  repo,
  sum(CASE WHEN exit_code = 0 THEN 1 ELSE 0 END) AS reviews,
  sum(CASE WHEN exit_code != 0 THEN 1 ELSE 0 END) AS failed
FROM runs
GROUP BY repo
ORDER BY repo;

-- name: RepoKnown :one
-- Whether the store has any row at all for repo, regardless of exit code.
-- Distinguishes a mistyped --repo from one that is known but empty
-- (config.md section 6.2).
SELECT EXISTS(SELECT 1 FROM runs WHERE repo = sqlc.arg(repo)) AS known;
