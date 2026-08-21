package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

// TestGit_OriginURL_DeadlineExceededIsError proves refinery-xlp.12 end to
// end against a real killed process: a git invocation that never gets to
// answer because the deadline caught it must come back as an error, never
// as "no origin configured". The fake git here sleeps far past the
// deadline it is given, forcing run to kill it the same way a real git
// stuck behind a slow filesystem or a hung credential helper would be
// killed.
//
// Not t.Parallel(): it calls t.Setenv, which the testing package forbids
// on a parallel test.
func TestGit_OriginURL_DeadlineExceededIsError(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	require.NoError(t, os.WriteFile(shim, []byte("#!/bin/sh\n/bin/sleep 30\n"), 0o755))
	t.Setenv("PATH", dir)
	g := NewGit(testLogger())
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	origin, err := g.originURL(ctx, t.TempDir())
	assert.Less(t, time.Since(start), 20*time.Second, "run must not wait out the full sleep once the deadline kills it")
	require.Error(t, err)
	assert.ErrorIs(t, err, errGitTimedOut, "a deadline-killed invocation must be an error, not an absent remote")
	assert.Empty(t, origin)
}

// TestClassifyOriginErr_DeadlineExceededIsErrorEvenWhenExitedReportsTrue
// reproduces the bug refinery-xlp.12 fixes without needing Windows to prove
// it: exec.ExitError.Exited() is hardcoded true there regardless of whether
// the process was killed, so a wrapped ExitError reporting Exited()==true
// is exactly what run would hand back for a deadline-killed git on that
// platform. errGitTimedOut is what makes that case distinguishable from a
// process that genuinely ran to completion and answered "no such key".
func TestClassifyOriginErr_DeadlineExceededIsErrorEvenWhenExitedReportsTrue(t *testing.T) {
	t.Parallel()
	exited := exec.Command("false").Run()
	require.Error(t, exited)
	var exit *exec.ExitError
	require.ErrorAs(t, exited, &exit)
	require.True(t, exit.Exited(), "the fixture must be a normally-exited process for this test to mean anything")
	timedOut := fmt.Errorf("%w: %w", errGitTimedOut, exited)
	origin, err := classifyOriginErr(timedOut)
	assert.Empty(t, origin)
	assert.ErrorIs(t, err, errGitTimedOut, "a deadline-killed invocation must be an error, not an absent remote")
}

// TestClassifyOriginErr_ExitedWithoutTimeoutIsAbsentRemote is the case
// classifyOriginErr must still get right: an exited process that never hit
// the deadline reads as git having looked and found nothing.
func TestClassifyOriginErr_ExitedWithoutTimeoutIsAbsentRemote(t *testing.T) {
	t.Parallel()
	exited := exec.Command("false").Run()
	require.Error(t, exited)
	origin, err := classifyOriginErr(exited)
	assert.NoError(t, err)
	assert.Empty(t, origin)
}

// TestClassifyOriginErr_NonExitFailureIsError covers a failure that never
// produced an *exec.ExitError at all — a missing git binary, say — which
// must read as an error rather than as an absent remote.
func TestClassifyOriginErr_NonExitFailureIsError(t *testing.T) {
	t.Parallel()
	_, err := classifyOriginErr(errors.New(`exec: "git": executable file not found in $PATH`))
	assert.Error(t, err)
}
