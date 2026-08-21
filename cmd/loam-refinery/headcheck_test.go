package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/cli"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These pin refinery-uyb.10's behaviour in Go values, per docs/features/
// combined-reviews.md §4.3 and §4.3.1: the four JSON-shape assertions belong
// to the CLI-wiring bead that encodes head_check, but is_head, the
// qualified-id translation, the JSON-Pointer-to-file translation, and the
// nil-vs-empty distinction for diverged are this adapter's own contract and
// are pinned here, against real git.

// discoverAndCheck runs the two-call sequence internal/cli.collectHeadCheck
// itself makes: Discover once, then Diverged against doc, mirroring how a
// real collect-reviews invocation drives headChecker (refinery-k3h).
func discoverAndCheck(t *testing.T, dir string, doc *review.Document, qualifiedIDs map[string]string) (source string, isHead bool, diverged []cli.DivergedAnchor) {
	t.Helper()
	head, err := newHeadCheckAdapter(quietLog()).Discover(t.Context(), dir, doc.Ref.Value)
	require.NoError(t, err)
	d, err := head.Diverged(t.Context(), doc, qualifiedIDs)
	require.NoError(t, err)
	return head.Source(), head.IsHead(), d
}

func TestHeadCheckAdapter_SourceIsNoneOutsideARepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	doc := parseDoc(t, `{"ref":"`+strings.Repeat("a", 40)+`","comments":[]}`)
	source, isHead, diverged := discoverAndCheck(t, dir, doc, nil)
	assert.Equal(t, "none", source)
	assert.False(t, isHead)
	assert.Nil(t, diverged, "diverged is absent, not empty, outside a repository")
}

// A bare repository has no working tree to resolve --show-toplevel against,
// so git refuses with a real error that is not "not a git repository" -
// exactly the "a repository exists but could not be asked" case source
// "unavailable" names.
func TestHeadCheckAdapter_SourceIsUnavailableWhenDiscoveryFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "--bare", "-q")
	doc := parseDoc(t, `{"ref":"`+strings.Repeat("a", 40)+`","comments":[]}`)
	source, isHead, diverged := discoverAndCheck(t, dir, doc, nil)
	assert.Equal(t, "unavailable", source)
	assert.False(t, isHead)
	assert.Nil(t, diverged, "diverged is absent, not empty, when the repository could not be asked")
}

func TestHeadCheckAdapter_IsHeadTrueAndDivergedEmptyForAnUntouchedWorkingTree(t *testing.T) {
	t.Parallel()
	dir, sha := headCheckRepo(t, map[string]string{"a.go": "one\ntwo\n"})
	doc := parseDoc(t, `{"ref":"`+sha+`","comments":[{"id":"c1","anchors":[{"file":"a.go","line":1}]}]}`)
	source, isHead, diverged := discoverAndCheck(t, dir, doc, map[string]string{"c1": "backend:c1"})
	assert.Equal(t, "repo", source)
	assert.True(t, isHead)
	require.NotNil(t, diverged, "diverged is present, not absent, once the check has run")
	assert.Empty(t, diverged, "nothing drifted, so the array is empty rather than carrying a false positive")
}

// Mutation guard for §4.3.1's absent-vs-empty distinction: a ref that is not
// HEAD must report diverged as nil, never []DivergedAnchor{}, however much
// the working tree has actually moved - the check does not apply to it at
// all, so there is nothing to report, not nothing found.
func TestHeadCheckAdapter_DivergedIsAbsentWhenRefIsNotHEAD(t *testing.T) {
	t.Parallel()
	dir, firstSHA, _ := headCheckTwoCommitRepo(t, "a.go", "first\n", "second\n")
	doc := parseDoc(t, `{"ref":"`+firstSHA+`","comments":[{"id":"c1","anchors":[{"file":"a.go","line":1}]}]}`)
	source, isHead, diverged := discoverAndCheck(t, dir, doc, map[string]string{"c1": "backend:c1"})
	assert.Equal(t, "repo", source)
	assert.False(t, isHead)
	assert.Nil(t, diverged, "the check does not apply to a non-HEAD ref, so diverged must be absent, not []")
}

// Pins both translations §4.3.1 requires in one fixture with more than one
// comment and more than one anchor per comment, so a bug that emits the bare
// origin id or the wrong anchor's pointer would be caught by a collision or
// a visibly wrong path rather than passing on a single-anchor fixture.
func TestHeadCheckAdapter_DivergedCarriesQualifiedIdsAndRealFilePathsNotPointers(t *testing.T) {
	t.Parallel()
	dir, sha := headCheckRepo(t, map[string]string{
		"a.go": "unchanged\n",
		"b.go": "line one\nline two\n",
		"c.go": "line one\nline two\n",
	})
	writeHeadCheckFile(t, dir, "b.go", "changed\n")
	writeHeadCheckFile(t, dir, "c.go", "also changed\n")
	doc := parseDoc(t, `{"ref":"`+sha+`","comments":[
		{"id":"dropped-context-1","anchors":[{"file":"a.go","line":1},{"file":"b.go","line":1}]},
		{"id":"dropped-context-2","anchors":[{"file":"c.go","line":1}]}
	]}`)
	qualified := map[string]string{"dropped-context-1": "backend:dropped-context-1", "dropped-context-2": "#2:dropped-context-2"}
	_, isHead, diverged := discoverAndCheck(t, dir, doc, qualified)
	require.True(t, isHead)
	require.Len(t, diverged, 2, "a.go's anchor did not diverge; only b.go's and c.go's did")
	byFile := map[string]string{}
	for _, d := range diverged {
		assert.Equal(t, "anchor-worktree-diverged", d.Name)
		byFile[d.File] = d.Comment
	}
	assert.Equal(t, "backend:dropped-context-1", byFile["b.go"], "the qualified id, not the bare origin id dropped-context-1")
	assert.Equal(t, "#2:dropped-context-2", byFile["c.go"], "each comment's own qualified id, not the first comment's")
}

// A second submission's qualifiedIDs map is consulted independently: two
// origin ids that collide across submissions must not resolve to each
// other's qualified id. This also exercises refinery-k3h's fix directly: one
// Discover call, its HeadCheck reused across two Diverged calls, exactly the
// shape internal/cli.collectHeadCheck now drives for two surviving
// submissions of the same ref.
func TestHeadCheckAdapter_QualifiedIdsAreScopedToTheSuppliedMap(t *testing.T) {
	t.Parallel()
	dir, sha := headCheckRepo(t, map[string]string{"a.go": "line one\n"})
	writeHeadCheckFile(t, dir, "a.go", "changed\n")
	doc := parseDoc(t, `{"ref":"`+sha+`","comments":[{"id":"dropped-context-1","anchors":[{"file":"a.go","line":1}]}]}`)
	head, err := newHeadCheckAdapter(quietLog()).Discover(t.Context(), dir, sha)
	require.NoError(t, err)
	first, err := head.Diverged(t.Context(), doc, map[string]string{"dropped-context-1": "backend:dropped-context-1"})
	require.NoError(t, err)
	second, err := head.Diverged(t.Context(), doc, map[string]string{"dropped-context-1": "#2:dropped-context-1"})
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, "backend:dropped-context-1", first[0].Comment)
	assert.Equal(t, "#2:dropped-context-1", second[0].Comment)
}

func TestAnchorFile_ResolvesTheAnchorsOwnFileFieldFromThePointer(t *testing.T) {
	t.Parallel()
	doc := parseDoc(t, `{"comments":[{"id":"c1","anchors":[{"file":"first.go"},{"file":"second.go"}]},{"id":"c2","anchors":[{"file":"third.go"}]}]}`)
	assert.Equal(t, "first.go", anchorFile(doc, "/comments/0/anchors/0"))
	assert.Equal(t, "second.go", anchorFile(doc, "/comments/0/anchors/1"))
	assert.Equal(t, "third.go", anchorFile(doc, "/comments/1/anchors/0"))
}

// A pointer this cannot resolve reports "" rather than a guess: the
// unresolvable path itself is not a valid file, and a mismatched suffix must
// not be silently truncated into a resolvable one.
func TestAnchorFile_ReturnsEmptyForAnUnresolvablePointer(t *testing.T) {
	t.Parallel()
	doc := parseDoc(t, `{"comments":[{"id":"c1","anchors":[{"file":"only.go"}]}]}`)
	assert.Empty(t, anchorFile(doc, "/comments/0/anchors/9"))
	assert.Empty(t, anchorFile(doc, "/comments/9/anchors/0"))
	assert.Empty(t, anchorFile(doc, "/ref"))
	assert.Empty(t, anchorFile(doc, "/comments/0/anchors/0/extra"))
}

func parseDoc(t *testing.T, source string) *review.Document {
	t.Helper()
	doc, err := review.Parse([]byte(source))
	require.NoError(t, err)
	return doc
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

func writeHeadCheckFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// headCheckRepo builds a throwaway repository with one commit holding files,
// and returns it with that commit's SHA.
func headCheckRepo(t *testing.T, files map[string]string) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	for name, content := range files {
		writeHeadCheckFile(t, dir, name, content)
	}
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "init")
	sha = strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	return dir, sha
}

// headCheckTwoCommitRepo builds a repository with two commits touching one
// file and returns it with both SHAs; the second is HEAD.
func headCheckTwoCommitRepo(t *testing.T, name, first, second string) (dir, firstSHA, secondSHA string) {
	t.Helper()
	dir = t.TempDir()
	writeHeadCheckFile(t, dir, name, first)
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "first")
	firstSHA = strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	writeHeadCheckFile(t, dir, name, second)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "second")
	secondSHA = strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	return dir, firstSHA, secondSHA
}
