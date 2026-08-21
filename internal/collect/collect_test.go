package collect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimalDoc(profile, id string) string {
	if profile == "" {
		return `{"version":"1","verdict":"comment","summary":"s","comments":[{"id":"` + id + `","priority":1,"category":"style","body":"body text long enough to pass","anchors":[],"suggestions":[]}]}`
	}
	return `{"version":"1","verdict":"comment","profile":"` + profile + `","summary":"s","comments":[{"id":"` + id + `","priority":1,"category":"style","body":"body text long enough to pass","anchors":[],"suggestions":[]}]}`
}

// TestAssemble_EqualAtTieBreaksOnDigestNotInputOrder proves section
// 5.3.3's tie-break: two submissions under the same profile with an
// identical at resolve which one is "current" by comparing digest
// strings, and that resolution does not depend on the order the caller
// happened to pass them in.
//
// Mutation this kills: dropping the digest comparator from sortedDigests
// (leaving only the at comparison) makes sort.Slice's result depend on Go's
// unspecified pivot choice for equal elements — this test's two calls with
// reversed input order would then be free to disagree.
func TestAssemble_EqualAtTieBreaksOnDigestNotInputOrder(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		switch digest {
		case "aaa":
			return []byte(minimalDoc("x", "finding-1")), nil
		case "zzz":
			return []byte(minimalDoc("x", "finding-1")), nil
		default:
			t.Fatalf("unexpected digest %q", digest)
			return nil, nil
		}
	}}
	forward := []DigestRow{{Digest: "aaa", At: at}, {Digest: "zzz", At: at}}
	reversed := []DigestRow{{Digest: "zzz", At: at}, {Digest: "aaa", At: at}}
	forwardResult, err := Assemble(t.Context(), forward, r)
	require.NoError(t, err)
	reversedResult, err := Assemble(t.Context(), reversed, r)
	require.NoError(t, err)
	require.Len(t, forwardResult.Submissions, 2)
	require.NotNil(t, forwardResult.Submissions[0].SupersededBy, "the lexicographically smaller digest breaks the tie as the earlier, superseded submission")
	assert.Equal(t, 2, *forwardResult.Submissions[0].SupersededBy)
	assert.Nil(t, forwardResult.Submissions[1].SupersededBy)
	assert.Equal(t, forwardResult, reversedResult, "the tie-break must not depend on the caller's input order")
}

// TestAssemble_RepeatedCallsAgainstUnchangedInputAreDeterministic proves
// section 5.3.3's "repeated calls... return the same output" claim
// against an unchanged input, ordinals included.
//
// Mutation this kills: iterating a Go map of profile names without
// sorting them (e.g. dropping sort.Strings(names) in buildSubmissions)
// would make ordinal assignment depend on map iteration order, which Go
// deliberately randomizes — this loop would go red, possibly flakily,
// the first time two profiles landed in the wrong relative order.
func TestAssemble_RepeatedCallsAgainstUnchangedInputAreDeterministic(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		switch digest {
		case "d-alpha":
			return []byte(minimalDoc("alpha", "a-1")), nil
		case "d-mu":
			return []byte(minimalDoc("mu", "m-1")), nil
		case "d-zulu":
			return []byte(minimalDoc("zulu", "z-1")), nil
		case "d-none":
			return []byte(minimalDoc("", "n-1")), nil
		default:
			t.Fatalf("unexpected digest %q", digest)
			return nil, nil
		}
	}}
	digests := []DigestRow{
		{Digest: "d-zulu", At: at},
		{Digest: "d-none", At: at.Add(time.Minute)},
		{Digest: "d-alpha", At: at.Add(2 * time.Minute)},
		{Digest: "d-mu", At: at.Add(3 * time.Minute)},
	}
	first, err := Assemble(t.Context(), digests, r)
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		next, err := Assemble(t.Context(), digests, r)
		require.NoError(t, err)
		require.Equal(t, idsInOrder(first.Comments), idsInOrder(next.Comments), "iteration %d", i)
		for j := range first.Submissions {
			assert.Equal(t, first.Submissions[j].Ordinal, next.Submissions[j].Ordinal, "iteration %d, submission %d", i, j)
			assert.Equal(t, first.Submissions[j].Profile, next.Submissions[j].Profile, "iteration %d, submission %d", i, j)
		}
	}
}

// TestAssemble_UnreadableFileIsSkippedAndCounted proves section 9: a
// digest whose file cannot be read is skipped, not fatal, and counted in
// Result.Unreadable rather than silently producing an empty submission.
func TestAssemble_UnreadableFileIsSkippedAndCounted(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		if digest == "missing" {
			return nil, errors.New("no such file")
		}
		return []byte(minimalDoc("backend", "ok-1")), nil
	}}
	digests := []DigestRow{{Digest: "missing", At: at}, {Digest: "present", At: at.Add(time.Minute)}}
	result, err := Assemble(t.Context(), digests, r)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Unreadable)
	require.Len(t, result.Submissions, 1, "the unreadable digest contributes no submission")
	assert.Equal(t, "backend", result.Submissions[0].Profile)
}

// TestAssemble_CorruptedContentIsSkippedAndCounted proves the other half
// of section 9's "deleted or corrupted file": bytes that read fine but do
// not parse as a single JSON object are treated the same as a missing
// file, not as a fatal error.
func TestAssemble_CorruptedContentIsSkippedAndCounted(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	r := &readerMock{ReadReviewFunc: func(_ context.Context, digest string) ([]byte, error) {
		if digest == "corrupt" {
			return []byte("not json{{{"), nil
		}
		return []byte(minimalDoc("backend", "ok-1")), nil
	}}
	digests := []DigestRow{{Digest: "corrupt", At: at}, {Digest: "fine", At: at.Add(time.Minute)}}
	result, err := Assemble(t.Context(), digests, r)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Unreadable)
	require.Len(t, result.Submissions, 1)
}

// TestAssemble_ContextCancellationStopsEarly proves Assemble respects a
// canceled context instead of reading every remaining digest.
func TestAssemble_ContextCancellationStopsEarly(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := &readerMock{ReadReviewFunc: func(context.Context, string) ([]byte, error) {
		t.Fatal("ReadReview must not be called once the context is already canceled")
		return nil, nil
	}}
	digests := []DigestRow{{Digest: "any", At: time.Now()}}
	_, err := Assemble(ctx, digests, r)
	assert.Error(t, err)
}

// TestAssemble_NoDigestsIsEmptyNotError matches the empty-store case
// (config.md section 6.2's precedent, reused by combined-reviews.md
// section 9): zero distinct digests produces an empty result, not an
// error.
func TestAssemble_NoDigestsIsEmptyNotError(t *testing.T) {
	t.Parallel()
	r := &readerMock{ReadReviewFunc: func(context.Context, string) ([]byte, error) {
		t.Fatal("ReadReview must not be called when there are no digests")
		return nil, nil
	}}
	result, err := Assemble(t.Context(), nil, r)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Unreadable)
	assert.Empty(t, result.Submissions)
	assert.Empty(t, result.Comments)
}
