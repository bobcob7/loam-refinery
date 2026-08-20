package cli

import (
	"bytes"
	"sort"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/render"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cliDoc = "../../docs/cli.md"

// docResultObject builds the *review.Result docs/cli.md §5.2's first
// example describes: valid, not strict, verified in a repository (source
// "repo", so no "reason"), and exactly the two advisories the example
// itself names — suggestion-no-cons (which carries a comment id and a JSON
// pointer path) and priority-flat (which, being document-level, carries
// neither). Driving these two specific checks was picked deliberately: the
// example was written around them, so this reproduces it rather than an
// arbitrary result of the same shape.
func docResultObject() *review.Result {
	return &review.Result{
		Valid:        true,
		Strict:       false,
		Comments:     6,
		Verification: review.Verification{Source: "repo", Anchors: 9, Verified: 9},
		Diagnostics: []review.Diagnostic{
			{
				Severity: review.SeverityAdvisory, Name: "suggestion-no-cons", Comment: "dropped-context-1",
				Path:    "/comments/0/suggestions/0/cons",
				Message: "suggestion 1 lists no cons; state the tradeoff or say the fix is free",
			},
			{
				Severity: review.SeverityAdvisory, Name: "priority-flat",
				Message: "all 6 comments are priority 7; the scale is not being used",
			},
		},
	}
}

// renderResult runs result through the real JSON renderer — the same
// render.NewJSON validate wires as a.renderer (see cmd/loam-refinery/main.go)
// — so "real command output" means the actual encoding path a.validate()
// calls, not a hand-rolled stand-in for it.
func renderResult(t *testing.T, result *review.Result) string {
	t.Helper()
	stdout := &bytes.Buffer{}
	require.NoError(t, render.NewJSON().Result(stdout, result))
	return stdout.String()
}

// TestValidateResultObjectMatchesDocumentedShape reproduces docs/cli.md
// §5.2's main result-object example for real: two advisories, one of them
// document-level, so both diagnostics.comment/path being present and being
// absent are exercised, along with lenses appearing once diagnostics is
// non-empty.
func TestValidateResultObjectMatchesDocumentedShape(t *testing.T) {
	t.Parallel()
	got := realJSON(t, renderResult(t, docResultObject()))
	assertShapeMatchesDoc(t, got, cliDoc, "### 5.2 The result object", 1,
		"docs/cli.md §5.2: the result object must match the documented example")
}

// TestValidateResultObjectOutsideRepositoryMatchesDocumentedShape
// reproduces docs/cli.md §5.2's second example — a document validated
// outside a repository — through the real validate pipeline rather than a
// hand-built Result, since the shape here (verification.reason present,
// skipped grouped by reason, diagnostics and lenses both absent) is
// exactly what TestValidateOutsideARepositoryStaysWithinBudget already
// drives for real.
func TestValidateResultObjectOutsideRepositoryMatchesDocumentedShape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clean := `{"version":"1","verdict":"approve","summary":"The retry loop is sound and the deadline propagates to every call it makes.","ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f","comments":[]}`
	out, code := runValidate(t, dir, clean)
	require.Equal(t, ExitValid, code, out)
	assertShapeMatchesDoc(t, realJSON(t, out), cliDoc, "### 5.2 The result object", 2,
		"docs/cli.md §5.2: validating outside a repository must match the documented example")
}

// TestValidateResultObjectKeysAreExactlyDocumented pins refinery-a96.23's
// closed set for the validate result object: valid, strict, verification,
// counts, skipped, diagnostics, lenses, and nothing describing the store
// (docs/cli.md §5.2: "Nothing here describes the store."). It uses the
// maximal-shape result above — the one case where every optional key
// (lenses) is present — because a field added anywhere else would already
// appear here too; checking only the "clean" case would miss a field the
// renderer only adds once there is something to say, the same blind spot
// omitempty gives everywhere else in this codebase.
func TestValidateResultObjectKeysAreExactlyDocumented(t *testing.T) {
	t.Parallel()
	got := realJSON(t, renderResult(t, docResultObject())).(map[string]any)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"counts", "diagnostics", "lenses", "skipped", "strict", "valid", "verification"}
	assert.Equal(t, want, keys, "docs/cli.md §5.2: the result object has exactly these keys, and none describing the store")
}
