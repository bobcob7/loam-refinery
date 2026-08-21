package collect

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// combinedReviewsDoc, heading121, and heading122 locate the two worked
// examples this file reproduces: docs/features/combined-reviews.md §12.1
// and §12.2.
const combinedReviewsDoc = "../../docs/features/combined-reviews.md"

const heading121 = "### 12.1 Two profiles, one ref"

const heading122 = "### 12.2 One profile, revised, plus one unprofiled submission"

// backtickRun returns the number of leading backtick characters in s.
func backtickRun(s string) int {
	n := 0
	for _, r := range s {
		if r != '`' {
			break
		}
		n++
	}
	return n
}

// docSectionEnd returns the offset, within text, of the next heading line
// (any level) following the heading line found at anchorPos, ignoring any
// line inside a fenced code block — a fence can contain a line that merely
// looks like a heading, and CommonMark never reads it as one. A minimal,
// package-local mirror of internal/cli/docs_shape_test.go's own
// docSectionEnd (refinery-xlp.9): this package does not import
// internal/cli, so the technique is duplicated here rather than shared,
// but the property it enforces is the same one that helper protects — a
// search for a fenced block must never roll past the section it was asked
// about into whatever comes next.
func docSectionEnd(text string, anchorPos int) int {
	lines := strings.SplitAfter(text[anchorPos:], "\n")
	pos := anchorPos
	fenceLen := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		run := backtickRun(trimmed)
		switch {
		case fenceLen == 0 && run >= 3:
			fenceLen = run
		case fenceLen > 0 && run >= fenceLen && run == len(trimmed):
			fenceLen = 0
		case fenceLen == 0 && i > 0 && strings.HasPrefix(trimmed, "#"):
			return pos
		}
		pos += len(line)
	}
	return len(text)
}

// findFenceClose scans lines starting at offset from in text for the first
// line whose trimmed content is entirely backticks, at least closeLen of
// them — CommonMark's actual fence-closing rule, matched by line shape
// rather than by the first bare "```" substring anywhere in the content,
// which a fenced excerpt containing its own embedded backticks could
// otherwise close early.
func findFenceClose(text string, from, closeLen int) (blockEnd, nextPos int) {
	lines := strings.SplitAfter(text[from:], "\n")
	pos := from
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && len(trimmed) >= closeLen && strings.Trim(trimmed, "`") == "" {
			return pos, pos + len(line)
		}
		pos += len(line)
	}
	return -1, -1
}

// docFencedJSON returns the occurrence-th fenced ```json block that
// appears after anchor, and before the next heading, in the document at
// path, as raw trimmed text — read fresh every run rather than copied into
// a Go literal (refinery-xlp.9), so editing the doc's own displayed
// example changes what this test feeds Assemble, and a docs edit that
// drifts from the documented output fails here instead of nothing
// noticing.
func docFencedJSON(t *testing.T, path, anchor string, occurrence int) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	at := strings.Index(text, anchor)
	require.GreaterOrEqualf(t, at, 0, "anchor %q not found in %s", anchor, path)
	rest := text[at:docSectionEnd(text, at)]
	const fence = "```json"
	pos := 0
	for n := 1; ; n++ {
		open := strings.Index(rest[pos:], fence)
		require.GreaterOrEqualf(t, open, 0, "json block #%d after %q not found before the next heading in %s", occurrence, anchor, path)
		openLineEnd := pos + open + len(fence)
		if nl := strings.IndexByte(rest[openLineEnd:], '\n'); nl >= 0 {
			openLineEnd += nl + 1
		} else {
			openLineEnd = len(rest)
		}
		blockEnd, nextPos := findFenceClose(rest, openLineEnd, backtickRun(fence))
		require.GreaterOrEqualf(t, blockEnd, 0, "unterminated json block after %q in %s", anchor, path)
		block := strings.TrimSpace(rest[openLineEnd:blockEnd])
		pos = nextPos
		if n == occurrence {
			return block
		}
	}
}

// submissionA and submissionB are docs/features/combined-reviews.md
// §12.1's two stored documents, read from the file rather than copied
// verbatim into a literal — occurrence 1 is Submission A, occurrence 2 is
// Submission B.
func submissionA(t *testing.T) string { return docFencedJSON(t, combinedReviewsDoc, heading121, 1) }

func submissionB(t *testing.T) string { return docFencedJSON(t, combinedReviewsDoc, heading121, 2) }

// TestAssemble_WorkedExample_TwoProfilesOneRef reproduces
// docs/features/combined-reviews.md section 12.1: two current submissions
// under different profiles land on the same slug, dropped-context-1, and
// neither is fused nor ordinal-qualified, because both are current for
// their own profile.
//
// Mutation this kills: grouping by slug instead of by profile would fuse
// these two comments into one entry, or drop one on a priority
// comparison; testing qualifier-by-profile (rather than by digest or
// origin id) is what a "group by slug" regression would fail.
func TestAssemble_WorkedExample_TwoProfilesOneRef(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		switch digest {
		case "digest-a":
			return []byte(submissionA(t)), nil
		case "digest-b":
			return []byte(submissionB(t)), nil
		default:
			t.Fatalf("unexpected digest %q", digest)
			return nil, nil
		}
	}}
	digests := []DigestRow{
		{Digest: "digest-a", At: at},
		{Digest: "digest-b", At: at.Add(11 * time.Minute)},
	}
	result, err := Assemble(t.Context(), digests, r)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Unreadable)
	require.Len(t, result.Submissions, 2)
	backend := result.Submissions[0]
	assert.Equal(t, 1, backend.Ordinal)
	assert.Equal(t, "backend", backend.Profile)
	assert.Equal(t, "request_changes", backend.Verdict)
	assert.Equal(t, "The retry loop is sound, but the context deadline is not propagated to the downstream call.", backend.Summary)
	assert.Nil(t, backend.SupersededBy, "a lone submission under a profile is current")
	security := result.Submissions[1]
	assert.Equal(t, 2, security.Ordinal)
	assert.Equal(t, "security", security.Profile)
	assert.Equal(t, "comment", security.Verdict)
	assert.Nil(t, security.SupersededBy)
	require.Len(t, result.Comments, 2)
	byID := commentsByID(result.Comments)
	backendComment, ok := byID["backend:dropped-context-1"]
	require.True(t, ok, "backend's comment must be profile-qualified, not ordinal-qualified, since it is current")
	assert.Equal(t, "backend", backendComment.Profile)
	assert.Equal(t, 9, backendComment.Priority)
	assert.Equal(t, "correctness", backendComment.Category)
	require.Len(t, backendComment.Anchors, 1)
	assert.Equal(t, "internal/fetch/client.go", backendComment.Anchors[0].File)
	assert.Equal(t, 88, backendComment.Anchors[0].Line)
	require.NotNil(t, backendComment.Anchors[0].EndLine)
	assert.Equal(t, 94, *backendComment.Anchors[0].EndLine)
	securityComment, ok := byID["security:dropped-context-1"]
	require.True(t, ok, "security's comment must be profile-qualified too — same slug, different profile, never fused")
	assert.Equal(t, "security", securityComment.Profile)
	assert.Equal(t, 3, securityComment.Priority)
	assert.Nil(t, securityComment.Anchors[0].EndLine, "security's anchor never set end_line")
	assert.Equal(t, []string{"backend:dropped-context-1", "security:dropped-context-1"}, idsInOrder(result.Comments))
	assert.Equal(t, backend.Document.Verdict.Value, "request_changes", "Submission.Document carries the full parsed review, not just the rendered fields")
	assert.Equal(t, "dropped-context-1", func() string {
		for id, qualified := range backend.QualifiedIDs {
			if qualified == "backend:dropped-context-1" {
				return id
			}
		}
		return ""
	}(), "QualifiedIDs maps the submission's own origin id to the id this package assigned it")
}

// submissionD1, submissionD2, and submissionD3 are docs/features/
// combined-reviews.md §12.2's three stored documents — D1, D2, D3 in the
// spec's own labels — read from the file rather than copied verbatim into
// a literal. Occurrences 1-3 within §12.2 are D1, D2, and D3; occurrence 4
// is the documented output.
func submissionD1(t *testing.T) string { return docFencedJSON(t, combinedReviewsDoc, heading122, 1) }

func submissionD2(t *testing.T) string { return docFencedJSON(t, combinedReviewsDoc, heading122, 2) }

func submissionD3(t *testing.T) string { return docFencedJSON(t, combinedReviewsDoc, heading122, 3) }

// TestAssemble_WorkedExample_RevisedAndUnprofiled reproduces
// docs/features/combined-reviews.md section 12.2: one profile revised
// its own finding, plus one unprofiled submission. This is the named
// three-way test the bead's acceptance criteria calls for: a comment
// that is current and profiled, one that is superseded and yet still
// profiled, and one that never claimed a profile at all — collapsing any
// two of these three must turn this test red.
func TestAssemble_WorkedExample_RevisedAndUnprofiled(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		switch digest {
		case "d1":
			return []byte(submissionD1(t)), nil
		case "d2":
			return []byte(submissionD2(t)), nil
		case "d3":
			return []byte(submissionD3(t)), nil
		default:
			t.Fatalf("unexpected digest %q", digest)
			return nil, nil
		}
	}}
	digests := []DigestRow{
		{Digest: "d1", At: at},
		{Digest: "d2", At: at.Add(time.Hour)},
		{Digest: "d3", At: at.Add(2 * time.Hour)},
	}
	result, err := Assemble(t.Context(), digests, r)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Unreadable)
	require.Len(t, result.Submissions, 3)
	d1, d2, d3 := result.Submissions[0], result.Submissions[1], result.Submissions[2]
	assert.Equal(t, 1, d1.Ordinal)
	assert.Equal(t, "backend", d1.Profile)
	require.NotNil(t, d1.SupersededBy, "D1 is superseded by D2 within the backend profile")
	assert.Equal(t, 2, *d1.SupersededBy)
	assert.Equal(t, 2, d2.Ordinal)
	assert.Equal(t, "backend", d2.Profile)
	assert.Nil(t, d2.SupersededBy, "D2 is the current backend submission")
	assert.Equal(t, 3, d3.Ordinal)
	assert.Equal(t, "", d3.Profile, "D3 never claimed a profile")
	assert.Nil(t, d3.SupersededBy, "an unprofiled submission has no supersession axis")
	byID := commentsByID(result.Comments)
	current, ok := byID["#1:stale-cache-1"]
	require.True(t, ok, "D1's comment is ordinal-qualified because its submission is superseded")
	assert.Equal(t, "backend", current.Profile, "superseded-but-profiled: profile survives supersession")
	assert.Equal(t, 6, current.Priority)
	revised, ok := byID["backend:stale-cache-1"]
	require.True(t, ok, "D2's revision keeps the origin id but is now profile-qualified, since D2 is current")
	assert.Equal(t, "backend", revised.Profile, "current-and-profiled")
	assert.Equal(t, 8, revised.Priority)
	sibling, ok := byID["backend:missing-invalidation-1"]
	require.True(t, ok)
	assert.Equal(t, "backend", sibling.Profile)
	never, ok := byID["#3:todo-left-in-1"]
	require.True(t, ok, "D3's comment is ordinal-qualified since it has no profile")
	assert.Empty(t, never.Profile, "never-profiled: no profile field at all, not merely an empty one masking supersession")
	assert.Equal(t, 4, len(result.Comments), "stale-cache-1 legitimately appears twice, under two different qualified ids")
	assert.Equal(t,
		[]string{"#1:stale-cache-1", "#3:todo-left-in-1", "backend:missing-invalidation-1", "backend:stale-cache-1"},
		idsInOrder(result.Comments),
		"comments are ordered by their own qualified id, lexicographically (section 8.1)",
	)
}

func commentsByID(comments []Comment) map[string]Comment {
	out := make(map[string]Comment, len(comments))
	for _, c := range comments {
		out[c.ID] = c
	}
	return out
}

func idsInOrder(comments []Comment) []string {
	ids := make([]string, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.ID)
	}
	return ids
}
