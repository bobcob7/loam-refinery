package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/cli"
	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

// homeFor points LOAM_REFINERY_HOME at a fresh directory, collapsing config
// and store into one place (docs/config.md §2) so a test never touches the
// real machine's config. t.Setenv forbids t.Parallel() on every test that
// calls it.
func homeFor(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("LOAM_REFINERY_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	return home
}

func TestStoreAdapter_ValidRunWritesReviewAndRecordsRow(t *testing.T) {
	home := homeFor(t)
	adapter := newStoreAdapter(quietLog())
	ref := "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"
	source := []byte(`{"version":"1","verdict":"approve","ref":"` + ref + `","comments":[]}`)
	err := adapter.Save(t.Context(), cli.StoreInput{
		Dir: t.TempDir(), Source: source, Valid: true, Ref: ref, Verdict: "approve",
		Comments: 0, ToolVersion: "test", SchemaVersion: "1",
	})
	require.NoError(t, err)
	files := listFiles(t, filepath.Join(home, "reviews"))
	require.Len(t, files, 1, "one review file under reviews/")
	got, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Equal(t, source, got, "the stored bytes are exactly what was submitted")
	rows := runsOf(t, filepath.Join(home, "store.db"))
	require.Len(t, rows, 1)
	assert.Equal(t, 0, rows[0].exitCode)
	assert.Equal(t, "approve", rows[0].verdict.String)
	assert.Equal(t, ref, rows[0].ref.String)
}

func TestStoreAdapter_InvalidRunWritesRejectedAndRecordsRow(t *testing.T) {
	home := homeFor(t)
	adapter := newStoreAdapter(quietLog())
	source := []byte(`{}`)
	err := adapter.Save(t.Context(), cli.StoreInput{
		Dir: t.TempDir(), Source: source, Valid: false, ToolVersion: "test", SchemaVersion: "1",
	})
	require.NoError(t, err)
	files := listFiles(t, filepath.Join(home, "rejected"))
	require.Len(t, files, 1)
	got, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Equal(t, source, got)
	rows := runsOf(t, filepath.Join(home, "store.db"))
	require.Len(t, rows, 1)
	assert.Equal(t, 1, rows[0].exitCode)
	assert.False(t, rows[0].verdict.Valid, "no verdict on a document with none")
	assert.False(t, rows[0].ref.Valid, "no ref on a document with none")
}

// Fifty identical failures leave one file and fifty rows: content addressing
// deduplicates the file, and every attempt still gets its own row
// (docs/config.md §4.4.1).
func TestStoreAdapter_RepeatedFailuresLeaveOneFileManyRows(t *testing.T) {
	home := homeFor(t)
	adapter := newStoreAdapter(quietLog())
	source := []byte(`{}`)
	for range 50 {
		err := adapter.Save(t.Context(), cli.StoreInput{
			Dir: t.TempDir(), Source: source, Valid: false, ToolVersion: "test", SchemaVersion: "1",
		})
		require.NoError(t, err)
	}
	files := listFiles(t, filepath.Join(home, "rejected"))
	assert.Len(t, files, 1, "content addressing collapses fifty identical inputs to one file")
	rows := runsOf(t, filepath.Join(home, "store.db"))
	assert.Len(t, rows, 50, "every attempt still records its own row")
}

// An input over 1 MiB records a row but writes no file (docs/config.md
// §4.4.1); the omission is visible as a row with nothing under rejected/.
func TestStoreAdapter_OversizedRejectedInputRecordsRowWithNoFile(t *testing.T) {
	home := homeFor(t)
	adapter := newStoreAdapter(quietLog())
	source := make([]byte, (1<<20)+1)
	err := adapter.Save(t.Context(), cli.StoreInput{
		Dir: t.TempDir(), Source: source, Valid: false, ToolVersion: "test", SchemaVersion: "1",
	})
	require.NoError(t, err)
	assert.Empty(t, listFiles(t, filepath.Join(home, "rejected")), "an oversized input is never written")
	rows := runsOf(t, filepath.Join(home, "store.db"))
	require.Len(t, rows, 1)
	assert.Equal(t, 1, rows[0].exitCode)
}

// store.enabled:false touches neither the reviews/rejected trees nor
// store.db, on a passing run or a failing one.
func TestStoreAdapter_DisabledTouchesNeitherPath(t *testing.T) {
	home := homeFor(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.json"),
		[]byte(`{"version":"1","store":{"enabled":false}}`), 0o600))
	adapter := newStoreAdapter(quietLog())
	for _, valid := range []bool{true, false} {
		err := adapter.Save(t.Context(), cli.StoreInput{
			Dir: t.TempDir(), Source: []byte(`{}`), Valid: valid, Ref: "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f",
			ToolVersion: "test", SchemaVersion: "1",
		})
		require.NoError(t, err)
	}
	assert.NoDirExists(t, filepath.Join(home, "reviews"))
	assert.NoDirExists(t, filepath.Join(home, "rejected"))
	assert.NoFileExists(t, filepath.Join(home, "store.db"))
}

// A store.repos value that does not fit is a config error, exit 101, and it
// is caught before any store filesystem lookup — including before store.db
// is created, so a typo in a config a person never even runs validate
// against yet still fails loudly rather than silently accepting garbage.
func TestStoreAdapter_BadRepoOverrideFailsBeforeAnyFilesystemLookup(t *testing.T) {
	home := homeFor(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.json"),
		[]byte(`{"version":"1","store":{"repos":{"/some/path":"../../etc"}}}`), 0o600))
	adapter := newStoreAdapter(quietLog())
	err := adapter.Save(t.Context(), cli.StoreInput{
		Dir: t.TempDir(), Source: []byte(`{}`), Valid: false, ToolVersion: "test", SchemaVersion: "1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store.repos")
	assert.Contains(t, err.Error(), "../../etc")
	assert.NoDirExists(t, filepath.Join(home, "reviews"))
	assert.NoDirExists(t, filepath.Join(home, "rejected"))
	assert.NoFileExists(t, filepath.Join(home, "store.db"), "no store is created once config is invalid")
}

// A valid store.repos value is used exactly as written — never normalized.
func TestStoreAdapter_ValidRepoOverrideIsNeverNormalized(t *testing.T) {
	home := homeFor(t)
	dir := t.TempDir()
	// A double dash and a non-".." run of dots are both valid per
	// docs/config.md §4.8's segment pattern, and both are exactly what
	// normalizeSegment would collapse or reject if this path ran the
	// override through it — proving it does not.
	const name = "a--b/c..d"
	config := `{"version":"1","store":{"repos":{"` + jsonEscape(dir) + `":"` + name + `"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.json"), []byte(config), 0o600))
	adapter := newStoreAdapter(quietLog())
	err := adapter.Save(t.Context(), cli.StoreInput{
		Dir: dir, Source: []byte(`{}`), Valid: false, ToolVersion: "test", SchemaVersion: "1",
	})
	require.NoError(t, err)
	rows := runsOf(t, filepath.Join(home, "store.db"))
	require.Len(t, rows, 1)
	assert.Equal(t, name, rows[0].repo, "an override is used as written, never normalized or validated for shape")
}

func TestStoreAdapter_ReadOnlyStoreExitsWithErrorOnPassAndFail(t *testing.T) {
	home := homeFor(t)
	require.NoError(t, os.Chmod(home, 0o500))
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) })
	adapter := newStoreAdapter(quietLog())
	for _, valid := range []bool{true, false} {
		err := adapter.Save(t.Context(), cli.StoreInput{
			Dir: t.TempDir(), Source: []byte(`{"version":"1","verdict":"approve","ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f","comments":[]}`),
			Valid: valid, Ref: "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", Verdict: "approve",
			ToolVersion: "test", SchemaVersion: "1",
		})
		assert.Error(t, err)
	}
}

func TestValidRef(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", validRef("4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"))
	assert.Empty(t, validRef(""))
	assert.Empty(t, validRef("not-a-sha"))
}

func TestValidVerdict(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"approve", "request_changes", "comment"} {
		assert.Equal(t, v, validVerdict(v))
	}
	assert.Empty(t, validVerdict(""))
	assert.Empty(t, validVerdict("bogus"))
}

func TestValidateRepoOverrides(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateRepoOverrides(nil))
	assert.NoError(t, validateRepoOverrides(map[string]string{"/a": "github.com/x/y"}))
	err := validateRepoOverrides(map[string]string{"/a": "../../etc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store.repos")
	assert.Contains(t, err.Error(), "/a")
}

func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b[1 : len(b)-1])
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files
}

type runRow struct {
	exitCode int
	repo     string
	ref      sql.NullString
	verdict  sql.NullString
}

func runsOf(t *testing.T, dbPath string) []runRow {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.QueryContext(context.Background(), "SELECT exit_code, repo, ref, verdict FROM runs ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	var out []runRow
	for rows.Next() {
		var r runRow
		require.NoError(t, rows.Scan(&r.exitCode, &r.repo, &r.ref, &r.verdict))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}
