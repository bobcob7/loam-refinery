package advisory

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanDocumentRaisesNothing is the passing case for every advisory: the
// same fixture each failing case is a mutation of.
func TestCleanDocumentRaisesNothing(t *testing.T) {
	t.Parallel()
	diagnostics, skipped := run(t, "clean.json")
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
		{name: "assessment-priority-mismatch", message: "assessment is strong but dropped-context-1 is filed at priority 9; strong claims nothing serious was found"},
		{name: "duplicate-anchor", message: "anchors the same span as dropped-context-1 (internal/fetch/client.go:88-94)"},
		{name: "duplicate-body", message: "body is identical to dropped-context-1"},
		{name: "comment-flood", message: "26 comments; feedback at this volume is not actionable"},
	}
	require.Len(t, tests, len(All()), "every registered advisory needs a fixture")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			diagnostics, _ := run(t, test.name+".json")
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
			diagnostics, skipped := run(t, test.file)
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
	diagnostics, _ := New(log, only).Run(doc)
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

func run(t *testing.T, file string) ([]review.Diagnostic, []review.Skipped) {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return New(log, All()).Run(parse(t, file))
}

func parse(t *testing.T, file string) *review.Document {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)
	doc, err := review.Parse(source)
	require.NoError(t, err)
	return doc
}

func TestVacuousBodyJudgesEveryClause(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		body  string
		fires bool
	}{
		"fillers joined to clear the floor": {"Looks good to me. LGTM.", true},
		"one stock phrase":                  {"Consider refactoring", true},
		"a hedge in front of a finding":     {"Looks good overall, but the retry loop drops the caller's deadline.", false},
		"a real finding":                    {"c.do is called with context.Background(), so a cancelled request keeps retrying.", false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := build(t, map[string]any{"body": test.body})
			diagnostics, _ := vacuousBody(doc)
			assert.Equal(t, test.fires, len(diagnostics) == 1)
		})
	}
}

func TestFileLevelAnchorsAreNotDuplicateSpans(t *testing.T) {
	t.Parallel()
	doc := build(t,
		map[string]any{"id": "a-1", "anchors": []any{map[string]any{"file": "a.go"}}},
		map[string]any{"id": "b-1", "anchors": []any{map[string]any{"file": "a.go"}}},
	)
	diagnostics, _ := duplicateAnchor(doc)
	assert.Empty(t, diagnostics, "file-level anchors sharing a file must not earn an advisory")
}

func TestBodyThinCatchesAWhitespaceOnlyBody(t *testing.T) {
	t.Parallel()
	doc := build(t, map[string]any{"body": strings.Repeat(" ", 24)})
	diagnostics, _ := bodyThin(doc)
	assert.Len(t, diagnostics, 1, "a body that normalizes to nothing is the thinnest body there is")
}

// build assembles a document in memory so a test can state the one shape it is
// about, rather than adding a fixture file per case.
func build(t *testing.T, comments ...map[string]any) *review.Document {
	t.Helper()
	source, err := json.Marshal(map[string]any{
		"version": "1", "verdict": "comment", "comments": comments,
	})
	require.NoError(t, err)
	doc, err := review.Parse(source)
	require.NoError(t, err)
	return doc
}

// TestAssessmentPriorityMismatchFixturesFireBothDirections proves the two
// on-disk fixtures fire: the strong direction already covered by
// TestEveryAdvisoryFiresOnItsFixture, and the weak direction, which needs a
// second fixture because the table above pins exactly one file per advisory.
func TestAssessmentPriorityMismatchFixturesFireBothDirections(t *testing.T) {
	t.Parallel()
	diagnostics, skipped := run(t, "assessment-priority-mismatch-weak.json")
	assert.Empty(t, skipped)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "assessment-priority-mismatch", diagnostics[0].Name)
	assert.Equal(t,
		"assessment is weak but the highest filed priority is 2, no higher than the optional band; weak claims significant concerns",
		diagnostics[0].Message)
}

// TestAssessmentPriorityMismatchSoundFixturesFireBothPolarities proves sound
// gets exactly strong's floor — strong inherits it from sound by the ordinal
// rule, not the other way around: a priority-9 finding fires the same as
// strong would, naming "sound" rather than a hardcoded "strong" in the
// message, and a review whose comments sit entirely in the optional band —
// sound's own documented anchor (docs/review-document.md section 3) — stays
// silent.
func TestAssessmentPriorityMismatchSoundFixturesFireBothPolarities(t *testing.T) {
	t.Parallel()
	diagnostics, skipped := run(t, "assessment-priority-mismatch-sound.json")
	assert.Empty(t, skipped)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "assessment-priority-mismatch", diagnostics[0].Name)
	assert.Equal(t,
		"assessment is sound but dropped-context-1 is filed at priority 9; sound claims nothing serious was found",
		diagnostics[0].Message)

	diagnostics, skipped = run(t, "assessment-priority-mismatch-sound-silent.json")
	assert.Empty(t, skipped)
	for _, d := range diagnostics {
		assert.NotEqual(t, "assessment-priority-mismatch", d.Name)
	}
}

// TestAssessmentPriorityMismatchStaysSilentOnCoherentWeakPlusComment proves
// weak paired with comment is coherent and never flagged here when the
// priorities genuinely back the grade: docs/review-document.md section 3
// gives exactly this combination — serious concerns from a reviewer whose
// lens does not cover whether the change merges — as coherent.
func TestAssessmentPriorityMismatchStaysSilentOnCoherentWeakPlusComment(t *testing.T) {
	t.Parallel()
	diagnostics, skipped := run(t, "assessment-priority-mismatch-silent.json")
	assert.Empty(t, skipped)
	for _, d := range diagnostics {
		assert.NotEqual(t, "assessment-priority-mismatch", d.Name)
	}
}

// TestAssessmentPriorityMismatch covers the boundaries and the cases that
// must stay silent regardless of what the fixtures above show: the check
// never reasons about verdict, strong and sound share one priority floor —
// sound's own anchor sets it at 4, strong inherits it by the ordinal rule
// (docs/review-document.md section 3) rather than getting a seriousness
// claim of its own — mixed carries none at all, and an absent assessment
// has made no claim to be inconsistent with.
func TestAssessmentPriorityMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		verdict    string
		assessment string
		priorities []int
		fires      bool
	}{
		{name: "strong with a must-fix finding fires", verdict: "request_changes", assessment: "strong", priorities: []int{9}, fires: true},
		{name: "strong with a should-fix finding fires", verdict: "comment", assessment: "strong", priorities: []int{2, 7}, fires: true},
		{name: "strong with a worth-fixing finding fires — inherits sound's floor by the ordinal rule", verdict: "comment", assessment: "strong", priorities: []int{5, 6}, fires: true},
		{name: "strong at the floor (priority 4) fires", verdict: "comment", assessment: "strong", priorities: []int{4}, fires: true},
		{name: "strong with only optional findings stays silent", verdict: "comment", assessment: "strong", priorities: []int{1, 3}, fires: false},
		{name: "sound with a must-fix finding fires — the default gets strong's test", verdict: "comment", assessment: "sound", priorities: []int{9}, fires: true},
		{name: "sound with a should-fix finding fires", verdict: "comment", assessment: "sound", priorities: []int{2, 7}, fires: true},
		{name: "sound with a worth-fixing finding fires — the case its own anchor rules out", verdict: "comment", assessment: "sound", priorities: []int{6}, fires: true},
		{name: "sound at the floor (priority 4) fires", verdict: "comment", assessment: "sound", priorities: []int{4}, fires: true},
		{name: "sound with only optional findings stays silent — its own documented anchor", verdict: "approve", assessment: "sound", priorities: []int{1, 3}, fires: false},
		{name: "sound with no comments stays silent", verdict: "approve", assessment: "sound", priorities: nil, fires: false},
		{name: "weak with no comments fires", verdict: "comment", assessment: "weak", priorities: nil, fires: true},
		{name: "weak with only optional findings fires", verdict: "comment", assessment: "weak", priorities: []int{1, 3}, fires: true},
		{name: "weak with a worth-fixing finding stays silent", verdict: "comment", assessment: "weak", priorities: []int{4}, fires: false},
		{name: "mixed with approve stays silent — coherent mixed+approve", verdict: "approve", assessment: "mixed", priorities: nil, fires: false},
		{name: "mixed never fires even with a serious finding — its anchor names no priority claim", verdict: "comment", assessment: "mixed", priorities: []int{9}, fires: false},
		{name: "absent assessment never fires", verdict: "request_changes", assessment: "", priorities: []int{9}, fires: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc := buildWithAssessment(t, test.verdict, test.assessment, test.priorities...)
			diagnostics, skipped := assessmentPriorityMismatch(doc)
			assert.Empty(t, skipped)
			assert.Equal(t, test.fires, len(diagnostics) == 1)
		})
	}
}

// TestAssessmentPriorityMismatchSkipsOnUnusablePriority proves the check
// reports itself skipped, rather than guessing, when a comment's priority
// did not parse — the same aggregate-check discipline priority-flat follows,
// because a claim that "nothing above the optional band was filed" cannot be
// trusted when one comment's priority is unknown.
func TestAssessmentPriorityMismatchSkipsOnUnusablePriority(t *testing.T) {
	t.Parallel()
	source := `{"version":"1","verdict":"comment","assessment":"weak","comments":[` +
		`{"id":"c-1","priority":"high","category":"style","body":"body text long enough to say something","anchors":[],"suggestions":[]}` +
		`]}`
	doc, err := review.Parse([]byte(source))
	require.NoError(t, err)
	diagnostics, skipped := assessmentPriorityMismatch(doc)
	assert.Empty(t, diagnostics)
	require.Len(t, skipped, 1)
	assert.Equal(t, "assessment-priority-mismatch", skipped[0].Name)
	assert.Equal(t, "1 comment has unusable priority", skipped[0].Reason)
}

// buildWithAssessment assembles a document in memory carrying a top-level
// assessment (omitted from the object entirely when empty, exercising the
// absent-field path the same way a real document without the key would) and
// one comment per element of priorities, each in category "style" so
// priority-category-convention never enters into it.
func buildWithAssessment(t *testing.T, verdict, assessment string, priorities ...int) *review.Document {
	t.Helper()
	comments := make([]map[string]any, 0, len(priorities))
	for i, p := range priorities {
		comments = append(comments, map[string]any{
			"id": fmt.Sprintf("c-%d", i+1), "priority": p, "category": "style",
			"body": "body text long enough to say something", "anchors": []any{}, "suggestions": []any{},
		})
	}
	obj := map[string]any{"version": "1", "verdict": verdict, "comments": comments}
	if assessment != "" {
		obj["assessment"] = assessment
	}
	source, err := json.Marshal(obj)
	require.NoError(t, err)
	doc, err := review.Parse(source)
	require.NoError(t, err)
	return doc
}
