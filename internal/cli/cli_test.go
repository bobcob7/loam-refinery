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
	app       *App
	validator *documentValidatorMock
	store     *documentStoreMock
	reviews   *reviewStoreMock
	profiles  *profileSourceMock
	stdout    *bytes.Buffer
	stderr    *bytes.Buffer
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
	app := New(
		validator,
		storeMock,
		reviewsMock,
		profilesMock,
		testRegistry(t),
		render.NewJSON(),
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
	return &harness{app: app, validator: validator, store: storeMock, reviews: reviewsMock, profiles: profilesMock, stdout: stdout, stderr: stderr}
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
		{name: "an unknown format", args: []string{"describe", "--format=yaml"}, code: ExitUsage, stderr: `unknown format "yaml"`},
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
	for _, code := range []string{"0", "1", "2", "101"} {
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
	h := newHarness(t, `{"version":"1","verdict":"approve","ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f","comments":[]}`)
	h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
		return &review.Result{Valid: true, Comments: 2}, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"submit-review"}))
	require.Len(t, h.store.SaveCalls(), 1)
	in := h.store.SaveCalls()[0].In
	assert.True(t, in.Valid)
	assert.Equal(t, "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", in.Ref)
	assert.Equal(t, "approve", in.Verdict)
	assert.Equal(t, 2, in.Comments)
	assert.NotEmpty(t, in.Dir, "repository identity is resolved from the working directory")
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

func TestValidateRejectsCheckNamesThatCannotBeUsed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		stderr string
	}{
		{name: "an unknown advisory", args: []string{"--disable=nope"}, stderr: `--disable: unknown check "nope"`},
		{name: "a structural check", args: []string{"--disable=id-unique"}, stderr: "structural checks cannot be disabled"},
		{name: "a verification check in --disable", args: []string{"--disable=ref-unknown"}, stderr: "use --warn-only to demote it"},
		{name: "an advisory in --warn-only", args: []string{"--warn-only=body-thin"}, stderr: "advisories never fail a run"},
		{name: "an empty list", args: []string{"--disable="}, stderr: "--disable needs at least one check name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "{}")
			assert.Equal(t, ExitUsage, h.app.Run(t.Context(), append([]string{"submit-review"}, test.args...)))
			assert.Contains(t, h.stderr.String(), test.stderr)
			assert.Empty(t, h.validator.ValidateCalls(), "a typo surfaces before any work is done")
		})
	}
}

func TestValidatePassesFlagsThrough(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "{}")
	require.Equal(t, ExitValid, h.app.Run(t.Context(),
		[]string{"submit-review", "--strict", "--disable=body-thin", "--warn-only=ref-unknown"}))
	options := h.validator.ValidateCalls()[0].Options
	assert.True(t, options.Strict)
	assert.True(t, options.Disabled["body-thin"])
	assert.True(t, options.WarnOnly["ref-unknown"])
	assert.NotEmpty(t, options.Dir, "verification starts from the working directory")
}

// testRegistry holds one entry per namespace, plus the two field paths ending
// in code that the ambiguity rule exists for.
func testRegistry(t *testing.T) *entry.Registry {
	t.Helper()
	registry, err := entry.NewRegistry(&stubProvider{entries: []entry.Entry{
		{Name: "comments.priority", Namespace: entry.NamespaceField, Title: "Priority", Body: "Integer 1-10."},
		{Name: "comments.code", Namespace: entry.NamespaceField, Title: "Code", Body: "The problem as it stands."},
		{Name: "comments.suggestions.code", Namespace: entry.NamespaceField, Title: "Resulting code", Body: "The fix."},
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
	h := newHarness(t, `{"version":"1"}`)
	assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"submit-review", "--disable=body-thin,,vacuous-body"}))
	assert.Contains(t, h.stderr.String(), "empty name")
}

// Exit 1 has to mean the same thing in both formats. Writing the failure past
// the renderer left --format=json exiting 1 with an empty stdout, which reads
// to a caller unmarshalling it as a crashed tool rather than as a document to
// repair.
func TestAnUnparseableDocumentIsRenderedInTheChosenFormat(t *testing.T) {
	t.Parallel()
	t.Run("the failure is a document, not prose", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "nonsense")
		h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
			return nil, parseError(t)
		}
		require.Equal(t, ExitInvalid, h.app.Run(t.Context(), []string{"submit-review", "--format=json"}))
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

// The flag has to reach the validator, and has to be off unless asked for.
// Nothing else in the suite would notice it defaulting to true.
func TestRequireVerificationReachesTheValidatorAndDefaultsOff(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "absent by default", args: []string{"submit-review"}},
		{name: "given", args: []string{"submit-review", "--require-verification"}, want: true},
		{name: "given as false", args: []string{"submit-review", "--require-verification=false"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, `{"version":"1"}`)
			var got validate.Options
			h.validator.ValidateFunc = func(_ context.Context, _ []byte, options validate.Options) (*review.Result, error) {
				got = options
				return &review.Result{Valid: true}, nil
			}
			require.Equal(t, ExitValid, h.app.Run(t.Context(), test.args))
			assert.Equal(t, test.want, got.RequireVerification)
		})
	}
}

// The one format left is still a decision, and a wrong --format has to fail the
// same way on both commands that take it. Nothing else in the suite would
// notice the flag being ignored entirely.
func TestFormatAcceptsOnlyJSON(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"submit-review", "describe"} {
		for _, test := range []struct {
			name   string
			value  string
			code   int
			stderr string
		}{
			{name: "json", value: "json", code: -1},
			{name: "text says where it went", value: "text", code: ExitUsage, stderr: "the text format is gone"},
			{name: "another format is unknown", value: "yaml", code: ExitUsage, stderr: `unknown format "yaml"`},
			{name: "empty is unknown", value: "", code: ExitUsage, stderr: `unknown format ""`},
		} {
			t.Run(command+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t, `{"version":"1"}`)
				code := h.app.Run(t.Context(), []string{command, "--format=" + test.value})
				if test.code == -1 {
					assert.NotEqual(t, ExitUsage, code, "json is the format that works")
					return
				}
				assert.Equal(t, test.code, code)
				assert.Contains(t, h.stderr.String(), test.stderr)
			})
		}
	}
}
