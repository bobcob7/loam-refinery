package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRejectsAnythingButOneObject(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"not json":        "nonsense",
		"an array":        `[{"version":"1"}]`,
		"a bare string":   `"review"`,
		"two documents":   `{"version":"1"} {"version":"1"}`,
		"an empty input":  ``,
		"jsonl documents": "{\"version\":\"1\"}\n{\"version\":\"1\"}\n",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc, err := Parse([]byte(source))
			require.Error(t, err)
			assert.Nil(t, doc)
		})
	}
}

func TestParseKeepsIllTypedFieldsUsable(t *testing.T) {
	t.Parallel()
	source := `{
		"version": 1,
		"verdict": "comment",
		"summary": "fine",
		"comments": [
			{"id": "a-1", "priority": "high", "category": "style", "body": "b",
			 "anchors": [{"file": "a.go", "line": 3}], "suggestions": []},
			"not an object",
			{"id": "b-1", "priority": 4, "anchors": [], "suggestions": [{"pros": ["x"], "cons": "no"}]}
		]
	}`
	doc, err := Parse([]byte(source))
	require.NoError(t, err)
	assert.False(t, doc.Version.OK, "a numeric version is present but ill-typed")
	assert.True(t, doc.Version.Present)
	assert.True(t, doc.Verdict.OK)
	assert.Len(t, doc.Comments, 3)
	assert.True(t, doc.CommentsArray)
	assert.False(t, doc.CommentsWellTyped, "one element is not an object")
	assert.False(t, doc.Comments[0].Priority.OK, "a string priority is unusable")
	assert.True(t, doc.Comments[0].Priority.Present)
	assert.False(t, doc.Comments[1].Object)
	assert.Equal(t, 4, doc.Comments[2].Priority.Value)
	assert.True(t, doc.Comments[2].Suggestions[0].Pros.OK)
	assert.False(t, doc.Comments[2].Suggestions[0].Cons.OK, "a string cons list is unusable")
	assert.Equal(t, "/comments/0/anchors/0", doc.Comments[0].Anchors[0].Path)
}

func TestParseReadsProfileField(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		source  string
		present bool
		ok      bool
		value   string
	}{
		"omitted":                {`{"version":"1"}`, false, false, ""},
		"present and well-typed": {`{"profile":"backend"}`, true, true, "backend"},
		"present but empty":      {`{"profile":""}`, true, true, ""},
		"present but wrong type": {`{"profile":123}`, true, false, ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc, err := Parse([]byte(test.source))
			require.NoError(t, err)
			assert.Equal(t, test.present, doc.Profile.Present)
			assert.Equal(t, test.ok, doc.Profile.OK)
			assert.Equal(t, test.value, doc.Profile.Value)
		})
	}
}

func TestSortDiagnosticsPutsErrorsFirstThenDocumentOrder(t *testing.T) {
	t.Parallel()
	diagnostics := []Diagnostic{
		{Severity: SeverityAdvisory, Name: "late-advisory", Path: "/comments/3/body"},
		{Severity: SeverityError, Name: "late-error", Path: "/comments/2"},
		{Severity: SeverityAdvisory, Name: "document-advisory"},
		{Severity: SeverityError, Name: "early-error", Path: "/comments/0/id"},
	}
	SortDiagnostics(diagnostics)
	names := []string{}
	for _, d := range diagnostics {
		names = append(names, d.Name)
	}
	assert.Equal(t, []string{"early-error", "late-error", "document-advisory", "late-advisory"}, names)
}

func TestLensesDeduplicatesInDiagnosticOrder(t *testing.T) {
	t.Parallel()
	result := &Result{Diagnostics: []Diagnostic{
		{Name: "schema", Lens: "priority"},
		{Name: "id-unique"},
		{Name: "schema", Lens: "priority"},
	}}
	assert.Equal(t, []string{"priority", "id-unique"}, result.Lenses())
}

// An unverified anchor is not a diagnostic, but a caller still needs to open
// its check name the same way — docs/cli.md §5.2 says lenses covers "the
// diagnostics and any unverified anchors".
func TestLensesIncludesUnverifiedAnchors(t *testing.T) {
	t.Parallel()
	result := &Result{
		Diagnostics: []Diagnostic{{Name: "schema", Lens: "priority"}},
		Verification: Verification{Unverified: []Unverified{
			{Name: "anchor-worktree-diverged"},
			{Name: "anchor-worktree-diverged"},
		}},
	}
	assert.Equal(t, []string{"priority", "anchor-worktree-diverged"}, result.Lenses(),
		"the check name is deduplicated the same way a diagnostic's is")
}

func TestParseAcceptsEveryIntegralSpellingOfANumber(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		literal string
		want    int
		ok      bool
	}{
		"a plain integer":     {"100", 100, true},
		"a zero fraction":     {"100.0", 100, true},
		"an exponent":         {"1e2", 100, true},
		"a negative exponent": {"1000e-1", 100, true},
		"a real fraction":     {"100.5", 0, false},
		"beyond exact range":  {"1e300", 0, false},
		"a string":            {`"100"`, 0, false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := `{"comments":[{"anchors":[{"file":"a.go","line":` + test.literal + `}]}]}`
			doc, err := Parse([]byte(source))
			require.NoError(t, err)
			require.Len(t, doc.Comments, 1)
			require.Len(t, doc.Comments[0].Anchors, 1)
			line := doc.Comments[0].Anchors[0].Line
			assert.True(t, line.Present)
			assert.Equal(t, test.ok, line.OK, "JSON Schema calls any zero-fraction number an integer")
			assert.Equal(t, test.want, line.Value)
		})
	}
}
