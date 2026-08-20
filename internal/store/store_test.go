package store

import (
	"path/filepath"
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
