package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewReadOnly_WorksOnAReadOnlyDirectory proves refinery-a96.19: a store
// NewReadOnly opens answers reads even when the store directory itself is
// not writable — a read-only container, a CI sandbox, a machine with no
// writable $HOME (docs/config.md §5.2) — because it never migrates, never
// creates root, and opens store.db with mode=ro rather than New's
// read-write connection. Measured against the branch before this fix:
// store.New on a chmod'd 0500 directory failed "attempt to write a
// readonly database (1544)", because opening read-write always attempts
// the WAL/migration path even when nothing has changed.
func TestNewReadOnly_WorksOnAReadOnlyDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not restrict directory writes the same way on windows")
	}
	dir := t.TempDir()
	rw, err := New(t.Context(), dir, NewClock())
	require.NoError(t, err)
	digest, _, err := rw.WriteRejected("github.com/example/example", []byte("{}"))
	require.NoError(t, err)
	require.NoError(t, rw.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: digest, ExitCode: 1,
		ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	require.NoError(t, rw.Close())
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	ro, err := NewReadOnly(t.Context(), dir)
	require.NoError(t, err, "a read-only open must succeed on a read-only directory")
	defer ro.Close()
	known, err := ro.Known(t.Context(), "github.com/example/example")
	require.NoError(t, err)
	assert.True(t, known)
	failed, total, err := ro.ListFailedRuns(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, failed, 1)
}

// TestNewReadOnly_NeverCreatesRoot proves NewReadOnly does not call
// os.MkdirAll the way New does (docs/config.md §2.2, §6): a read must
// never materialize a store on a machine that has none.
func TestNewReadOnly_NeverCreatesRoot(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "nonexistent")
	_, err := NewReadOnly(t.Context(), root)
	assert.Error(t, err)
	assert.NoDirExists(t, root)
}

// TestNewReadOnly_ReadsLiveWALNotOnlyTheCheckpointedFile proves the
// correctness half of OpenReadOnly's mode=ro-first strategy: with a second
// connection deliberately held open to keep the WAL live (unchecked-
// pointed) — the state a store is actually in while validate is running,
// or right after one crashes before its automatic checkpoint-on-close —
// NewReadOnly still returns every row, including the one that only exists
// in the WAL, on a directory chmod'd to 0500. immutable=1 by itself cannot
// do this: it assumes the file will never change and reads only the
// checkpointed main database file, silently missing whatever is still in
// the WAL. This is what would go red if OpenReadOnly's mode=ro attempt
// were dropped in favor of always using immutable=1.
func TestNewReadOnly_ReadsLiveWALNotOnlyTheCheckpointedFile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not restrict directory writes the same way on windows")
	}
	dir := t.TempDir()
	rw, err := New(t.Context(), dir, NewClock())
	require.NoError(t, err)
	digest1, _, err := rw.WriteRejected("github.com/example/example", []byte("{}"))
	require.NoError(t, err)
	require.NoError(t, rw.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: digest1, ExitCode: 1,
		ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	// A second connection to the same file, left open, is what keeps SQLite
	// from doing its automatic full checkpoint when rw closes below — the
	// same effect a concurrent reader or a crash-before-close has in
	// production.
	holder, err := Open(t.Context(), filepath.Join(dir, "store.db"))
	require.NoError(t, err)
	defer holder.Close()
	rows, err := holder.QueryContext(t.Context(), "SELECT 1")
	require.NoError(t, err)
	require.NoError(t, rows.Close())
	digest2, _, err := rw.WriteRejected("github.com/example/example", []byte("{}") /* same bytes: distinct row, same file */)
	require.NoError(t, err)
	require.NoError(t, rw.Record(t.Context(), RunInput{
		Repo: "github.com/example/example", Digest: digest2, ExitCode: 1,
		ToolVersion: "0.1.0", SchemaVersion: "1",
	}))
	require.NoError(t, rw.Close())
	require.FileExists(t, filepath.Join(dir, "store.db-wal"), "holder must have kept the WAL live for this test to prove anything")
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	ro, err := NewReadOnly(t.Context(), dir)
	require.NoError(t, err)
	defer ro.Close()
	_, total, err := ro.ListFailedRuns(t.Context(), "github.com/example/example", "", 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total, "both rows must be visible, including the one only in the live WAL")
}

// TestNewReadOnly_NeverMigrates proves a database at a schema version this
// binary does not recognize is still opened for reading: NewReadOnly's
// contract is "answer from what is there," not "bring it current."
func TestNewReadOnly_NeverMigrates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rw, err := New(t.Context(), dir, NewClock())
	require.NoError(t, err)
	require.NoError(t, rw.Close())
	ro, err := NewReadOnly(t.Context(), dir)
	require.NoError(t, err)
	defer ro.Close()
	var version int
	require.NoError(t, ro.db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version))
	assert.Equal(t, schemaVersion, version, "the database New already migrated stays as it was")
}
