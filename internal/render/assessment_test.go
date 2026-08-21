package render

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONCollectReviews_AssessmentAbsentOmitsKey proves a submission
// whose document never set assessment renders with no "assessment" key
// at all in the JSON envelope — the same omitempty treatment
// superseded_by and severity.max already get, and the one rendering that
// can never be misread as "the reviewer graded this at some level"
// (docs/features/combined-reviews.md §8.1).
func TestJSONCollectReviews_AssessmentAbsentOmitsKey(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Verdict: "comment", Summary: "s"}},
	}
	var out bytes.Buffer
	require.NoError(t, NewJSON().CollectReviews(&out, envelope))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	submissions, ok := decoded["submissions"].([]any)
	require.True(t, ok)
	require.Len(t, submissions, 1)
	submission, ok := submissions[0].(map[string]any)
	require.True(t, ok)
	_, present := submission["assessment"]
	assert.False(t, present, "an absent assessment must omit the key entirely, never render null or a default level")
}

// TestJSONCollectReviews_AssessmentPresentIncludesValue proves a
// submission whose document set assessment renders the exact value under
// the "assessment" key.
func TestJSONCollectReviews_AssessmentPresentIncludesValue(t *testing.T) {
	t.Parallel()
	strong := "strong"
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Verdict: "approve", Summary: "s", Assessment: &strong}},
	}
	var out bytes.Buffer
	require.NoError(t, NewJSON().CollectReviews(&out, envelope))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	submissions, ok := decoded["submissions"].([]any)
	require.True(t, ok)
	require.Len(t, submissions, 1)
	submission, ok := submissions[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "strong", submission["assessment"])
}

// TestMarkdownSubmissions_AssessmentAbsentRendersNoneNotAGrade proves the
// markdown submission line marks an absent assessment visibly, as
// "(none)" — the same placeholder profile already uses for "claimed
// none" — never silently dropping the clause and never rendering one of
// the four real grade words, either of which a reader could mistake for
// an actual opinion the reviewer never gave.
func TestMarkdownSubmissions_AssessmentAbsentRendersNoneNotAGrade(t *testing.T) {
	t.Parallel()
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "comment", Summary: "s"}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	assert.Contains(t, rendered, "assessment: (none)", "an absent assessment must render a visible, unambiguous absence marker")
	for _, level := range []string{"strong", "sound", "mixed", "weak"} {
		assert.NotContains(t, rendered, "assessment: "+level, "an absent assessment must never render as though it were a real grade")
	}
}

// TestMarkdownSubmissions_AssessmentPresentRendersTheGradeUnescaped
// proves a submission whose document set assessment renders that exact
// word, unescaped — assessment is structurally-constrained per §8.3.2,
// the same treatment verdict gets, since a stored document's assessment
// was already validated against the closed four-value enum before it
// reached the store.
func TestMarkdownSubmissions_AssessmentPresentRendersTheGradeUnescaped(t *testing.T) {
	t.Parallel()
	mixed := "mixed"
	envelope := docs121Envelope()
	envelope.Result = &collect.Result{
		Submissions: []collect.Submission{{Ordinal: 1, Profile: "backend", Verdict: "approve", Summary: "s", Assessment: &mixed}},
	}
	var out bytes.Buffer
	require.NoError(t, NewMarkdown().CollectReviews(&out, envelope))
	rendered := out.String()
	assert.Contains(t, rendered, "assessment: mixed")
}
