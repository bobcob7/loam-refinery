package store

import (
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
}

func TestGit_Root_OutsideRepository(t *testing.T) {
	t.Parallel()
	g := NewGit(testLogger())
	_, err := g.root(t.Context(), t.TempDir())
	assert.ErrorIs(t, err, verify.ErrNoRepository)
}

func TestGit_Root_And_OriginURL_InsideRepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/bobcob7/loam-refinery.git")
	g := NewGit(testLogger())
	root, err := g.root(t.Context(), dir)
	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, resolved, root)
	origin, err := g.originURL(t.Context(), root)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/bobcob7/loam-refinery.git", origin)
}

func TestGit_OriginURL_NoOrigin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	g := NewGit(testLogger())
	origin, err := g.originURL(t.Context(), dir)
	require.NoError(t, err)
	assert.Empty(t, origin)
}

// TestRepoName_EndToEnd_OutsideAnyRepository proves config.md section 4.2
// against a real git binary: validate outside any git repository files
// under no-repo.
func TestRepoName_EndToEnd_OutsideAnyRepository(t *testing.T) {
	t.Parallel()
	name, err := RepoName(t.Context(), NewGit(testLogger()), t.TempDir(), nil)
	require.NoError(t, err)
	assert.Equal(t, "no-repo", name)
}

func TestRepoName_EndToEnd_RepositoryWithOrigin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "git@github.com:bobcob7/loam-refinery.git")
	name, err := RepoName(t.Context(), NewGit(testLogger()), dir, nil)
	require.NoError(t, err)
	assert.Equal(t, "github.com/bobcob7/loam-refinery", name)
}

func TestRepoName_EndToEnd_RepositoryWithNoOrigin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	name, err := RepoName(t.Context(), NewGit(testLogger()), dir, nil)
	require.NoError(t, err)
	assert.Equal(t, "local/"+filepath.Base(dir), name)
}
