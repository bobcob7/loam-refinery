package collect

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// submissionA and submissionB are docs/features/combined-reviews.md
// section 12.1's two stored documents, copied verbatim.
const submissionA = `{
  "version": "1",
  "verdict": "request_changes",
  "profile": "backend",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "The retry loop is sound, but the context deadline is not propagated to the downstream call.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 9,
      "category": "correctness",
      "body": "The retry loop calls c.do(context.Background(), req) rather than passing the caller's ctx.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 88, "end_line": 94 }
      ],
      "suggestions": [
        {
          "summary": "Pass the caller's context straight through to c.do",
          "effort": "trivial",
          "scope": "line",
          "pros": ["Cancellation and deadlines propagate immediately"],
          "cons": ["A caller relying on retries outliving the request context sees a behavior change"]
        }
      ]
    }
  ]
}`

const submissionB = `{
  "version": "1",
  "verdict": "comment",
  "profile": "security",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "One low-severity logging concern; nothing blocking.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 3,
      "category": "security",
      "body": "The retry loop's debug log includes req.Header verbatim, which can carry an Authorization value on a retried request.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 82 }
      ],
      "suggestions": [
        {
          "summary": "Redact known-sensitive headers before logging the request",
          "effort": "small",
          "scope": "file",
          "pros": ["Removes the leak at the one place it can happen"],
          "cons": ["A future header added to the allowlist could reopen this silently"]
        }
      ]
    }
  ]
}`

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
			return []byte(submissionA), nil
		case "digest-b":
			return []byte(submissionB), nil
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

// docs/features/combined-reviews.md section 12.2's three documents,
// referred to as D1, D2, D3 in the spec. Suggestion content is invented
// where the spec elides it with "/* … */" — only the fields the spec
// pins (ids, profile, priority, supersession, ordering) are asserted.
const submissionD1 = `{
  "version": "1",
  "verdict": "request_changes",
  "profile": "backend",
  "ref": "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a",
  "summary": "Cache invalidation is missing on the write path.",
  "comments": [
    {
      "id": "stale-cache-1",
      "priority": 6,
      "category": "correctness",
      "body": "The write path updates the DB but never invalidates the cache entry.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 41 }],
      "suggestions": [
        { "summary": "Invalidate the cache entry after the write", "effort": "small", "scope": "function", "pros": ["Closes the gap"], "cons": ["Adds one more call on the write path"] }
      ]
    }
  ]
}`

const submissionD2 = `{
  "version": "1",
  "verdict": "request_changes",
  "profile": "backend",
  "ref": "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a",
  "summary": "Cache invalidation is missing on two write paths, not one.",
  "comments": [
    {
      "id": "stale-cache-1",
      "priority": 8,
      "category": "correctness",
      "body": "The write path updates the DB but never invalidates the cache entry; on closer read, the batch-update path has the same gap.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 41 }],
      "suggestions": [
        { "summary": "Invalidate the cache entry after every write path", "effort": "small", "scope": "function", "pros": ["Closes the gap on both paths"], "cons": ["Adds one more call on both write paths"] }
      ]
    },
    {
      "id": "missing-invalidation-1",
      "priority": 8,
      "category": "correctness",
      "body": "Same gap as stale-cache-1, on the batch-update path.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 96 }],
      "suggestions": [
        { "summary": "Invalidate within the batch-update transaction", "effort": "small", "scope": "function", "pros": ["Keeps both paths consistent"], "cons": ["Couples the batch path to the cache layer"] }
      ]
    }
  ]
}`

const submissionD3 = `{
  "version": "1",
  "verdict": "comment",
  "ref": "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a",
  "summary": "One stray TODO; nothing blocking.",
  "comments": [
    {
      "id": "todo-left-in-1",
      "priority": 2,
      "category": "style",
      "body": "A TODO from the previous change is still here and looks resolved by this one.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 12 }],
      "suggestions": []
    }
  ]
}`

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
			return []byte(submissionD1), nil
		case "d2":
			return []byte(submissionD2), nil
		case "d3":
			return []byte(submissionD3), nil
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
