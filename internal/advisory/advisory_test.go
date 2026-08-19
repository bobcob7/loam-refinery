package advisory

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobcob7/refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanDocumentRaisesNothing is the passing case for every advisory: the
// same fixture each failing case is a mutation of.
func TestCleanDocumentRaisesNothing(t *testing.T) {
	t.Parallel()
	diagnostics, skipped := run(t, "clean.json", nil)
	assert.Empty(t, diagnostics)
	assert.Empty(t, skipped)
}

func TestEveryAdvisoryFiresOnItsFixture(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		message string
	}{
		{name: "id-grouping", message: `slug "dropped-context" has suffixes 1, 3; renumber contiguously`},
		{name: "ref-missing", message: "anchor 1 carries line 88 and the document has no ref; nobody can verify it"},
		{name: "body-thin", message: "body is 33 characters; state the finding and what follows from it"},
		{name: "vacuous-body", message: `body ("Consider refactoring.") says nothing a consumer can act on`},
		{name: "suggestion-absent", message: "priority 9 with no suggestions; propose a way out"},
		{name: "suggestion-no-cons", message: "suggestion 1 (\"Pass the caller's context straight throu…\") lists no cons; state the tradeoff or say the fix is free"},
		{name: "suggestion-no-pros"},
		{name: "broad-scope-alone", message: "the only suggestion is scope module; offer a narrower alternative too"},
		{name: "broad-scope-no-cons", message: "suggestion 2 is scope module with no cons; reaching that far always costs something"},
		{name: "summary-thin", message: "summary is 48 characters with 3 comments; expand it"},
		{name: "priority-category-convention", message: "documentation at priority 9 claims the change must not merge"},
		{name: "priority-flat", message: "all 6 comments are priority 7; the scale is not being used"},
		{name: "duplicate-anchor", message: "anchors the same span as dropped-context-1 (internal/fetch/client.go:88-94)"},
		{name: "duplicate-body", message: "body is identical to dropped-context-1"},
		{name: "comment-flood", message: "26 comments; feedback at this volume is not actionable"},
	}
	require.Len(t, tests, len(All()), "every registered advisory needs a fixture")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			diagnostics, _ := run(t, test.name+".json", nil)
			names := []string{}
			messages := map[string]string{}
			for _, diagnostic := range diagnostics {
				names = append(names, diagnostic.Name)
				if _, seen := messages[diagnostic.Name]; !seen {
					messages[diagnostic.Name] = diagnostic.Message
				}
				assert.Equal(t, review.SeverityAdvisory, diagnostic.Severity, "advisories are never hard")
			}
			assert.Contains(t, names, test.name)
			if test.message != "" {
				assert.Equal(t, test.message, messages[test.name])
			}
		})
	}
}

func TestAggregateChecksSkipRatherThanGuess(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		file   string
		want   map[string]string
		absent []string
	}{
		{
			name:   "an ill-typed priority skips the population checks",
			file:   "priority-unusable.json",
			want:   map[string]string{"priority-flat": "1 comment has unusable priority"},
			absent: []string{"comment-flood"},
		},
		{
			name: "a comment that is not an object skips comment-flood",
			file: "comments-ill-typed.json",
			want: map[string]string{"comment-flood": "some comments are not objects"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			diagnostics, skipped := run(t, test.file, nil)
			reasons := map[string]string{}
			for _, skip := range skipped {
				reasons[skip.Name] = skip.Reason
			}
			for name, reason := range test.want {
				assert.Equal(t, reason, reasons[name], "%s should be reported as skipped", name)
			}
			for _, name := range test.absent {
				assert.NotContains(t, reasons, name)
			}
			for _, diagnostic := range diagnostics {
				assert.NotContains(t, test.want, diagnostic.Name, "a skipped check must not also report a finding")
			}
		})
	}
}

func TestDisabledAdvisoriesDoNotRun(t *testing.T) {
	t.Parallel()
	diagnostics, _ := run(t, "duplicate-body.json", map[string]bool{"duplicate-body": true})
	for _, diagnostic := range diagnostics {
		assert.NotEqual(t, "duplicate-body", diagnostic.Name)
	}
}

func TestRegistryHoldsWhatItIsGiven(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	only := []Advisory{}
	for _, advisory := range All() {
		if advisory.Meta.Name == "duplicate-body" {
			only = append(only, advisory)
		}
	}
	require.Len(t, only, 1)
	doc := parse(t, "duplicate-body.json")
	diagnostics, _ := New(log, only).Run(doc, nil)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "duplicate-body", diagnostics[0].Name)
}

func TestEveryAdvisoryIsExplainable(t *testing.T) {
	t.Parallel()
	for _, check := range Checks() {
		assert.NotEmpty(t, check.Title, "%s has no title", check.Name)
		assert.NotEmpty(t, check.Summary, "%s has no one-line summary", check.Name)
		assert.NotEmpty(t, check.Body, "%s has no entry body", check.Name)
		assert.Equal(t, review.TierAdvisory, check.Tier)
	}
}

func run(t *testing.T, file string, disabled map[string]bool) ([]review.Diagnostic, []review.Skipped) {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return New(log, All()).Run(parse(t, file), disabled)
}

func parse(t *testing.T, file string) *review.Document {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)
	doc, err := review.Parse(source)
	require.NoError(t, err)
	return doc
}
