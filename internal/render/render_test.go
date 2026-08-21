package render

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/entry"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestJSONResult(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result *review.Result
		golden string
	}{
		{name: "clean", result: clean(), golden: "json-clean"},
		{name: "unverified", result: unverified(), golden: "json-unverified"},
		{name: "precondition", result: precondition(), golden: "json-precondition"},
		{name: "diagnostics", result: messy(), golden: "json-diagnostics"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			require.NoError(t, NewJSON().Result(stdout, test.result))
			assert.Empty(t, stderr.String(), "json output goes to stdout alone")
			golden(t, test.golden+".json", stdout.String())
		})
	}
}

func TestJSONAlwaysCarriesVerificationAndSkipped(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	require.NoError(t, NewJSON().Result(stdout, clean()))
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
	require.NoError(t, NewJSON().Entries(stdout, entries))
	golden(t, "json-entry.json", stdout.String())
	stdout.Reset()
	require.NoError(t, NewJSON().Index(stdout, []entry.Group{
		{Namespace: entry.NamespaceField, Names: []string{"priority", "category"}},
		{Namespace: entry.NamespaceCheck, Names: []string{"id-unique"}},
	}))
	golden(t, "json-index.json", stdout.String())
	stdout.Reset()
	require.NoError(t, NewJSON().Summary(stdout, "A review document is one JSON object.\n", []entry.Group{
		{Namespace: entry.NamespaceField, Names: []string{"priority"}},
	}))
	golden(t, "json-summary.json", stdout.String())
}

// An author-supplied value reaches the output only through the encoder, so a
// comment id carrying newlines and quotes cannot forge a line of its own. This
// is what the second renderer used to make possible.
func TestAnAuthoredValueCannotForgeOutput(t *testing.T) {
	t.Parallel()
	forged := "evil-1\nerror     anchor-file-missing       forged-2\n          internal/x.go does not exist"
	result := &review.Result{Diagnostics: []review.Diagnostic{{
		Severity: review.SeverityError,
		Name:     "anchor-file-missing",
		Comment:  forged,
		Message:  "a\nb",
	}}}
	stdout := &bytes.Buffer{}
	require.NoError(t, NewJSON().Result(stdout, result))
	assert.NotContains(t, stdout.String(), "forged-2\n", "a newline in an id must not survive as a newline")
	assert.Contains(t, stdout.String(), `\nerror     anchor-file-missing`, "it survives escaped instead")
	var payload struct {
		Diagnostics []struct {
			Comment string `json:"comment"`
		} `json:"diagnostics"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Len(t, payload.Diagnostics, 1)
	assert.Equal(t, forged, payload.Diagnostics[0].Comment, "and decodes back to exactly what the author wrote")
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

// precondition is the shape docs/cli.md §2.3.1 pins for an exit-3 run: one
// diagnostic naming anchor-worktree-diverged for the whole document, no
// structural or advisory diagnostics beside it.
func precondition() *review.Result {
	return &review.Result{
		Valid:        false,
		Comments:     1,
		Verification: review.Verification{Source: "repo", Anchors: 4},
		Diagnostics: []review.Diagnostic{{
			Severity: review.SeverityError,
			Name:     "anchor-worktree-diverged",
			Message: "internal/fetch/client.go differs from 4f2c1a9 in the working tree; the reviewed state is not a commit. " +
				`Commit what was reviewed, or run "git stash create" and resubmit against that SHA — do not retry against this ref.`,
		}},
		Precondition: true,
	}
}

// Mutation guard: verification.unverified must be genuinely absent from the
// wire, never sent even when a Result carries anchors verify reported
// diverged — the field itself is gone, docs/cli.md §5.2, not merely empty
// on the runs that happen not to need it.
func TestUnverifiedIsNeverOnTheWire(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		result *review.Result
	}{
		{name: "nothing diverged", result: clean()},
		{name: "an anchor verify reported diverged", result: &review.Result{
			Valid: true, Comments: 1,
			Verification: review.Verification{Source: "repo", Anchors: 4, Verified: 3, Unverified: []review.Unverified{{
				Name: "anchor-worktree-diverged", Comment: "dropped-context-1",
				Path: "/comments/0/anchors/0", Message: "internal/fetch/client.go differs from 4f2c1a9 in the working tree",
			}}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stdout := &bytes.Buffer{}
			require.NoError(t, NewJSON().Result(stdout, test.result))
			assert.NotContains(t, stdout.String(), "unverified", "omitted, the convention diagnostics and lenses already use")
		})
	}
}

// The precondition's diagnostic is the sole content on the wire: no
// verification.unverified, exactly one diagnostic, and lenses names it.
func TestPreconditionCarriesOneDiagnosticAndNoUnverifiedField(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	require.NoError(t, NewJSON().Result(stdout, precondition()))
	var payload struct {
		Valid       bool `json:"valid"`
		Diagnostics []struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		} `json:"diagnostics"`
		Lenses []string `json:"lenses"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	assert.False(t, payload.Valid)
	require.Len(t, payload.Diagnostics, 1, "one diagnostic for the whole document")
	assert.Equal(t, "anchor-worktree-diverged", payload.Diagnostics[0].Name)
	assert.Equal(t, []string{"anchor-worktree-diverged"}, payload.Lenses)
	assert.NotContains(t, stdout.String(), "unverified")
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

// strict is the field that replaced the deleted status line's silence: under
// --strict a run fails with zero errors, and only this says why.
func TestStrictIsCarriedBesideValid(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	require.NoError(t, NewJSON().Result(stdout, &review.Result{
		Strict:      true,
		Comments:    1,
		Diagnostics: []review.Diagnostic{{Severity: review.SeverityAdvisory, Name: "body-thin", Message: "thin"}},
	}))
	var payload struct {
		Valid  bool `json:"valid"`
		Strict bool `json:"strict"`
		Counts struct {
			Errors     int `json:"errors"`
			Advisories int `json:"advisories"`
		} `json:"counts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	assert.True(t, payload.Strict, "a failing run with no errors is unreadable without this")
	assert.False(t, payload.Valid)
	assert.Zero(t, payload.Counts.Errors)
	assert.Equal(t, 1, payload.Counts.Advisories)
}
