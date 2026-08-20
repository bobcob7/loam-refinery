package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/advisory"
	"github.com/bobcob7/loam-refinery/internal/entry"
	"github.com/bobcob7/loam-refinery/internal/render"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/schema"
	"github.com/bobcob7/loam-refinery/internal/structural"
	"github.com/bobcob7/loam-refinery/internal/validate"
	"github.com/bobcob7/loam-refinery/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// approxTokens estimates tokens as characters over four. It is deliberately
// crude: the budgets in docs/cli.md §6.1 are ceilings on how much a caller pays
// per call, and one character per quarter token is close enough for English
// prose to catch a command growing past its budget, which is what the ceiling
// is for. A real tokenizer would be a dependency for no extra signal.
var update = flag.Bool("update", false, "rewrite the golden files")

func approxTokens(text string) int {
	return len(text) / 4
}

// The budgets in docs/cli.md §6.1, enforced so that a command growing past its
// ceiling fails rather than erodes it quietly.
func TestCommandsStayWithinBudget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		golden string
		budget int
	}{
		{name: "prime", args: []string{"prime"}, golden: "prime.txt", budget: 250},
		{name: "describe", args: []string{"describe"}, golden: "describe.txt", budget: 850},
		{name: "describe --list", args: []string{"describe", "--list"}, golden: "list.txt", budget: 380},
		{name: "schema", args: []string{"schema"}, budget: 1000},
		{name: "schema --annotated", args: []string{"schema", "--annotated"}, budget: 5000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			out := runReal(t, test.args...)
			assert.LessOrEqual(t, approxTokens(out), test.budget,
				"%s costs about %d tokens; its ceiling is %d", test.name, approxTokens(out), test.budget)
			if test.golden != "" {
				goldenFile(t, test.golden, out)
			}
		})
	}
}

// The validate rows in docs/cli.md 6.1 are the ones paid on every loop, and
// were the only rows nothing measured — a limit nothing measures is a limit
// that erodes, which is what the table above it says.
func TestValidateStaysWithinBudget(t *testing.T) {
	t.Parallel()
	const (
		cleanBudget         = 80
		perDiagnosticBudget = 60
	)
	dir, ref := resolvableRefDir(t)
	clean := `{"version":"1","verdict":"approve","summary":"The retry loop is sound and the deadline propagates to every call it makes.","ref":"` + ref + `","comments":[]}`
	out, code := runValidate(t, dir, clean)
	require.Equal(t, ExitValid, code, out)
	assert.LessOrEqual(t, approxTokens(out), cleanBudget,
		"a clean validate costs about %d tokens; its ceiling is %d", approxTokens(out), cleanBudget)
	base := approxTokens(out)
	flawed, code := runValidate(t, dir, `{"version":"1","verdict":"comment","summary":"too short","ref":"`+ref+`","comments":[]}`)
	require.Equal(t, ExitInvalid, code, flawed)
	var payload struct {
		Diagnostics []struct{} `json:"diagnostics"`
	}
	require.NoError(t, json.Unmarshal([]byte(flawed), &payload))
	require.NotEmpty(t, payload.Diagnostics)
	perDiagnostic := (approxTokens(flawed) - base) / len(payload.Diagnostics)
	assert.LessOrEqual(t, perDiagnostic, perDiagnosticBudget,
		"each diagnostic costs about %d tokens; the ceiling is %d", perDiagnostic, perDiagnosticBudget)
}

func TestEveryLensStaysWithinBudget(t *testing.T) {
	t.Parallel()
	const budget = 350
	rendered := &strings.Builder{}
	for _, name := range lensNames(t) {
		out := runReal(t, "describe", "--lens="+name)
		assert.LessOrEqual(t, approxTokens(out), budget,
			"lens %s costs about %d tokens; the per-entry ceiling is %d", name, approxTokens(out), budget)
		rendered.WriteString(out)
		rendered.WriteString("\n")
	}
	goldenFile(t, "lenses.txt", rendered.String())
}

func TestEveryCheckNameIsALens(t *testing.T) {
	t.Parallel()
	registry := realRegistry(t)
	for _, group := range [][]string{names(structural.Checks()), names(verify.Checks()), names(advisory.Checks())} {
		for _, name := range group {
			resolved, err := registry.Resolve(name)
			require.NoError(t, err, "check %s has no lens", name)
			assert.Equal(t, entry.NamespaceCheck, resolved.Namespace)
		}
	}
}

func TestEveryPrintedNameResolves(t *testing.T) {
	t.Parallel()
	registry := realRegistry(t)
	for _, group := range registry.Index() {
		for _, name := range group.Names {
			_, err := registry.Resolve(name)
			assert.NoError(t, err, "the index printed %q, which does not resolve", name)
		}
	}
	for _, e := range registry.All() {
		for _, related := range e.Related {
			_, err := registry.Resolve(related)
			assert.NoError(t, err, "%s relates to %q, which does not resolve", e.Qualified(), related)
		}
	}
}

func TestSchemaOutputIsValidJSON(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"schema"}, {"schema", "--annotated"}} {
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(runReal(t, args...)), &decoded))
		assert.Equal(t, false, decoded["additionalProperties"])
	}
}

// runReal drives the App wired exactly as main wires it, so the golden files
// and the budgets measure what a caller actually gets.
// runValidate runs a real validate, rooted at dir, over source on stdin and
// returns stdout with its exit code, since validate is the one command whose
// cost depends on input.
func runValidate(t *testing.T, dir, source string) (string, int) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := New(
		validate.New(structural.New(mustValidator(t), quietLog()), advisory.New(quietLog(), advisory.All()),
			validate.NewGitFinder(quietLog()), quietLog()),
		noopStore(t),
		realRegistry(t),
		render.NewJSON(),
		CheckNames{},
		Build{Version: "test", Commit: "test", Schema: schema.Version()},
		func(bool) ([]byte, error) { return nil, nil },
		dir,
		strings.NewReader(source),
		stdout,
		stderr,
		quietLog(),
	)
	code := app.Run(t.Context(), []string{"validate"})
	return stdout.String(), code
}

// resolvableRefDir makes a throwaway repository with one commit and returns
// its directory and that commit's SHA, so a document's ref can genuinely
// resolve without depending on this repository's own history. A ref that
// does not resolve is reported as an error even on a document with no
// anchors, so the budget's clean case needs a ref that is real somewhere;
// tying it to refinery's own commits would pass only on this checkout, so
// the test grows its own repository instead.
func resolvableRefDir(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) []byte {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return out
	}
	run("init", "--quiet")
	run("-c", "user.email=test@example.com", "-c", "user.name=test",
		"commit", "--quiet", "--allow-empty", "-m", "seed")
	return dir, strings.TrimSpace(string(run("rev-parse", "HEAD")))
}

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

// noopStore stands in for documentStore wherever a test drives validate but
// is not exercising storing itself: no test here should touch a real config
// file or a real database on disk.
func noopStore(t *testing.T) *documentStoreMock {
	t.Helper()
	return &documentStoreMock{SaveFunc: func(context.Context, StoreInput) error { return nil }}
}

func mustValidator(t *testing.T) *schema.Validator {
	t.Helper()
	v, err := schema.NewValidator()
	require.NoError(t, err)
	return v
}

func runReal(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := New(
		&documentValidatorMock{},
		noopStore(t),
		realRegistry(t),
		render.NewJSON(),
		CheckNames{},
		Build{Version: "test", Commit: "test", Schema: schema.Version()},
		func(annotated bool) ([]byte, error) {
			if annotated {
				return schema.Annotated(), nil
			}
			return schema.Minimal()
		},
		t.TempDir(),
		strings.NewReader(""),
		stdout,
		stderr,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)
	require.Equal(t, ExitValid, app.Run(t.Context(), args), "stderr: %s", stderr.String())
	return stdout.String()
}

func realRegistry(t *testing.T) *entry.Registry {
	t.Helper()
	schemaProvider, err := entry.NewSchemaProvider(schema.Annotated())
	require.NoError(t, err)
	registry, err := entry.NewRegistry(
		schemaProvider,
		entry.NewChecksProvider(structural.Checks(), verify.Checks(), advisory.Checks()),
		entry.NewTopicsProvider(),
	)
	require.NoError(t, err)
	return registry
}

// lensNames asks for every entry by its qualified name, so the per-entry budget
// covers entries an index shortening would otherwise hide.
func lensNames(t *testing.T) []string {
	t.Helper()
	all := []string{}
	for _, e := range realRegistry(t).All() {
		all = append(all, e.Qualified())
	}
	return all
}

func names(checks []review.Check) []string {
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		out = append(out, check.Name)
	}
	return out
}

func goldenFile(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run go test ./internal/cli -update to create %s", path)
	assert.Equal(t, string(want), got)
}
