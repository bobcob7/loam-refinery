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

	"github.com/bobcob7/refinery/internal/entry"
	"github.com/bobcob7/refinery/internal/render"
	"github.com/bobcob7/refinery/internal/review"
	"github.com/bobcob7/refinery/internal/structural"
	"github.com/bobcob7/refinery/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type harness struct {
	app       *App
	validator *documentValidatorMock
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
	app := New(
		validator,
		testRegistry(t),
		render.NewText(),
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
	return &harness{app: app, validator: validator, stdout: stdout, stderr: stderr}
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
		{name: "no arguments print usage", args: nil, code: ExitUsage, stderr: "refinery — check a review document"},
		{name: "an unknown command", args: []string{"lint"}, code: ExitUsage, stderr: `unknown command "lint"`},
		{name: "help", args: []string{"--help"}, code: ExitValid, stdout: "refinery validate"},
		{name: "prime", args: []string{"prime"}, code: ExitValid, stdout: "refinery describe --lens="},
		{name: "version", args: []string{"version"}, code: ExitValid, stdout: "refinery 1.2.3\ncommit abc\nschema 1\n"},
		{name: "schema", args: []string{"schema"}, code: ExitValid, stdout: "minimal\n"},
		{name: "annotated schema", args: []string{"schema", "--annotated"}, code: ExitValid, stdout: "annotated\n"},
		{name: "a bad flag", args: []string{"validate", "--nope"}, code: ExitUsage},
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
			stdout: []string{"field:comments.priority — Priority"},
		},
		{
			name:   "several entries, deduplicated and in order",
			args:   []string{"describe", "--lens=priority,id-unique,priority"},
			code:   ExitValid,
			stdout: []string{"field:comments.priority", "check:id-unique"},
		},
		{
			name:   "an unknown lens prints the whole index",
			args:   []string{"describe", "--lens=nonsense"},
			code:   ExitUsage,
			stderr: []string{`unknown lens "nonsense"`, "comments.suggestions.code"},
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
			assert.Equal(t, test.code, h.app.Run(t.Context(), []string{"validate"}))
		})
	}
}

func TestValidateReadsStdinOrAPath(t *testing.T) {
	t.Parallel()
	t.Run("stdin when the path is omitted", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, `{"from":"stdin"}`)
		require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"validate"}))
		assert.Equal(t, `{"from":"stdin"}`, string(h.validator.ValidateCalls()[0].Source))
	})
	t.Run("stdin for a dash", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, `{"from":"dash"}`)
		require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"validate", "-"}))
		assert.Equal(t, `{"from":"dash"}`, string(h.validator.ValidateCalls()[0].Source))
	})
	t.Run("a file", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "")
		path := filepath.Join(t.TempDir(), "review.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"from":"file"}`), 0o644))
		require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"validate", path}))
		assert.Equal(t, `{"from":"file"}`, string(h.validator.ValidateCalls()[0].Source))
	})
	t.Run("an unreadable path is a usage error", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "")
		assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"validate", "/nope/review.json"}))
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
			assert.Equal(t, ExitUsage, h.app.Run(t.Context(), append([]string{"validate"}, test.args...)))
			assert.Contains(t, h.stderr.String(), test.stderr)
			assert.Empty(t, h.validator.ValidateCalls(), "a typo surfaces before any work is done")
		})
	}
}

func TestValidatePassesFlagsThrough(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "{}")
	require.Equal(t, ExitValid, h.app.Run(t.Context(),
		[]string{"validate", "--strict", "--disable=body-thin", "--warn-only=ref-unknown"}))
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
	assert.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"validate", path, "--strict"}))
	assert.True(t, got.Strict, "a flag written after the path is still a flag")
}

func TestSubcommandsRejectStrayArguments(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"version":          {"version", "foo"},
		"list with a lens": {"describe", "--list", "--lens=verdict"},
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
	assert.Equal(t, ExitUsage, h.app.Run(t.Context(), []string{"validate", "--disable=body-thin,,vacuous-body"}))
	assert.Contains(t, h.stderr.String(), "empty name")
}

// Exit 1 has to mean the same thing in both formats. Writing the failure past
// the renderer left --format=json exiting 1 with an empty stdout, which reads
// to a caller unmarshalling it as a crashed tool rather than as a document to
// repair.
func TestAnUnparseableDocumentIsRenderedInTheChosenFormat(t *testing.T) {
	t.Parallel()
	t.Run("json", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "nonsense")
		h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
			return nil, parseError(t)
		}
		require.Equal(t, ExitInvalid, h.app.Run(t.Context(), []string{"validate", "--format=json"}))
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
	t.Run("text names a check and hands back the lens", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "nonsense")
		h.validator.ValidateFunc = func(context.Context, []byte, validate.Options) (*review.Result, error) {
			return nil, parseError(t)
		}
		require.Equal(t, ExitInvalid, h.app.Run(t.Context(), []string{"validate"}))
		assert.Contains(t, h.stdout.String(), "INVALID", "the status line still goes to stdout")
		assert.Contains(t, h.stderr.String(), "document-unparseable",
			"prime promises every exit-1 diagnostic names a check")
		assert.Contains(t, h.stderr.String(), "describe --lens=document-unparseable",
			"prime promises the run ends with that command already assembled")
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
