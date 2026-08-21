package collect

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docWithAssessment builds a minimal valid review document JSON string,
// carrying no comments (verdict "approve" is the one case
// review-document.md section 3 permits a comment-free submission), and
// setting "assessment" to raw verbatim — a quoted string when the caller
// passes one, or any other JSON literal (including omission via "") when
// exercising the ill-typed and absent cases.
func docWithAssessment(assessmentJSON string) string {
	if assessmentJSON == "" {
		return `{"version":"1","verdict":"approve","summary":"s","comments":[]}`
	}
	return `{"version":"1","verdict":"approve","summary":"s","assessment":` + assessmentJSON + `,"comments":[]}`
}

// TestAssessmentOf_AbsentFieldIsNilNotEmptyString proves a document that
// never sets "assessment" reports nil, never "" — the same
// absent-is-a-real-state contract SupersededBy and Severity.Max already
// hold: a review that omitted the field is not a review that graded the
// work as some default level, and collapsing the two would silently
// invent an opinion the reviewer declined to give.
func TestAssessmentOf_AbsentFieldIsNilNotEmptyString(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, docWithAssessment(""))
	assert.Nil(t, assessmentOf(doc), "an absent field must report nil, never a zero-value empty string")
}

// TestAssessmentOf_PresentFieldReturnsPointerToValue proves a document
// that sets "assessment" to one of the four documented levels reports a
// non-nil pointer to that exact value.
func TestAssessmentOf_PresentFieldReturnsPointerToValue(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, docWithAssessment(`"sound"`))
	got := assessmentOf(doc)
	require.NotNil(t, got)
	assert.Equal(t, "sound", *got)
}

// TestAssessmentOf_WrongTypeIsNilNotEmptyString proves a well-typed-JSON
// but wrong-kind "assessment" — a number here, not a string — is treated
// the same as absent: nil, not a stringified zero value. This mirrors
// severityOf's own treatment of an ill-typed priority (Field.OK false is
// excluded, never coerced).
func TestAssessmentOf_WrongTypeIsNilNotEmptyString(t *testing.T) {
	t.Parallel()
	doc := mustParse(t, docWithAssessment("7"))
	assert.Nil(t, assessmentOf(doc), "a well-typed-JSON but wrong-kind value must report nil, not a coerced string")
}

// TestAssemble_AssessmentIsCarriedPerSubmission proves Assemble itself
// wires assessmentOf's result onto each Submission, independently per
// submission: one submission that set assessment and one that never did,
// sharing a ref, must not influence each other's result.
func TestAssemble_AssessmentIsCarriedPerSubmission(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		switch digest {
		case "graded":
			return []byte(docWithAssessment(`"strong"`)), nil
		case "ungraded":
			return []byte(docWithAssessment("")), nil
		default:
			t.Fatalf("unexpected digest %q", digest)
			return nil, nil
		}
	}}
	digests := []DigestRow{{Digest: "graded", At: at}, {Digest: "ungraded", At: at.Add(time.Minute)}}
	result, err := Assemble(t.Context(), digests, r)
	require.NoError(t, err)
	require.Len(t, result.Submissions, 2)
	graded, ungraded := result.Submissions[0], result.Submissions[1]
	require.NotNil(t, graded.Assessment)
	assert.Equal(t, "strong", *graded.Assessment)
	assert.Nil(t, ungraded.Assessment, "a submission that never set assessment stays absent, not defaulted")
}
