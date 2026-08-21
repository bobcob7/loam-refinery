package collect

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobcob7/loam-refinery/internal/review"
)

// docWithPriorities builds a minimal valid review document JSON string
// carrying one comment per element of priorities, each on its own origin
// id, category "style" so priority-category-convention never enters into
// it. verdict is "approve" when priorities is empty, matching the one
// case review-document.md section 3 permits a comment-free submission.
func docWithPriorities(verdict string, priorities []int) string {
	comments := make([]string, 0, len(priorities))
	for i, p := range priorities {
		comments = append(comments, fmt.Sprintf(
			`{"id":"c-%d","priority":%d,"category":"style","body":"body text long enough to pass","anchors":[],"suggestions":[]}`,
			i+1, p))
	}
	return fmt.Sprintf(`{"version":"1","verdict":"%s","summary":"s","comments":[%s]}`, verdict, strings.Join(comments, ","))
}

// mustParse parses a review document JSON string, failing the test
// immediately if it does not parse — every fixture here is hand-built
// valid JSON, so a parse failure means the fixture itself is broken.
func mustParse(t *testing.T, data string) *review.Document {
	t.Helper()
	doc, err := review.Parse([]byte(data))
	require.NoError(t, err)
	return doc
}

// TestSeverityOf_NoCommentsIsAbsentNotZero proves the one case
// review-document.md section 3 permits — an approve carrying no comments
// — reports Max as nil, never as a zero value that would misread as
// "filed at priority 0", and every band count is zero.
func TestSeverityOf_NoCommentsIsAbsentNotZero(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, docWithPriorities("approve", nil))
	sev := severityOf(doc)
	assert.Nil(t, sev.Max, "a comment-free submission has no maximum")
	assert.Equal(t, Severity{}, sev, "every band count stays zero alongside the nil maximum")
}

// TestSeverityOf_SingleComment proves a submission with exactly one
// comment reports that comment's priority as both the maximum and the
// sole occupant of its band.
func TestSeverityOf_SingleComment(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, docWithPriorities("request_changes", []int{9}))
	sev := severityOf(doc)
	require.NotNil(t, sev.Max)
	assert.Equal(t, 9, *sev.Max)
	assert.Equal(t, Severity{Max: sev.Max, MustFix: 1}, sev)
}

// TestSeverityOf_MultiBandTracksMaxAndEachBand proves a submission whose
// comments span every band reports the true maximum — not the first or
// last comment's priority — and a correct count in each of the four
// bands, unaffected by the order comments appear in.
func TestSeverityOf_MultiBandTracksMaxAndEachBand(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, docWithPriorities("request_changes", []int{2, 10, 5, 8, 1, 7, 4, 9}))
	sev := severityOf(doc)
	require.NotNil(t, sev.Max)
	assert.Equal(t, 10, *sev.Max)
	assert.Equal(t, 2, sev.MustFix, "priorities 10 and 9")
	assert.Equal(t, 2, sev.ShouldFix, "priorities 8 and 7")
	assert.Equal(t, 2, sev.WorthFixing, "priorities 5 and 4")
	assert.Equal(t, 2, sev.Optional, "priorities 2 and 1")
}

// TestBandOf_Boundaries pins the four band boundaries this package reuses
// from docs/review-document.md section 8 — the same scale
// priority-flat and priority-category-convention (internal/advisory)
// reason about — one priority at a time across the full 1-10 scale, plus
// bandOf's own doc comment's promise about the low side of out-of-range:
// a sub-1 priority, which the schema forbids but a lenient parse does
// not exclude, still falls to Optional rather than being dropped
// uncounted. TestSeverityOf_OutOfRangeButWellTypedPriorityStillCounts
// already covers the high side (12); nothing covered the low side before
// this — narrowing the default case to priority >= 1 survived.
func TestBandOf_Boundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		priority int
		want     Severity
	}{
		{priority: -5, want: Severity{Optional: 1}},
		{priority: 0, want: Severity{Optional: 1}},
		{priority: 1, want: Severity{Optional: 1}},
		{priority: 2, want: Severity{Optional: 1}},
		{priority: 3, want: Severity{Optional: 1}},
		{priority: 4, want: Severity{WorthFixing: 1}},
		{priority: 5, want: Severity{WorthFixing: 1}},
		{priority: 6, want: Severity{WorthFixing: 1}},
		{priority: 7, want: Severity{ShouldFix: 1}},
		{priority: 8, want: Severity{ShouldFix: 1}},
		{priority: 9, want: Severity{MustFix: 1}},
		{priority: 10, want: Severity{MustFix: 1}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("priority=%d", tt.priority), func(t *testing.T) {
			t.Parallel()
			var sev Severity
			bandOf(tt.priority, &sev)
			assert.Equal(t, tt.want, sev)
		})
	}
}

// TestSeverityOf_IllTypedPriorityIsExcludedNotZero proves a comment whose
// priority did not parse as an integer — absent entirely, or the wrong
// JSON type — never contributes to Max or any band, rather than
// defaulting to Go's zero value for int and being misread as a real
// priority 0. This mirrors review-document.md section 11.4's table entry
// for a wrong-type priority: skipped by priority-based checks, every
// other comment still counted.
func TestSeverityOf_IllTypedPriorityIsExcludedNotZero(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		doc  string
	}{
		{
			name: "wrong type",
			doc: `{"version":"1","verdict":"comment","summary":"s","comments":[` +
				`{"id":"c-1","priority":"high","category":"style","body":"body text long enough to pass","anchors":[],"suggestions":[]}` +
				`]}`,
		},
		{
			name: "absent",
			doc: `{"version":"1","verdict":"comment","summary":"s","comments":[` +
				`{"id":"c-1","category":"style","body":"body text long enough to pass","anchors":[],"suggestions":[]}` +
				`]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := mustParse(t, tt.doc)
			sev := severityOf(doc)
			assert.Nil(t, sev.Max, "an unusable priority must not stand in as a real maximum")
			assert.Equal(t, Severity{}, sev, "an unusable priority is excluded from every band, not counted as Optional")
		})
	}
}

// TestSeverityOf_IllTypedPriorityDoesNotSuppressOtherComments proves the
// exclusion in TestSeverityOf_IllTypedPriorityIsExcludedNotZero is
// per-comment, not document-wide: a submission with one unusable
// priority alongside a usable one still reports the usable one's
// severity.
func TestSeverityOf_IllTypedPriorityDoesNotSuppressOtherComments(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, `{"version":"1","verdict":"request_changes","summary":"s","comments":[`+
		`{"id":"c-1","priority":"high","category":"style","body":"body text long enough to pass","anchors":[],"suggestions":[]},`+
		`{"id":"c-2","priority":9,"category":"correctness","body":"body text long enough to pass","anchors":[],"suggestions":[]}`+
		`]}`)
	sev := severityOf(doc)
	require.NotNil(t, sev.Max)
	assert.Equal(t, 9, *sev.Max)
	assert.Equal(t, Severity{Max: sev.Max, MustFix: 1}, sev)
}

// TestSeverityOf_OutOfRangeButWellTypedPriorityStillCounts proves a
// priority that is a real integer but outside the 1-10 schema range —
// review-document.md section 11.4's "12 is greater than the maximum of
// 10" example — still runs through band assignment rather than being
// treated as unusable, the same distinction priority-category-convention
// draws between "wrong type" and "out of range".
func TestSeverityOf_OutOfRangeButWellTypedPriorityStillCounts(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, docWithPriorities("request_changes", []int{12}))
	sev := severityOf(doc)
	require.NotNil(t, sev.Max)
	assert.Equal(t, 12, *sev.Max)
	assert.Equal(t, Severity{Max: sev.Max, MustFix: 1}, sev)
}

// TestAssemble_SeverityIsComputedPerSubmission proves Assemble itself
// wires severityOf's result onto each Submission, and that it is
// computed per submission rather than pooled across the whole result:
// two submissions sharing a ref get independent severity shapes.
func TestAssemble_SeverityIsComputedPerSubmission(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		switch digest {
		case "quiet":
			return []byte(docWithPriorities("approve", nil)), nil
		case "loud":
			return []byte(docWithPriorities("request_changes", []int{10, 3})), nil
		default:
			t.Fatalf("unexpected digest %q", digest)
			return nil, nil
		}
	}}
	digests := []DigestRow{{Digest: "quiet", At: at}, {Digest: "loud", At: at.Add(time.Minute)}}
	result, err := Assemble(t.Context(), digests, r)
	require.NoError(t, err)
	require.Len(t, result.Submissions, 2)
	quiet, loud := result.Submissions[0], result.Submissions[1]
	assert.Nil(t, quiet.Severity.Max, "the comment-free submission stays absent")
	assert.Equal(t, Severity{}, quiet.Severity)
	require.NotNil(t, loud.Severity.Max)
	assert.Equal(t, 10, *loud.Severity.Max)
	assert.Equal(t, 1, loud.Severity.MustFix)
	assert.Equal(t, 1, loud.Severity.Optional)
}
