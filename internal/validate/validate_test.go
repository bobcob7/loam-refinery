package validate

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bobcob7/refinery/internal/review"
	"github.com/bobcob7/refinery/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const document = `{"version":"1","verdict":"comment","summary":"a summary long enough for the schema to accept it","comments":[]}`

func TestNoTierGatesAnother(t *testing.T) {
	t.Parallel()
	structural := &structuralCheckerMock{CheckFunc: func(*review.Document) []review.Diagnostic {
		return []review.Diagnostic{{Severity: review.SeverityError, Name: "schema", Path: "/summary", Message: "too short"}}
	}}
	verifier := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return []review.Diagnostic{{Severity: review.SeverityError, Name: "anchor-file-missing", Message: "gone"}},
			nil, review.Verification{Source: "repo", Anchors: 1}
	}}
	advisories := &advisoryRunnerMock{RunFunc: func(*review.Document, map[string]bool) ([]review.Diagnostic, []review.Skipped) {
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
	assert.True(t, result.Valid, "a skipped tier does not make a document invalid")
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
// checked nothing as a run that had nothing to check.
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
	assert.True(t, outside.Valid, "a document is not wrong because there was no repository")
	assert.True(t, unreachable.Valid, "nor because git refused to answer")
}

func TestWarnOnlyDemotesVerificationFailures(t *testing.T) {
	t.Parallel()
	verifier := &verifierMock{VerifyFunc: func(context.Context, *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification) {
		return []review.Diagnostic{{Severity: review.SeverityError, Name: "ref-unknown", Message: "absent"}},
			nil, review.Verification{Source: "repo"}
	}}
	options := Options{WarnOnly: map[string]bool{"ref-unknown": true}}
	result, err := validator(passing(), quiet(), finder(verifier)).Validate(t.Context(), []byte(document), options)
	require.NoError(t, err)
	assert.Zero(t, result.Errors())
	assert.Equal(t, 1, result.Advisories())
	assert.True(t, result.Valid)
}

func TestStrictMakesAdvisoriesGate(t *testing.T) {
	t.Parallel()
	advisories := &advisoryRunnerMock{RunFunc: func(*review.Document, map[string]bool) ([]review.Diagnostic, []review.Skipped) {
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

func TestDisabledAdvisoriesArePassedThrough(t *testing.T) {
	t.Parallel()
	advisories := quiet()
	_, err := validator(passing(), advisories, finder(nil)).Validate(t.Context(), []byte(document),
		Options{Disabled: map[string]bool{"body-thin": true}})
	require.NoError(t, err)
	require.Len(t, advisories.RunCalls(), 1)
	assert.True(t, advisories.RunCalls()[0].Disabled["body-thin"])
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
	return &advisoryRunnerMock{RunFunc: func(*review.Document, map[string]bool) ([]review.Diagnostic, []review.Skipped) {
		return nil, nil
	}}
}

// --require-verification answers one question: were the anchor claims actually
// checked? Off, the run passes whatever the answer; on, an unchecked document
// fails. Nothing about it changes what the document itself is.
func TestRequireVerificationFailsOnlyWhenNothingWasChecked(t *testing.T) {
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
		name    string
		finder  *repositoryFinderMock
		require bool
		valid   bool
		reason  string
	}{
		{name: "checked, not required", finder: finder(checked), valid: true},
		{name: "checked, required", finder: finder(checked), require: true, valid: true},
		{name: "no repository, not required", finder: absent, valid: true},
		{name: "no repository, required", finder: absent, require: true, reason: "not a git repository"},
		{name: "some anchors unread, not required", finder: finder(partly), valid: true},
		{
			name: "some anchors unread, required", finder: finder(partly), require: true,
			reason: "git could not read the file for 1 anchor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := Options{RequireVerification: test.require}
			result, err := validator(passing(), quiet(), test.finder).Validate(t.Context(), []byte(document), options)
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

// The flag is a verification check like any other, so --warn-only reaches it.
// Asking for both is contradictory, but visibly so on the command line.
func TestWarnOnlyDemotesTheVerificationRequirement(t *testing.T) {
	t.Parallel()
	absent := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) {
		return nil, verify.ErrNoRepository
	}}
	options := Options{RequireVerification: true, WarnOnly: map[string]bool{"verification-required": true}}
	result, err := validator(passing(), quiet(), absent).Validate(t.Context(), []byte(document), options)
	require.NoError(t, err)
	assert.Zero(t, result.Errors())
	assert.Equal(t, 1, result.Advisories())
	assert.True(t, result.Valid)
}
