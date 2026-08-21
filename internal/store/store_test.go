package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_SchemaConstraints(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	t.Run("rejects a STRICT type violation", func(t *testing.T) {
		t.Parallel()
		_, err := db.ExecContext(t.Context(),
			"INSERT INTO runs (at, repo, digest, exit_code, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?)",
			"2026-08-19T00:00:00Z", "repo", "digest-strict", "not-an-int", "v1", "v1")
		assert.Error(t, err)
	})

	t.Run("rejects an invalid verdict", func(t *testing.T) {
		t.Parallel()
		_, err := db.ExecContext(t.Context(),
			"INSERT INTO runs (at, repo, digest, exit_code, verdict, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"2026-08-19T00:00:00Z", "repo", "digest-verdict", 0, "bogus", "v1", "v1")
		assert.Error(t, err)
	})

	t.Run("accepts a NULL verdict", func(t *testing.T) {
		t.Parallel()
		_, err := db.ExecContext(t.Context(),
			"INSERT INTO runs (at, repo, digest, exit_code, verdict, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"2026-08-19T00:00:00Z", "repo", "digest-null-verdict", 0, nil, "v1", "v1")
		assert.NoError(t, err)
	})

	t.Run("rejects an invalid assessment", func(t *testing.T) {
		t.Parallel()
		_, err := db.ExecContext(t.Context(),
			"INSERT INTO runs (at, repo, digest, exit_code, assessment, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"2026-08-19T00:00:00Z", "repo", "digest-assessment", 0, "bogus", "v1", "v1")
		assert.Error(t, err)
	})

	t.Run("accepts a NULL assessment", func(t *testing.T) {
		t.Parallel()
		_, err := db.ExecContext(t.Context(),
			"INSERT INTO runs (at, repo, digest, exit_code, assessment, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"2026-08-19T00:00:00Z", "repo", "digest-null-assessment", 0, nil, "v1", "v1")
		assert.NoError(t, err)
	})

	t.Run("accepts an unrecognized exit code without a migration", func(t *testing.T) {
		t.Parallel()
		_, err := db.ExecContext(t.Context(),
			"INSERT INTO runs (at, repo, digest, exit_code, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?)",
			"2026-08-19T00:00:00Z", "repo", "digest-exit-code", 102, "v1", "v1")
		assert.NoError(t, err)
	})
}

func TestOpen_StampsUserVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	defer db.Close()
	var version int
	require.NoError(t, db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version))
	assert.Equal(t, schemaVersion, version)
}

func TestExists_ReportsFalseWithoutCreatingAnything(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "nested", "store")
	exists, err := Exists(dir)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.NoDirExists(t, dir)
}

func TestExists_ReportsTrueOnceTheDatabaseIsThere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	exists, err := Exists(dir)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestOpen_ConcurrentFirstUseAllSucceed proves refinery-a96.28 and
// refinery-a96.29: several goroutines racing Open against a path with
// nothing there yet all succeed. Before the fix, this failed two ways —
// most goroutines got "database is locked (5) (SQLITE_BUSY)" converting to
// WAL (a96.28, since PRAGMA journal_mode=WAL does not honor the busy
// handler busy_timeout installs), and any that got past that could still
// collide inside migrate on "table runs already exists" (a96.29, since the
// version check and the schema application were not one transaction). Ten
// trials of eight goroutines each is enough to make either regression
// reappear reliably; it was 52/60 and 10/240 respectively against the code
// this test was written against.
func TestOpen_ConcurrentFirstUseAllSucceed(t *testing.T) {
	t.Parallel()
	const goroutines = 8
	const trials = 10
	for trial := 0; trial < trials; trial++ {
		dir := t.TempDir()
		path := filepath.Join(dir, "store.db")
		var wg sync.WaitGroup
		errs := make([]error, goroutines)
		for i := range goroutines {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				db, err := Open(t.Context(), path)
				if err != nil {
					errs[i] = err
					return
				}
				defer db.Close()
			}(i)
		}
		wg.Wait()
		for i, err := range errs {
			assert.NoErrorf(t, err, "trial %d, goroutine %d", trial, i)
		}
	}
}

// TestOpen_JournalModeIsWALAndBusyTimeoutIsSet names docs/config.md §4.6's
// two concrete claims — "opened in WAL mode with a busy timeout" — and
// asserts both rather than trusting the DSN string that sets them. A typo
// in a pragma name inside that string is not a compile error and would
// otherwise only be caught by a person running PRAGMA journal_mode by hand
// (refinery-a96.24).
func TestOpen_JournalModeIsWALAndBusyTimeoutIsSet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	defer db.Close()
	var journalMode string
	require.NoError(t, db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journalMode))
	assert.Equal(t, "wal", journalMode, "docs/config.md §4.6: \"opened in WAL mode\"")
	var busyTimeout int
	require.NoError(t, db.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&busyTimeout))
	assert.Equal(t, busyTimeoutMillis, busyTimeout, "docs/config.md §4.6: \"opened ... with a busy timeout\"")
}

// TestBusyTimeoutExpiry_IsSQLiteBusy is the decision docs/config.md §4.6
// and refinery-a96.24 ask for on the harder half of that section: whether
// the busy-timeout-expiry-to-101 path gets a test. It does, at the
// mechanism level — a second connection with a short busy_timeout,
// contending with a write lock the first is holding open, gets
// SQLITE_BUSY once its timeout elapses, which is the error isSQLiteBusy
// recognizes and Open's caller maps to exit 101. What is deliberately not
// tested here is Open's own five-second busy_timeout expiring: driving
// that path for real would mean either a five-second test or threading a
// configurable timeout through Open for tests alone, and the mechanism
// this test pins is what that path depends on — a connection-level DSN
// setting, unrelated to which number is in it.
func TestBusyTimeoutExpiry_IsSQLiteBusy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "store.db")
	held, err := Open(t.Context(), path)
	require.NoError(t, err)
	defer held.Close()
	tx, err := held.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(t.Context(), "INSERT INTO runs (at, repo, digest, exit_code, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?)",
		"2026-08-19T00:00:00Z", "repo", "digest-busy", 0, "v1", "v1")
	require.NoError(t, err, "an uncommitted write is what holds the lock the second connection contends on")
	contender, err := sql.Open("sqlite", storeDSN(path, "_pragma=busy_timeout(50)&_txlock=immediate"))
	require.NoError(t, err)
	defer contender.Close()
	_, err = contender.ExecContext(t.Context(), "INSERT INTO runs (at, repo, digest, exit_code, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?)",
		"2026-08-19T00:00:00Z", "repo", "digest-contender", 0, "v1", "v1")
	require.Error(t, err, "a busy timeout that expires is an error, not a silent wait")
	assert.True(t, isSQLiteBusy(err), "docs/config.md §4.6: \"a busy timeout that expires is a tool error\", err = %v", err)
}

// TestOpen_PathWithQuestionMarkStaysInsideDirectory proves refinery-a96.20:
// a store directory holding '?', '#', or '%' — all legal on every
// filesystem this tool runs on — still gets store.db inside it, rather
// than SQLite parsing everything from the first such character on as a DSN
// query string and creating store.db as a sibling file with a truncated
// name.
func TestOpen_PathWithQuestionMarkStaysInsideDirectory(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"we?ird", "sha#rp", "per%cent"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			dir := filepath.Join(base, name)
			require.NoError(t, os.MkdirAll(dir, 0o700))
			path := filepath.Join(dir, "store.db")
			db, err := Open(t.Context(), path)
			require.NoError(t, err)
			defer db.Close()
			assert.FileExists(t, path, "store.db must land inside the configured directory")
			entries, err := os.ReadDir(base)
			require.NoError(t, err)
			require.Len(t, entries, 1, "nothing must be created beside the configured directory")
			assert.Equal(t, name, entries[0].Name())
		})
	}
}

// TestOpen_RunsTableIsExactlyTheFourteenDocumentedColumns pins
// docs/config.md §4.5.1's schema: PRAGMA table_info(runs) must report
// exactly these fourteen columns, in this order, and no others. An
// unexpected extra column — the concrete regression refinery-a96.23 names,
// against the thirteen columns that predated assessment — changes nothing
// any existing test checks, since every other test here asks about one
// column or one constraint rather than the whole set.
func TestOpen_RunsTableIsExactlyTheFourteenDocumentedColumns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	rows, err := db.QueryContext(t.Context(), "PRAGMA table_info(runs)")
	require.NoError(t, err)
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		require.NoError(t, rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk))
		got = append(got, name)
	}
	require.NoError(t, rows.Err())
	want := []string{
		"id", "at", "repo", "ref", "digest", "exit_code", "verdict", "assessment",
		"num_comments", "num_errors", "num_advisories", "num_skipped",
		"tool_version", "schema_version",
	}
	assert.Equal(t, want, got, "docs/config.md §4.5.1: runs has exactly these fourteen columns, in this order")
}

// TestOpen_RunsTableHasExactlyItsThreeDocumentedIndexes pins the other half
// of docs/config.md §4.5.1: runs_repo_ref, runs_repo_at, and runs_digest,
// and no other index — a new query pattern that reaches for a fourth index
// should be a deliberate addition to the schema, not a side effect nobody
// notices.
func TestOpen_RunsTableHasExactlyItsThreeDocumentedIndexes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	rows, err := db.QueryContext(t.Context(),
		"SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'runs' ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got = append(got, name)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"runs_digest", "runs_repo_at", "runs_repo_ref"}, got,
		"docs/config.md §4.5.1: runs has exactly these three indexes")
}

// TestOpen_RunsTableIsStrict pins the third half-sentence of
// docs/config.md §4.5.1's schema: runs is created STRICT, which is what
// makes every other schema test here — the rejected type violation, the
// checked verdict — mean what it says rather than guard a column SQLite's
// ordinary type affinity had already let the wrong kind of value into.
func TestOpen_RunsTableIsStrict(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	var createSQL string
	require.NoError(t, db.QueryRowContext(t.Context(),
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'runs'").Scan(&createSQL))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(createSQL), "STRICT"),
		"docs/config.md §4.5.1: runs must be STRICT; CREATE TABLE was %q", createSQL)
}

func TestOpen_ReopeningAnExistingDatabaseSucceeds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "store.db")
	first, err := Open(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, first.Close())
	second, err := Open(t.Context(), path)
	require.NoError(t, err)
	defer second.Close()
	var version int
	require.NoError(t, second.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version))
	assert.Equal(t, schemaVersion, version)
}

// schemaVersion1 is the runs table exactly as it shipped before
// refinery-dbk.6 added assessment (schema version 1, thirteen columns, no
// assessment column or CHECK) — kept here only to build a database at that
// version, so TestMigrate_FromSchemaVersion1AddsAssessmentColumn exercises
// migrate's version-1-to-2 path against a database a previous version of
// this binary actually would have written, per config.md §4.5.4's "tested
// against a database written by the previous version".
const schemaVersion1 = `
CREATE TABLE runs (
  id             INTEGER PRIMARY KEY,
  at             TEXT    NOT NULL,
  repo           TEXT    NOT NULL,
  ref            TEXT,
  digest         TEXT    NOT NULL,
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
`

// TestMigrate_FromSchemaVersion1AddsAssessmentColumn builds a database at
// schema version 1 by hand, with a row already in it, then opens it through
// this package's own Open and checks migrate carried it to schemaVersion
// (2): the assessment column exists, the CHECK on it is live, and the row
// that predates the column survives with assessment reading back NULL —
// not an empty string masquerading as one.
func TestMigrate_FromSchemaVersion1AddsAssessmentColumn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "store.db")
	raw, err := sql.Open("sqlite", storeDSN(path, "_pragma=busy_timeout(5000)&_txlock=immediate"))
	require.NoError(t, err)
	_, err = raw.ExecContext(t.Context(), schemaVersion1)
	require.NoError(t, err)
	_, err = raw.ExecContext(t.Context(), "PRAGMA user_version = 1")
	require.NoError(t, err)
	_, err = raw.ExecContext(t.Context(),
		"INSERT INTO runs (at, repo, digest, exit_code, verdict, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"2026-08-19T00:00:00Z", "repo", "digest-pre-migration", 0, "approve", "v1", "v1")
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	db, err := Open(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	var version int
	require.NoError(t, db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version))
	assert.Equal(t, schemaVersion, version, "migrate must leave a version-1 database stamped at the current schemaVersion")

	rows, err := db.QueryContext(t.Context(), "PRAGMA table_info(runs)")
	require.NoError(t, err)
	names := []string{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		require.NoError(t, rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	assert.Contains(t, names, "assessment", "migrating from schema version 1 must add the assessment column")

	var verdict string
	var assessment sql.NullString
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT verdict, assessment FROM runs WHERE digest = ?", "digest-pre-migration").Scan(&verdict, &assessment))
	assert.Equal(t, "approve", verdict, "the pre-migration row's existing data must survive the migration")
	assert.False(t, assessment.Valid, "a pre-migration row has no assessment; the added column must read back NULL, not empty string")

	_, err = db.ExecContext(t.Context(),
		"INSERT INTO runs (at, repo, digest, exit_code, assessment, tool_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"2026-08-19T00:00:00Z", "repo", "digest-post-migration", 0, "bogus", "v1", "v1")
	assert.Error(t, err, "the migrated database's assessment CHECK must reject an invalid value")
}
