package render

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobcob7/refinery/internal/entry"
	"github.com/bobcob7/refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestTextResult(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result *review.Result
		golden string
	}{
		{name: "clean", result: clean(), golden: "text-clean"},
		{name: "unverified", result: unverified(), golden: "text-unverified"},
		{name: "diagnostics", result: messy(), golden: "text-diagnostics"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			require.NoError(t, NewText().Result(stdout, stderr, test.result))
			golden(t, test.golden+".txt", stdout.String()+stderr.String())
		})
	}
}

func TestTextSendsTheStatusLineToStdoutAndDiagnosticsToStderr(t *testing.T) {
	t.Parallel()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	require.NoError(t, NewText().Result(stdout, stderr, messy()))
	assert.Contains(t, stdout.String(), "INVALID")
	assert.NotContains(t, stdout.String(), "error ")
	assert.Contains(t, stderr.String(), "error ")
	assert.Contains(t, stderr.String(), "refinery describe --lens=")
	assert.NotContains(t, stdout.String(), "--lens=")
}

func TestTextSuppressesThePointerLineWhenThereIsNothingToSay(t *testing.T) {
	t.Parallel()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	require.NoError(t, NewText().Result(stdout, stderr, clean()))
	assert.Empty(t, stderr.String())
}

func TestTextWrapsMessagesUnderTheirColumn(t *testing.T) {
	t.Parallel()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	require.NoError(t, NewText().Result(stdout, stderr, messy()))
	for _, line := range bytes.Split(stderr.Bytes(), []byte("\n")) {
		if bytes.HasPrefix(line, []byte("refinery describe")) {
			continue // the pointer line is runnable, so it is never wrapped
		}
		assert.LessOrEqual(t, len([]rune(string(line))), width, "line is wider than the column: %q", line)
	}
}

func TestJSONResult(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result *review.Result
		golden string
	}{
		{name: "clean", result: clean(), golden: "json-clean"},
		{name: "unverified", result: unverified(), golden: "json-unverified"},
		{name: "diagnostics", result: messy(), golden: "json-diagnostics"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			require.NoError(t, NewJSON().Result(stdout, stderr, test.result))
			assert.Empty(t, stderr.String(), "json output goes to stdout alone")
			golden(t, test.golden+".json", stdout.String())
		})
	}
}

func TestJSONAlwaysCarriesVerificationAndSkipped(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	require.NoError(t, NewJSON().Result(stdout, &bytes.Buffer{}, clean()))
	assert.Contains(t, stdout.String(), `"verification"`)
	assert.Contains(t, stdout.String(), `"skipped": []`)
	assert.NotContains(t, stdout.String(), `"lenses"`, "omitted when there are no diagnostics")
}

func TestEntriesAndIndex(t *testing.T) {
	t.Parallel()
	entries := []entry.Entry{{
		Name:      "priority",
		Namespace: entry.NamespaceField,
		Title:     "Priority",
		Body:      "Integer 1-10, higher is more urgent.\n\n  before: 12\n  after:  9",
		Example:   "9",
		Related:   []string{"category", "id"},
	}}
	stdout := &bytes.Buffer{}
	require.NoError(t, NewText().Entries(stdout, entries))
	golden(t, "text-entry.txt", stdout.String())
	stdout.Reset()
	require.NoError(t, NewText().Index(stdout, []entry.Group{
		{Namespace: entry.NamespaceField, Names: []string{"priority", "category"}},
		{Namespace: entry.NamespaceCheck, Names: []string{"id-unique"}},
	}))
	golden(t, "text-index.txt", stdout.String())
	stdout.Reset()
	require.NoError(t, NewJSON().Entries(stdout, entries))
	golden(t, "json-entry.json", stdout.String())
}

func clean() *review.Result {
	return &review.Result{
		Valid:        true,
		Comments:     5,
		Verification: review.Verification{Source: "repo", Anchors: 5, Verified: 5},
	}
}

func unverified() *review.Result {
	return &review.Result{
		Valid:        true,
		Comments:     5,
		Verification: review.Verification{Source: "none", Reason: "not a git repository"},
		Skipped: []review.Skipped{
			{Name: "ref-unknown", Reason: "not a git repository"},
			{Name: "anchor-file-missing", Reason: "not a git repository"},
			{Name: "anchor-line-out-of-range", Reason: "not a git repository"},
		},
	}
}

func messy() *review.Result {
	return &review.Result{
		Comments:     6,
		Verification: review.Verification{Source: "repo", Anchors: 7, Verified: 6},
		Diagnostics: []review.Diagnostic{
			{
				Severity: review.SeverityError, Name: "anchor-file-missing", Comment: "dropped-context-1",
				Path: "/comments/0/anchors/0/file", Message: "internal/fetch/client.go does not exist at 4f2c1a9",
			},
			{
				Severity: review.SeverityError, Name: "schema", Path: "/comments/1/priority",
				Message: "12 is greater than the maximum of 10", Lens: "priority",
			},
			{
				Severity: review.SeverityAdvisory, Name: "suggestion-no-cons", Comment: "dropped-context-1",
				Path:    "/comments/0/suggestions/0/cons",
				Message: `suggestion 1 ("Pass the caller's context straight through") lists no cons; state the tradeoff or say the fix is free`,
			},
			{Severity: review.SeverityAdvisory, Name: "priority-flat", Message: "all 6 comments are priority 7; the scale is not being used"},
		},
		Skipped: []review.Skipped{
			{Name: "priority-flat", Reason: "2 comments have unusable priority"},
			{Name: "comment-flood", Reason: "2 comments have unusable priority"},
		},
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run go test ./... -update to create %s", path)
	assert.Equal(t, string(want), got)
}
