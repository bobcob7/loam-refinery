package validate

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const document = `{"version":"1","verdict":"comment","summary":"a summary long enough for the schema to accept it","comments":[]}`

// anchored carries one anchor, because a document with nothing to verify is not
// a document verification can require anything of.
const anchored = `{"version":"1","verdict":"comment","summary":"a summary long enough for the schema to accept it",` +
	`"comments":[{"id":"a-1","priority":5,"category":"correctness","body":"a body long enough for the schema to accept it",` +
	`"anchors":[{"file":"a.go","line":1}],"suggestions":[]}]}`

func TestNoTierGatesAnother(t *testing.T) {
	t.Parallel()
	structural := &structuralCheckerMock{CheckFunc: func(*review.Document) []review.Diagnostic {
		return []review.Diagnostic{{Severity: review.SeverityError, Name: "schema", Path: "/summary", Message: "too short"}}
	}}
	verifier := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return []review.Diagnostic{{Severity: review.SeverityError, Name: "anchor-file-missing", Message: "gone"}},
			nil, review.Verification{Source: "repo", Anchors: 1}
	}}
	advisories := &advisoryRunnerMock{RunFunc: func(*review.Document) ([]review.Diagnostic, []review.Skipped) {
		return []review.Diagnostic{{Severity: review.SeverityAdvisory, Name: "body-thin", Message: "thin"}},
			[]review.Skipped{{Name: "priority-flat", Reason: "1 comment has unusable priority"}}
	}}
	result, err := validator(structural, advisories, finder(verifier)).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err)
	assert.Len(t, structural.CheckCalls(), 1)
	assert.Len(t, verifier.VerifyCalls(), 1, "a schema failure does not stop verification")
	assert.Len(t, advisories.RunCalls(), 1, "a verification failure does not stop the advisories")
	assert.Equal(t, 2, result.Errors())
	assert.Equal(t, 1, result.Advisories())
	assert.Len(t, result.Skipped, 1)
	assert.False(t, result.Valid)
}

func TestVerificationSkipsAreReportedNotPassed(t *testing.T) {
	t.Parallel()
	find := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) {
		return nil, verify.ErrNoRepository
	}}
	result, err := validator(passing(), quiet(), find).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err)
	assert.Equal(t, "none", result.Verification.Source)
	assert.Equal(t, "not a git repository", result.Verification.Reason)
	assert.Zero(t, result.Verification.Verified)
	names := []string{}
	for _, skipped := range result.Skipped {
		names = append(names, skipped.Name)
		assert.Equal(t, "not a git repository", skipped.Reason)
	}
	assert.Equal(t, []string{"ref-unknown", "anchor-file-missing", "anchor-line-out-of-range"}, names)
	assert.True(t, result.Valid, "a document with no anchors is never unverified")
}

// Being outside a repository is ordinary. A repository that could not be asked
// is not, and the two must not look alike to a caller — but neither may stop
// the run, because the structural and advisory findings are about the document
// and stand whatever git did.
func TestAnUnreachableRepositoryIsReportedNotConfusedWithAbsence(t *testing.T) {
	t.Parallel()
	broken := errors.New(`running git: exec: "git": executable file not found in $PATH`)
	find := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) { return nil, broken }}
	structural := &structuralCheckerMock{CheckFunc: func(*review.Document) []review.Diagnostic {
		return []review.Diagnostic{{Severity: review.SeverityError, Name: "schema", Path: "/summary", Message: "too short"}}
	}}
	result, err := validator(structural, quiet(), find).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err, "a broken machine is not a reason to stop reporting the document")
	assert.Equal(t, "unavailable", result.Verification.Source,
		"a repository that could not be asked is not the same as standing outside one")
	assert.Equal(t, broken.Error(), result.Verification.Reason)
	assert.Equal(t, 1, result.Errors(), "the findings already computed survive a git that never ran")
	assert.False(t, result.Valid)
	names := []string{}
	for _, skipped := range result.Skipped {
		names = append(names, skipped.Name)
	}
	assert.Equal(t, []string{"ref-unknown", "anchor-file-missing", "anchor-line-out-of-range"}, names,
		"the checks that did not run are reported, never silently omitted")
}

// The two unverified sources must stay distinguishable, which is the whole
// point of separating them: identical output would let a caller read a run that
// checked nothing as a run that had nothing to check. Neither source fails a
// document with no anchors — there is nothing for either of them to withhold.
func TestAbsenceAndUnavailabilityAreDifferentSources(t *testing.T) {
	t.Parallel()
	absent := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) {
		return nil, verify.ErrNoRepository
	}}
	refused := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) {
		return nil, errors.New("git could not identify a repository here: fatal: detected dubious ownership")
	}}
	outside, err := validator(passing(), quiet(), absent).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err)
	unreachable, err := validator(passing(), quiet(), refused).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err)
	assert.Equal(t, "none", outside.Verification.Source)
	assert.Equal(t, "unavailable", unreachable.Verification.Source)
	assert.NotEqual(t, outside.Verification.Reason, unreachable.Verification.Reason)
	assert.True(t, outside.Valid, "a document with no anchors is not wrong because there was no repository")
	assert.True(t, unreachable.Valid, "nor because git refused to answer")
}

func TestStrictMakesAdvisoriesGate(t *testing.T) {
	t.Parallel()
	advisories := &advisoryRunnerMock{RunFunc: func(*review.Document) ([]review.Diagnostic, []review.Skipped) {
		return []review.Diagnostic{{Severity: review.SeverityAdvisory, Name: "body-thin"}}, nil
	}}
	relaxed, err := validator(passing(), advisories, finder(nil)).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err)
	assert.True(t, relaxed.Valid, "advisories alone never fail a run")
	strict, err := validator(passing(), advisories, finder(nil)).Validate(t.Context(), []byte(document), Options{Strict: true})
	require.NoError(t, err)
	assert.False(t, strict.Valid)
	assert.True(t, strict.Strict)
}

func TestMalformedJSONIsTheOnlyTrueStop(t *testing.T) {
	t.Parallel()
	structural := passing()
	result, err := validator(structural, quiet(), finder(nil)).Validate(t.Context(), []byte("nonsense"), Options{})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Empty(t, structural.CheckCalls(), "nothing runs when the document does not parse")
}

func validator(structural *structuralCheckerMock, advisories *advisoryRunnerMock, find *repositoryFinderMock) *Validator {
	return New(structural, advisories, find, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func finder(v *verifierMock) *repositoryFinderMock {
	if v == nil {
		v = &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
			return nil, nil, review.Verification{Source: "repo"}
		}}
	}
	return &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) { return v, nil }}
}

func passing() *structuralCheckerMock {
	return &structuralCheckerMock{CheckFunc: func(*review.Document) []review.Diagnostic { return nil }}
}

func quiet() *advisoryRunnerMock {
	return &advisoryRunnerMock{RunFunc: func(*review.Document) ([]review.Diagnostic, []review.Skipped) {
		return nil, nil
	}}
}

// Verification is unconditional now: an anchor claim that went unchecked
// fails the run regardless of how the gap happened, and the diagnostic names
// why nothing was checked.
func TestVerificationFailsOnlyWhenNothingWasChecked(t *testing.T) {
	t.Parallel()
	checked := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, nil, review.Verification{Source: "repo", Anchors: 1, Verified: 1}
	}}
	partly := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, []review.Skipped{{Name: "anchor-file-missing", Reason: "git could not read the file for 1 anchor"}},
			review.Verification{Source: "repo", Anchors: 2, Verified: 1}
	}}
	absent := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) {
		return nil, verify.ErrNoRepository
	}}
	tests := []struct {
		name   string
		finder *repositoryFinderMock
		valid  bool
		reason string
	}{
		{name: "every anchor checked", finder: finder(checked), valid: true},
		{name: "no repository", finder: absent, reason: "not a git repository"},
		{name: "some anchors unread", finder: finder(partly), reason: "git could not read the file for 1 anchor"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := validator(passing(), quiet(), test.finder).Validate(t.Context(), []byte(anchored), Options{})
			require.NoError(t, err)
			assert.Equal(t, test.valid, result.Valid)
			if test.valid {
				return
			}
			require.Len(t, result.Diagnostics, 1)
			assert.Equal(t, "verification-required", result.Diagnostics[0].Name)
			assert.Equal(t, review.SeverityError, result.Diagnostics[0].Severity)
			assert.Contains(t, result.Diagnostics[0].Message, test.reason,
				"the diagnostic names why nothing was checked, not just that nothing was")
		})
	}
}

// A document with nothing to verify must get the same answer wherever it is
// validated. Deciding on Source alone made one file pass inside a repository
// and fail outside it, over anchors that do not exist either way — and
// verification-required must never start firing on a document with no
// anchors just because no repository could be found.
func TestVerificationIgnoresADocumentWithNoAnchors(t *testing.T) {
	t.Parallel()
	inside := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, nil, review.Verification{Source: "repo"}
	}}
	outside := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) {
		return nil, verify.ErrNoRepository
	}}
	within, err := validator(passing(), quiet(), finder(inside)).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err)
	without, err := validator(passing(), quiet(), outside).Validate(t.Context(), []byte(document), Options{})
	require.NoError(t, err)
	assert.True(t, within.Valid)
	assert.True(t, without.Valid, "the same document cannot be valid in one directory and not in another")
}

// The diagnostic names every distinct cause, so a document-side skip reported
// first cannot hide a machine-side one behind it.
func TestTheVerificationRequirementNamesEveryCause(t *testing.T) {
	t.Parallel()
	mixed := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, []review.Skipped{
			{Name: "anchor-line-out-of-range", Reason: "unusable field on 1 anchor"},
			{Name: "anchor-file-missing", Reason: "git could not read the file for 1 anchor"},
		}, review.Verification{Source: "repo", Anchors: 2}
	}}
	result, err := validator(passing(), quiet(), finder(mixed)).Validate(t.Context(), []byte(anchored), Options{})
	require.NoError(t, err)
	require.Equal(t, 1, result.Errors())
	assert.Contains(t, result.Diagnostics[0].Message, "unusable field on 1 anchor")
	assert.Contains(t, result.Diagnostics[0].Message, "git could not read the file for 1 anchor",
		"a broken object store must not be hidden behind a malformed field")
}

// anchor-worktree-diverged used to be folded into verification-required,
// exit 1, once the ordinary per-anchor pass reached it. docs/cli.md §2.3.1
// now makes it a precondition instead: refinery-uyb.6 replaces this test
// with the ones below, which pin the precondition firing before structural
// checks or advisories run, once for the document, and only when ref is
// HEAD.

// The precondition: any anchor verify reports diverged, with Source "repo",
// stops the run before structural checks or advisories ever execute, and
// reports exactly one diagnostic naming anchor-worktree-diverged — never
// verification-required, which is a different check for a different cause
// (docs/cli.md §2.3.1, §4).
func TestWorktreeDivergedIsAPreconditionBeforeEverythingElse(t *testing.T) {
	t.Parallel()
	diverged := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, nil, review.Verification{Source: "repo", Anchors: 1, Verified: 0, Unverified: []review.Unverified{
			{Name: "anchor-worktree-diverged", Comment: "a-1", Path: "/comments/0/anchors/0", Message: "a.go differs from 4f2c1a9 in the working tree"},
		}}
	}}
	structural := &structuralCheckerMock{CheckFunc: func(*review.Document) []review.Diagnostic {
		t.Fatal("structural checks must not run once the precondition has fired")
		return nil
	}}
	advisories := &advisoryRunnerMock{RunFunc: func(*review.Document) ([]review.Diagnostic, []review.Skipped) {
		t.Fatal("advisories must not run once the precondition has fired")
		return nil, nil
	}}
	result, err := validator(structural, advisories, finder(diverged)).Validate(t.Context(), []byte(anchored), Options{})
	require.NoError(t, err)
	assert.Empty(t, structural.CheckCalls(), "the precondition must run before the structural tier")
	assert.Empty(t, advisories.RunCalls(), "the precondition must run before advisories")
	require.True(t, result.Precondition)
	assert.False(t, result.Valid)
	require.Len(t, result.Diagnostics, 1, "one diagnostic for the whole document, not one per anchor")
	assert.Equal(t, "anchor-worktree-diverged", result.Diagnostics[0].Name)
	assert.Equal(t, review.SeverityError, result.Diagnostics[0].Severity)
	assert.Contains(t, result.Diagnostics[0].Message, "a.go differs from 4f2c1a9 in the working tree")
	assert.Contains(t, result.Diagnostics[0].Message, "the reviewed state is not a commit")
	assert.Contains(t, result.Diagnostics[0].Message, `git stash create`)
	assert.Contains(t, result.Diagnostics[0].Message, "do not retry against this ref")
}

// Several diverged anchors, even across several files, still cost one
// diagnostic — the flat cost docs/cli.md §6.1 documents, replacing the old
// per-anchor row.
func TestWorktreeDivergedReportsOneDiagnosticForSeveralAnchors(t *testing.T) {
	t.Parallel()
	diverged := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, nil, review.Verification{Source: "repo", Anchors: 3, Verified: 0, Unverified: []review.Unverified{
			{Name: "anchor-worktree-diverged", Comment: "a-1", Path: "/comments/0/anchors/0", Message: "a.go differs from 4f2c1a9 in the working tree"},
			{Name: "anchor-worktree-diverged", Comment: "a-1", Path: "/comments/0/anchors/1", Message: "b.go differs from 4f2c1a9 in the working tree"},
			{Name: "anchor-worktree-diverged", Comment: "a-1", Path: "/comments/0/anchors/2", Message: "c.go differs from 4f2c1a9 in the working tree"},
		}}
	}}
	result, err := validator(passing(), quiet(), finder(diverged)).Validate(t.Context(), []byte(anchored), Options{})
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 1, "one diagnostic per document, never one per diverged anchor")
	assert.Contains(t, result.Diagnostics[0].Message, "a.go", "the first diverged anchor names the diagnostic")
}

// The precondition's Verified count must be the number verify's own pass
// actually confirmed, not zeroed out along with the rest of that pass's
// findings: docs/cli.md §2.3.1's precondition discards diagnostics and
// skips, but anchors: N, verified: 0 is ambiguous between "none of the N
// verified" and "some did, and this response throws that away" — carrying
// the real count through removes the second reading (refinery-695).
func TestWorktreeDivergedCarriesThroughItsVerifiedCount(t *testing.T) {
	t.Parallel()
	diverged := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, nil, review.Verification{Source: "repo", Anchors: 3, Verified: 2, Unverified: []review.Unverified{
			{Name: "anchor-worktree-diverged", Comment: "a-1", Path: "/comments/0/anchors/2", Message: "c.go differs from 4f2c1a9 in the working tree"},
		}}
	}}
	result, err := validator(passing(), quiet(), finder(diverged)).Validate(t.Context(), []byte(anchored), Options{})
	require.NoError(t, err)
	require.True(t, result.Precondition)
	assert.Equal(t, 3, result.Verification.Anchors)
	assert.Equal(t, 2, result.Verification.Verified, "the two anchors verify's own pass confirmed before the diverged one must still be counted")
}

// Mutation guard: the precondition must key off Source == "repo", the same
// signal that guarantees it never fires for a ref other than HEAD
// (internal/verify/verify.go only populates Unverified when isHEAD is
// true) — a broader trigger, such as any non-empty Unverified regardless of
// source, would be wrong, since "none" and "unavailable" never carry one.
func TestWorktreeDivergedNeverFiresOutsideARepository(t *testing.T) {
	t.Parallel()
	outside := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) {
		return nil, verify.ErrNoRepository
	}}
	result, err := validator(passing(), quiet(), outside).Validate(t.Context(), []byte(anchored), Options{})
	require.NoError(t, err)
	assert.False(t, result.Precondition)
}

// Mutation guard: a ref that is not HEAD never diverges, because
// internal/verify never consults the working tree for it — reported here as
// Source "repo" with an empty Unverified, exactly as a non-HEAD ref
// resolves today. The precondition must not fire on it, and the run must
// fall through to the ordinary structural-then-advisory path.
func TestWorktreeDivergedNeverFiresWhenRefIsNotHEAD(t *testing.T) {
	t.Parallel()
	notHEAD := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return nil, nil, review.Verification{Source: "repo", Anchors: 1, Verified: 1}
	}}
	structural := &structuralCheckerMock{CheckFunc: func(*review.Document) []review.Diagnostic { return nil }}
	advisories := &advisoryRunnerMock{RunFunc: func(*review.Document) ([]review.Diagnostic, []review.Skipped) { return nil, nil }}
	result, err := validator(structural, advisories, finder(notHEAD)).Validate(t.Context(), []byte(anchored), Options{})
	require.NoError(t, err)
	assert.False(t, result.Precondition)
	assert.Len(t, structural.CheckCalls(), 1, "the ordinary path still runs the structural tier")
	assert.Len(t, advisories.RunCalls(), 1, "the ordinary path still runs advisories")
	assert.True(t, result.Valid)
}
