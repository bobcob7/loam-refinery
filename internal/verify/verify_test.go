package verify

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobcob7/refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const absentSHA = "0123456789abcdef0123456789abcdef01234567"

func TestVerifyChecksAnchorsAgainstARealRepository(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	tests := []struct {
		name     string
		anchors  []map[string]any
		want     string
		message  string
		verified int
	}{
		{
			name:     "a file and a line that exist verify",
			anchors:  []map[string]any{{"file": "internal/fetch/client.go", "line": 12, "end_line": 20}},
			verified: 1,
		},
		{
			name:    "a path that does not exist at the ref",
			anchors: []map[string]any{{"file": "internal/fetch/missing.go", "line": 1}},
			want:    "anchor-file-missing",
			message: fmt.Sprintf("internal/fetch/missing.go does not exist at %s", sha[:7]),
		},
		{
			name:    "a directory is not a file",
			anchors: []map[string]any{{"file": "internal/fetch"}},
			want:    "anchor-file-missing",
			message: fmt.Sprintf("internal/fetch is a directory at %s, not a file", sha[:7]),
		},
		{
			name:    "a line past the end of the file",
			anchors: []map[string]any{{"file": "internal/fetch/client.go", "line": 900}},
			want:    "anchor-line-out-of-range",
			message: fmt.Sprintf("line 900 is out of range in a 61-line file at %s", sha[:7]),
		},
		{
			name:    "an end_line past the end of the file",
			anchors: []map[string]any{{"file": "internal/fetch/client.go", "line": 60, "end_line": 62}},
			want:    "anchor-line-out-of-range",
			message: fmt.Sprintf("end_line 62 is out of range in a 61-line file at %s", sha[:7]),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc := document(t, sha, test.anchors)
			diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
			assert.Empty(t, skipped)
			assert.Equal(t, "repo", verification.Source)
			assert.Equal(t, len(test.anchors), verification.Anchors)
			assert.Equal(t, test.verified, verification.Verified)
			if test.want == "" {
				assert.Empty(t, diagnostics)
				return
			}
			require.Len(t, diagnostics, 1)
			assert.Equal(t, test.want, diagnostics[0].Name)
			assert.Equal(t, review.SeverityError, diagnostics[0].Severity)
			assert.Equal(t, test.message, diagnostics[0].Message)
		})
	}
}

func TestUnknownRefIsOneDiagnosticForTheDocument(t *testing.T) {
	t.Parallel()
	repository, _ := repo(t)
	doc := document(t, absentSHA, []map[string]any{
		{"file": "internal/fetch/client.go", "line": 1},
		{"file": "internal/fetch/client.go", "line": 2},
	})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	require.Len(t, diagnostics, 1, "an unresolvable ref is one diagnostic, not one per anchor")
	assert.Equal(t, "ref-unknown", diagnostics[0].Name)
	assert.Equal(t, "/ref", diagnostics[0].Path)
	assert.Equal(t, "ref 0123456 does not resolve in this repository", diagnostics[0].Message)
	assert.Zero(t, verification.Verified)
	names := []string{}
	for _, skip := range skipped {
		names = append(names, skip.Name)
		assert.Equal(t, "the document ref does not resolve", skip.Reason)
	}
	assert.Equal(t, []string{"anchor-file-missing", "anchor-line-out-of-range"}, names,
		"the anchor checks never ran, and a caller must not read that as a pass")
}

func TestADocumentWithNoRefIsSkippedNotPassed(t *testing.T) {
	t.Parallel()
	repository, _ := repo(t)
	doc := document(t, "", []map[string]any{{"file": "internal/fetch/client.go", "line": 12}})
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Equal(t, 1, verification.Anchors)
	assert.Zero(t, verification.Verified, "an unverifiable anchor is never counted as verified")
	names := []string{}
	for _, skip := range skipped {
		names = append(names, skip.Name)
		assert.Equal(t, "the document has no ref", skip.Reason)
	}
	assert.Equal(t, []string{"anchor-file-missing", "anchor-line-out-of-range"}, names)
}

func TestOneMalformedAnchorDoesNotStopTheOthers(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	doc := document(t, sha, []map[string]any{
		{"file": 12},
		{"file": "internal/fetch/client.go", "line": 12},
	})
	_, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Equal(t, 1, verification.Verified, "one unusable anchor does not stop the other")
	require.NotEmpty(t, skipped)
	assert.Equal(t, "unusable field on 1 anchor", skipped[0].Reason)
}

func TestDiscoverFailsOutsideARepository(t *testing.T) {
	t.Parallel()
	_, err := Discover(t.Context(), t.TempDir())
	require.Error(t, err)
	assert.Equal(t, "not a git repository", err.Error())
}

func TestFileLookupsAreCachedPerRefAndPath(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{runFunc: func(_ context.Context, args ...string) ([]byte, error) {
		switch {
		case args[1] == "-t" && len(args) == 3 && !strings.Contains(args[2], ":"):
			return []byte("commit\n"), nil
		case args[1] == "-t":
			return []byte("blob\n"), nil
		default:
			return []byte("one\ntwo\nthree\nfour\n"), nil
		}
	}}
	doc := document(t, absentSHA, []map[string]any{
		{"file": "a.go", "line": 1},
		{"file": "a.go", "line": 2},
		{"file": "a.go", "line": 3},
	})
	_, _, verification := New(git, logger()).Verify(t.Context(), doc)
	assert.Equal(t, 3, verification.Verified)
	assert.Len(t, git.runCalls(), 3, "the ref resolves once, then one type and one blob read per distinct path")
}

func TestCountLinesTreatsATrailingFragmentAsALine(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, countLines(nil))
	assert.Equal(t, 1, countLines([]byte("one\n")))
	assert.Equal(t, 2, countLines([]byte("one\ntwo")))
	assert.Equal(t, 2, countLines([]byte("one\ntwo\n")))
}

// repo builds a throwaway repository holding one 61-line file and returns it
// with the SHA of its only commit.
func repo(t *testing.T) (*Repository, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "fetch"), 0o755))
	lines := make([]string, 61)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "fetch", "client.go"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644))
	run(t, dir, "init", "-q")
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-qm", "init")
	sha := strings.TrimSpace(run(t, dir, "rev-parse", "HEAD"))
	repository, err := Discover(t.Context(), dir)
	require.NoError(t, err)
	return repository, sha
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

func document(t *testing.T, ref string, anchors []map[string]any) *review.Document {
	t.Helper()
	doc := &review.Document{}
	if ref != "" {
		doc.Ref = review.Field[string]{Value: ref, Present: true, OK: true}
	}
	comment := review.Comment{
		Index: 0, Path: "/comments/0", Object: true,
		ID: review.Field[string]{Value: "dropped-context-1", Present: true, OK: true},
	}
	for i, anchor := range anchors {
		parsed := review.Anchor{Index: i, Path: fmt.Sprintf("/comments/0/anchors/%d", i), Object: true}
		if file, ok := anchor["file"].(string); ok {
			parsed.File = review.Field[string]{Value: file, Present: true, OK: true}
		} else if _, present := anchor["file"]; present {
			parsed.File = review.Field[string]{Present: true}
		}
		if line, ok := anchor["line"].(int); ok {
			parsed.Line = review.Field[int]{Value: line, Present: true, OK: true}
		}
		if end, ok := anchor["end_line"].(int); ok {
			parsed.EndLine = review.Field[int]{Value: end, Present: true, OK: true}
		}
		comment.Anchors = append(comment.Anchors, parsed)
	}
	doc.Comments = []review.Comment{comment}
	doc.CommentsArray = true
	doc.CommentsWellTyped = true
	return doc
}

func logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestAnAnchorWithAnUnreadableLineIsNotCountedAsVerified(t *testing.T) {
	t.Parallel()
	repository, sha := repo(t)
	doc := document(t, sha, []map[string]any{{"file": "internal/fetch/client.go"}})
	doc.Comments[0].Anchors[0].Line = review.Field[int]{Present: true}
	diagnostics, skipped, verification := New(repository, logger()).Verify(t.Context(), doc)
	assert.Empty(t, diagnostics)
	assert.Zero(t, verification.Verified, "a line that cannot be read was never range checked")
	require.NotEmpty(t, skipped)
	assert.Equal(t, "unusable field on 1 anchor", skipped[0].Reason)
}
