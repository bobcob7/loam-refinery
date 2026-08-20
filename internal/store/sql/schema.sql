CREATE TABLE runs (
  id             INTEGER PRIMARY KEY,
  at             TEXT    NOT NULL,  -- RFC 3339 UTC
  repo           TEXT    NOT NULL,  -- normalized name, §4.2
  ref            TEXT,              -- 40 hex; NULL when the input never parsed
  digest         TEXT    NOT NULL,  -- sha256 of the submitted bytes
  exit_code      INTEGER NOT NULL,
  verdict        TEXT             CHECK (verdict IN ('approve', 'request_changes', 'comment')),
  num_comments   INTEGER,
  num_errors     INTEGER,
  num_advisories INTEGER,
  num_skipped    INTEGER,
  tool_version   TEXT    NOT NULL,
  schema_version TEXT    NOT NULL
) STRICT;

CREATE INDEX runs_repo_ref ON runs(repo, ref);
CREATE INDEX runs_repo_at  ON runs(repo, at DESC);
CREATE INDEX runs_digest   ON runs(digest);
