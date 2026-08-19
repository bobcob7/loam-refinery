package validate

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bobcob7/refinery/internal/review"
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
	missing := errors.New("not a git repository")
	find := &repositoryFinderMock{FindFunc: func(context.Context, string) (verifier, error) { return nil, missing }}
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
