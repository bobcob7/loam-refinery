package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/entry"
	"github.com/bobcob7/loam-refinery/internal/profile"
	"github.com/bobcob7/loam-refinery/internal/render"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/store"
	"github.com/bobcob7/loam-refinery/internal/structural"
	"github.com/bobcob7/loam-refinery/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type harness struct {
	app         *App
	validator   *documentValidatorMock
	store       *documentStoreMock
	reviews     *reviewStoreMock
	profiles    *profileSourceMock
	headChecker *headCheckerMock
	stdout      *bytes.Buffer
	stderr      *bytes.Buffer
}

func newHarness(t *testing.T, stdin string) *harness {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	validator := &documentValidatorMock{
		ValidateFunc: func(context.Context, []byte, validate.Options) (*review.Result, error) {
			return &review.Result{Valid: true, Comments: 1, Verification: review.Verification{Source: "repo"}}, nil
		},
	}
	storeMock := &documentStoreMock{
		SaveFunc: func(context.Context, StoreInput) error { return nil },
	}
	reviewsMock := noopReviewStore()
	profilesMock := panickyProfileSource()
	headCheckerMock := noopHeadChecker()
	app := New(
		validator,
		storeMock,
		reviewsMock,
		profilesMock,
		testRegistry(t),
		render.NewJSON(),
		headCheckerMock,
		CheckNames{
			Structural:   []string{"id-unique"},
			Verification: []string{"ref-unknown"},
			Advisory:     []string{"body-thin"},
		},
		Build{Version: "1.2.3", Commit: "abc", Schema: "1"},
		func(annotated bool) ([]byte, error) {
			if annotated {
				return []byte("annotated\n"), nil
			}
			return []byte("minimal\n"), nil
		},
		t.TempDir(),
		strings.NewReader(stdin),
		stdout,
		stderr,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)
	return &harness{app: app, validator: validator, store: storeMock, reviews: reviewsMock, profiles: profilesMock, headChecker: headCheckerMock, stdout: stdout, stderr: stderr}
}

// panickyProfileSource stands in for profileSource wherever a test drives a
// command that must never construct or call one at all: bare prime, and
// every command other than prime (docs/cli.md §2.1.1). Panicking rather
// than returning a zero value turns an accidental call into a test failure
// pointing at the call site, instead of a silently wrong result.
func panickyProfileSource() *profileSourceMock {
	return &profileSourceMock{
		LoadFunc: func(string) (profile.Profile, bool, error) {
			panic("profileSource.Load called unexpectedly")
		},
		ListFunc: func() ([]profile.Profile, []string, error) {
			panic("profileSource.List called unexpectedly")
		},
	}
}

// noopReviewStore stands in for reviewStore wherever a test drives a command
// other than reviews itself: an empty, known-nothing store that never errors.
func noopReviewStore() *reviewStoreMock {
	return &reviewStoreMock{
		RepoNameFunc: func(context.Context, string) (string, bool, error) { return "", false, nil },
		KnownFunc:    func(context.Context, string) (bool, error) { return false, nil },
		ListReviewsFunc: func(context.Context, string, string, int) ([]store.Review, int, error) {
			return nil, 0, nil
		},
		ListFailedRunsFunc: func(context.Context, string, string, int) ([]store.FailedRun, int, error) {
			return nil, 0, nil
		},
		ListReposFunc: func(context.Context) ([]store.RepoCount, error) { return nil, nil },
		ReadContentFunc: func(string) ([]byte, error) {
			return nil, errors.New("noopReviewStore: no content")
		},
		DistinctDigestsFunc: func(context.Context, string, string) ([]store.DigestRow, error) {
			return nil, nil
		},
		ReviewPathFunc: func(context.Context, string, string, string) (string, error) {
			return "", errors.New("noopReviewStore: no path")
		},
		StoreEnabledFunc: func(context.Context) (bool, error) { return false, nil },
	}
}

// noopHeadChecker stands in for headChecker wherever a test drives a command
// other than collect-reviews: Discover reports "none" and never errors, the
// same answer Discover gives outside a repository.
func noopHeadChecker() *headCheckerMock {
	return &headCheckerMock{
		DiscoverFunc: func(context.Context, string, string) (HeadCheck, error) {
			return noopHeadCheckResult(), nil
		},
	}
}

// noopHeadCheckResult stands in for the HeadCheck a noopHeadChecker's
// Discover returns: source "none", never diverges, never errors.
func noopHeadCheckResult() *HeadCheckMock {
	return &HeadCheckMock{
		SourceFunc: func() string { return "none" },
		IsHeadFunc: func() bool { return false },
		DivergedFunc: func(context.Context, *review.Document, map[string]string) ([]DivergedAnchor, error) {
			return nil, nil
		},
	}
}

func TestRunDispatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{name: "no arguments print usage", args: nil, code: ExitUsage, stderr: "loam-refinery — check a review document"},
		{name: "an unknown command", args: []string{"lint"}, code: ExitUsage, stderr: `unknown command "lint"`},
		{name: "the old command name is unknown, not aliased", args: []string{"validate"}, code: ExitUsage, stderr: `unknown command "validate"`},
		{name: "help", args: []string{"--help"}, code: ExitValid, stdout: "loam-refinery submit-review"},
		{name: "prime", args: []string{"prime"}, code: ExitValid, stdout: "loam-refinery describe --lens="},
		{name: "version", args: []string{"version"}, code: ExitValid, stdout: "loam-refinery 1.2.3\ncommit abc\nschema 1\n"},
		{name: "schema", args: []string{"schema"}, code: ExitValid, stdout: "minimal\n"},
		{name: "annotated schema", args: []string{"schema", "--annotated"}, code: ExitValid, stdout: "annotated\n"},
		{name: "a bad flag", args: []string{"submit-review", "--nope"}, code: ExitUsage},
		{name: "format is not a flag anymore", args: []string{"describe", "--format=yaml"}, code: ExitUsage, stderr: "flag provided but not defined: -format"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			assert.Equal(t, test.code, h.app.Run(t.Context(), test.args))
			assert.Contains(t, h.stdout.String(), test.stdout)
			assert.Contains(t, h.stderr.String(), test.stderr)
		})
	}
}

func TestExitPreconditionIsTheReservedPreconditionBand(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 3, ExitPrecondition)
	assert.NotEqual(t, ExitInvalid, ExitPrecondition, "a review that is wrong must not share a code with a state that is not")
	assert.NotEqual(t, ExitUsage, ExitPrecondition, "a caller-typed mistake must not share a code with a precondition failure")
}

func TestExitToolIsTheReservedToolErrorBand(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 101, ExitTool)
	assert.NotEqual(t, ExitUsage, ExitTool, "a caller-typed mistake must not share a code with a machine failure")
}

// TestUsageBannerNamesEveryExitCode reproduces refinery-a96.27: the banner an
// unknown command and a no-argument invocation both print used to enumerate
// only three exit codes, so a reader who takes it as the list would map 101
// onto nothing, or worse onto 2 — the exact mistake docs/cli.md §4 warns
// against.
func TestUsageBannerNamesEveryExitCode(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.app.Run(t.Context(), nil)
	for _, code := range []string{"0", "1", "2", "3", "101"} {
		assert.Contains(t, h.stderr.String(), code, "the usage banner must name exit %s", code)
	}
}

func TestDescribeResolvesLenses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout []string
		stderr []string
	}{
		{
			name:   "the default summary ends with the index",
			args:   []string{"describe"},
			code:   ExitValid,
			stdout: []string{"A review document is one JSON object", "comments.suggestions.code"},
		},
		{
			name:   "the index alone",
			args:   []string{"describe", "--list"},
			code:   ExitValid,
			stdout: []string{"comments.suggestions.code"},
		},
		{
			name:   "one entry in full",
			args:   []string{"describe", "--lens=priority"},
			code:   ExitValid,
			stdout: []string{`"name": "comments.priority"`, `"namespace": "field"`, `"title": "Priority"`},
		},
		{
			name:   "several entries, deduplicated and in order",
			args:   []string{"describe", "--lens=priority,id-unique,priority"},
			code:   ExitValid,
			stdout: []string{`"name": "comments.priority"`, `"name": "id-unique"`, `"namespace": "check"`},
		},
		{
			name:   "an unknown lens prints the whole index",
			args:   []string{"describe", "--lens=nonsense"},
			code:   ExitUsage,
			stderr: []string{`unknown lens "nonsense"`, "loam-refinery describe --list"},
		},
		{
			name:   "an ambiguous lens prints the candidates alone",
			args:   []string{"describe", "--lens=code"},
			code:   ExitUsage,
			stderr: []string{"comments.code", "comments.suggestions.code"},
		},
		{
			// refinery-gne: a root field's own name is one of its ambiguous
			// candidates, unlike --lens=code above where neither candidate is
			// a bare root field; this is the shape that went silent.
			name:   "an ambiguous lens naming a root field still names both candidates",
			args:   []string{"describe", "--lens=summary"},
			code:   ExitUsage,
			stderr: []string{"comments.suggestions.summary", "field:summary"},
		},
		{
			name:   "an empty lens is a usage error",
			args:   []string{"describe", "--lens="},
			code:   ExitUsage,
			stderr: []string{"--lens needs at least one name"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			assert.Equal(t, test.code, h.app.Run(t.Context(), test.args))
			for _, want := range test.stdout {
				assert.Contains(t, h.stdout.String(), want)
			}
			for _, want := range test.stderr {
				assert.Contains(t, h.stderr.String(), want)
			}
		})
	}
}

func TestDescribeNeverFallsBackToTheSummary(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	require.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"describe", "--lens=nonsense"}))
	assert.NotContains(t, h.stdout.String(), "A review document is one JSON object")
}

func TestValidateExitCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result *review.Result
		err    error
		code   int
	}{
		{name: "valid", result: &review.Result{Valid: true}, code: ExitValid},
		{name: "invalid", result: &review.Result{}, code: ExitInvalid},
		{name: "precondition", result: &review.Result{Precondition: true}, code: ExitPrecondition},
		{name: "unparseable document", err: parseError(t), code: ExitInvalid},
		{name: "wiring failure", err: errors.New("no repository finder"), code: ExitUsage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, `{"version":"1"}`)
			h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
				return test.result, test.err
			}
			assert.Equal(t, test.code, h.app.Run(t.Context(), []string{"submit-review"}))
		})
	}
}

// A store that cannot be established, written, or recorded to exits
// ExitTool with nothing on stdout (docs/config.md §5.1) — storing happens
// before rendering specifically so a caller can never see a clean-looking
// result for a run that failed.
func TestStoreFailureExitsToolWithEmptyStdout(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		valid bool
	}{
		{name: "a passing run", valid: true},
		{name: "a failing run", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, `{"version":"1"}`)
			h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
				return &review.Result{Valid: test.valid}, nil
			}
			h.store.SaveFunc = func(context.Context, StoreInput) error {
				return errors.New("read-only home directory")
			}
			assert.Equal(t, ExitTool, h.app.Run(t.Context(), []string{"submit-review"}))
			assert.Empty(t, h.stdout.String())
			assert.Contains(t, h.stderr.String(), "read-only home directory")
		})
	}
}

// A run that never reads a document creates nothing (docs/config.md §2.2,
// §5): a usage error before the read, or one that fails the read itself,
// must never reach the store.
func TestValidateNeverStoresWithoutReadingADocument(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "a bad flag", args: []string{"submit-review", "--nope"}},
		{name: "an unreadable path", args: []string{"submit-review", "/nope/review.json"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			h.app.Run(t.Context(), test.args)
			assert.Empty(t, h.store.SaveCalls())
		})
	}
}

// The "wiring failure" branch of TestValidateExitCodes reports a validator
// error that never reached a document, and it must not store either — the
// same reasoning as an unreadable path, one call earlier in the pipeline.
func TestValidateNeverStoresOnAWiringFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t, `{"version":"1"}`)
	h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
		return nil, errors.New("no repository finder")
	}
	require.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"submit-review"}))
	assert.Empty(t, h.store.SaveCalls())
}

// Ref and verdict are not on review.Result, so validate has to read them off
// the document itself for the store — and only when they are actually
// there, so a document missing either still stores cleanly.
func TestValidateSendsRefAndVerdictToTheStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t, `{"version":"1","verdict":"approve","assessment":"strong","ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f","comments":[]}`)
	h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
		return &review.Result{Valid: true, Comments: 2}, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"submit-review"}))
	require.Len(t, h.store.SaveCalls(), 1)
	in := h.store.SaveCalls()[0].In
	assert.True(t, in.Valid)
	assert.Equal(t, "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", in.Ref)
	assert.Equal(t, "approve", in.Verdict)
	assert.Equal(t, "strong", in.Assessment)
	assert.Equal(t, 2, in.Comments)
	assert.NotEmpty(t, in.Dir, "repository identity is resolved from the working directory")
}

// A precondition result reaches the store with Precondition set, so
// documentStore knows to record a row and write no file (docs/config.md
// §5) rather than reaching for WriteRejected because Valid happens to be
// false too.
func TestValidateSendsPreconditionToTheStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t, `{"version":"1","ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"}`)
	h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
		return &review.Result{
			Precondition: true,
			Diagnostics: []review.Diagnostic{{
				Severity: review.SeverityError, Name: "anchor-worktree-diverged", Message: "a.go differs",
			}},
		}, nil
	}
	require.Equal(t, ExitPrecondition, h.app.Run(t.Context(), []string{"submit-review"}))
	require.Len(t, h.store.SaveCalls(), 1)
	in := h.store.SaveCalls()[0].In
	assert.False(t, in.Valid)
	assert.True(t, in.Precondition)
	assert.Equal(t, "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", in.Ref, "the ref is still read off the document that was submitted")
}

// An unparseable document never yields a ref or a verdict — there is no
// document to read either off of — and it still reaches the store, since
// exit 1 keeps the rejected input regardless.
func TestValidateSendsNoRefOrVerdictForAnUnparseableDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "nonsense")
	h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
		return nil, parseError(t)
	}
	require.Equal(t, ExitInvalid, h.app.Run(t.Context(), []string{"submit-review"}))
	require.Len(t, h.store.SaveCalls(), 1)
	in := h.store.SaveCalls()[0].In
	assert.False(t, in.Valid)
	assert.Empty(t, in.Ref)
	assert.Empty(t, in.Verdict)
	assert.Empty(t, in.Assessment)
	assert.Equal(t, []byte("nonsense"), in.Source)
}

func TestValidateReadsStdinOrAPath(t *testing.T) {
	t.Parallel()
	t.Run("stdin when the path is omitted", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, `{"from":"stdin"}`)
		require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"submit-review"}))
		assert.Equal(t, `{"from":"stdin"}`, string(h.validator.ValidateCalls()[0].Source))
	})
	t.Run("stdin for a dash", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, `{"from":"dash"}`)
		require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"submit-review", "-"}))
		assert.Equal(t, `{"from":"dash"}`, string(h.validator.ValidateCalls()[0].Source))
	})
	t.Run("a file", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "")
		path := filepath.Join(t.TempDir(), "review.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"from":"file"}`), 0o644))
		require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"submit-review", path}))
		assert.Equal(t, `{"from":"file"}`, string(h.validator.ValidateCalls()[0].Source))
	})
	t.Run("an unreadable path is a usage error", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "")
		assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"submit-review", "/nope/review.json"}))
		assert.Contains(t, h.stderr.String(), "reading /nope/review.json")
	})
}

// --disable, --warn-only, and --require-verification are gone: submit-review
// no longer registers them, so the flag package's own "flag provided but not
// defined" error is what a caller gets, the same as any other typo.
func TestRemovedFlagsAreUnknownOnSubmitReview(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--disable=body-thin", "--warn-only=ref-unknown", "--require-verification"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "{}")
			assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"submit-review", flag}))
			assert.Contains(t, h.stderr.String(), "flag provided but not defined")
			assert.Empty(t, h.validator.ValidateCalls(), "an unknown flag surfaces before any work is done")
		})
	}
}

func TestValidatePassesFlagsThrough(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "{}")
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"submit-review", "--strict"}))
	options := h.validator.ValidateCalls()[0].Options
	assert.True(t, options.Strict)
	assert.NotEmpty(t, options.Dir, "verification starts from the working directory")
}

// testRegistry holds one entry per namespace, plus the two field paths ending
// in code that the ambiguity rule exists for, and a root field ("summary")
// that a nested field path also ends in — refinery-gne's shape, where one
// ambiguity candidate resolves directly off the root name rather than only
// through the suffix scan.
func testRegistry(t *testing.T) *entry.Registry {
	t.Helper()
	registry, err := entry.NewRegistry(&stubProvider{entries: []entry.Entry{
		{Name: "comments.priority", Namespace: entry.NamespaceField, Title: "Priority", Body: "Integer 1-10."},
		{Name: "comments.code", Namespace: entry.NamespaceField, Title: "Code", Body: "The problem as it stands."},
		{Name: "comments.suggestions.code", Namespace: entry.NamespaceField, Title: "Resulting code", Body: "The fix."},
		{Name: "summary", Namespace: entry.NamespaceField, Title: "Summary", Body: "The review's own summary."},
		{Name: "comments.suggestions.summary", Namespace: entry.NamespaceField, Title: "Suggestion summary", Body: "The suggestion's summary."},
		{Name: "id-unique", Namespace: entry.NamespaceCheck, Title: "Duplicate id", Body: "Two comments share an id."},
		{Name: "ids", Namespace: entry.NamespaceTopic, Title: "Ids", Body: "Checkable claims."},
	}})
	require.NoError(t, err)
	return registry
}

type stubProvider struct {
	entries []entry.Entry
}

func (p *stubProvider) Name() string { return "test" }

func (p *stubProvider) Entries() ([]entry.Entry, error) { return p.entries, nil }

// parseError returns the error a real unparseable document produces, so the
// exit-code mapping is tested against the predicate rather than a stand-in.
func parseError(t *testing.T) error {
	t.Helper()
	_, err := review.Parse([]byte("nonsense"))
	require.Error(t, err)
	require.True(t, review.IsDocumentError(err))
	return fmt.Errorf("reading review document: %w", err)
}

func TestFlagsMayFollowThePath(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	var got validate.Options
	h.validator.ValidateFunc = func(_ context.Context, _ []byte, options validate.Options) (*review.Result, error) {
		got = options
		return &review.Result{Valid: true}, nil
	}
	path := filepath.Join(t.TempDir(), "review.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1"}`), 0o644))
	assert.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"submit-review", path, "--strict"}))
	assert.True(t, got.Strict, "a flag written after the path is still a flag")
}

func TestSubcommandsRejectStrayArguments(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"version":                          {"version", "foo"},
		"list with a lens":                 {"describe", "--list", "--lens=verdict"},
		"prime with a positional argument": {"prime", "somefile.md"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			assert.Equal(t, ExitUsage, h.app.Run(t.Context(), args))
		})
	}
}

func TestAnEmptyElementIsNotReportedAsAnEmptyList(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"describe", "--lens=priority,,id-unique"}))
	assert.Contains(t, h.stderr.String(), "empty name")
}

// Exit 1 has to mean the same thing in the one format there is. Writing the
// failure past the renderer left submit-review exiting 1 with an empty
// stdout, which reads to a caller unmarshalling it as a crashed tool rather
// than as a document to repair.
func TestAnUnparseableDocumentIsRenderedInTheChosenFormat(t *testing.T) {
	t.Parallel()
	t.Run("the failure is a document, not prose", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "nonsense")
		h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
			return nil, parseError(t)
		}
		require.Equal(t, ExitInvalid, h.app.Run(t.Context(), []string{"submit-review"}))
		var payload struct {
			Valid       bool `json:"valid"`
			Diagnostics []struct {
				Severity string `json:"severity"`
				Name     string `json:"name"`
				Message  string `json:"message"`
			} `json:"diagnostics"`
		}
		require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &payload), "stdout was %q", h.stdout.String())
		assert.False(t, payload.Valid)
		require.Len(t, payload.Diagnostics, 1)
		assert.Equal(t, "document-unparseable", payload.Diagnostics[0].Name)
		assert.Equal(t, "error", payload.Diagnostics[0].Severity)
		assert.Contains(t, payload.Diagnostics[0].Message, "reading review document")
	})
	t.Run("the lens to open is named in the payload", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "nonsense")
		h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
			return nil, parseError(t)
		}
		require.Equal(t, ExitInvalid, h.app.Run(t.Context(), []string{"submit-review"}))
		var payload struct {
			Lenses []string `json:"lenses"`
		}
		require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &payload))
		assert.Equal(t, []string{"document-unparseable"}, payload.Lenses,
			"prime tells an agent to read lenses rather than recall a name")
	})
}

// The diagnostic's check name has to be the registry's, or the describe command
// the run hands back exits 2 with "unknown lens" — and no golden file would
// notice, because regenerating them makes the drift look intended.
func TestTheUnparseableCheckNameIsTheRegisteredOne(t *testing.T) {
	t.Parallel()
	names := []string{}
	for _, check := range structural.Checks() {
		names = append(names, check.Name)
	}
	assert.Contains(t, names, checkDocumentUnparseable,
		"the name the CLI reports must be a check describe --lens can open")
}

// Verification is unconditional now: a document with any anchor, run where
// no repository can answer it, exits 1 — this was previously an ordinary,
// passing case (docs/cli.md §2.3.1, before its own amendment). Both ways a
// repository can fail to answer — standing outside one entirely ("none") and
// finding one git cannot ask ("unavailable") — must fail the same way, so
// this is asserted for the validator's real, unmocked wiring: the exact
// regression this bead exists to prevent is a mock that agrees with itself
// while the real path still exits 0.
func TestUnverifiedAnchorsFailOutsideARepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	anchoredDoc := `{"version":"1","verdict":"comment","summary":"a summary long enough for the schema to accept it",` +
		`"comments":[{"id":"a-1","priority":5,"category":"correctness","body":"a body long enough for the schema to accept it",` +
		`"anchors":[{"file":"a.go","line":1}],"suggestions":[]}]}`
	out, code := runValidate(t, dir, anchoredDoc)
	assert.Equal(t, ExitInvalid, code, out)
	assert.Contains(t, out, "verification-required")
}

// A document naming no anchors has nothing for verification to withhold, so
// it must keep exiting 0 in the same no-repository circumstance the anchored
// case above now fails in — verification-required must never start firing on
// nothing to verify.
func TestZeroAnchorDocumentStillPassesOutsideARepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clean := `{"version":"1","verdict":"approve","summary":"a summary long enough for the schema to accept it",` +
		`"ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f","comments":[]}`
	out, code := runValidate(t, dir, clean)
	assert.Equal(t, ExitValid, code, out)
}

// The "none" and "unavailable" verification sources must fail the same way:
// a repository git could not even ask must not read as though nothing had
// to answer. This is asserted at the wiring level — the validator's own
// account of Source is trusted, and the CLI's job is only to turn a result
// it reports invalid into exit 1, whichever source produced it.
func TestUnverifiedAnchorsFailWhicheverSourceCouldNotAnswer(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"none", "unavailable"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, `{"version":"1"}`)
			h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
				return &review.Result{
					Valid: false,
					Diagnostics: []review.Diagnostic{{
						Severity: review.SeverityError,
						Name:     "verification-required",
						Message:  "the anchors were not verified: no repository answered",
					}},
					Verification: review.Verification{Source: source, Anchors: 1},
				}, nil
			}
			assert.Equal(t, ExitInvalid, h.app.Run(t.Context(), []string{"submit-review"}))
		})
	}
}

// TestFormatIsAnUnknownFlagEverywhereItUsedToBeAccepted pins docs/cli.md
// §5.1: submit-review, describe, and reviews carry no --format flag at all
// (refinery-uyb.4), so passing it is an ordinary unknown-flag mistake, not
// a recognized-but-rejected value — the flag package's own error, not a
// hand-rolled "format is gone" message, which is collect-reviews's framing
// to use, not these three's.
func TestFormatIsAnUnknownFlagEverywhereItUsedToBeAccepted(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"submit-review", "describe", "reviews"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, `{"version":"1"}`)
			code := h.app.Run(t.Context(), []string{command, "--format=json"})
			assert.Equal(t, ExitUsage, code)
			assert.Contains(t, h.stderr.String(), "flag provided but not defined: -format")
		})
	}
}
