package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	"github.com/bobcob7/loam-refinery/internal/store"
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
	// docs/cli.md §6.1: storing every run adds nothing to the result object a
	// clean validate prints, so this must still measure what it measured
	// before the store existed — 63, unchanged by the whole epic.
	assert.Equal(t, 63, base, "a clean validate in a repository costs %d tokens; storing must add nothing to this hot path")
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

// docs/cli.md §6.1's dirty-checkout row was carried over from the
// per-diagnostic cost and marked "not yet measured" — this measures it
// against the real binary the way every other row is, and the way
// TestReviewsStaysWithinBudgetPerRow measures a per-row cost: two sample
// sizes, so the marginal cost of one more unverified anchor is what gets
// checked against the ceiling rather than a single noisy sample.
func TestValidateUnverifiedAnchorStaysWithinBudget(t *testing.T) {
	t.Parallel()
	const perUnverifiedBudget = 60
	dir, ref := divergedRefDir(t)
	small := unverifiedCount(t, runValidateOK(t, dir, unverifiedAnchorsDoc(ref, 3)))
	large := unverifiedCount(t, runValidateOK(t, dir, unverifiedAnchorsDoc(ref, 10)))
	require.Equal(t, 3, small.n)
	require.Equal(t, 10, large.n)
	perUnverified := (large.tokens - small.tokens) / (large.n - small.n)
	assert.LessOrEqual(t, perUnverified, perUnverifiedBudget,
		"each unverified anchor costs about %d tokens; the ceiling is %d", perUnverified, perUnverifiedBudget)
}

// unverifiedCounted is one measurement of a validate run reporting n
// unverified anchors.
type unverifiedCounted struct {
	n      int
	tokens int
}

// runValidateOK runs validate and requires it exited clean: an unverified
// anchor withholds a verification, it does not fail the document.
func runValidateOK(t *testing.T, dir, source string) string {
	t.Helper()
	out, code := runValidate(t, dir, source)
	require.Equal(t, ExitValid, code, out)
	return out
}

// unverifiedCount reads how many anchors a validate run reported unverified,
// pairing it with the run's token cost.
func unverifiedCount(t *testing.T, out string) unverifiedCounted {
	t.Helper()
	var payload struct {
		Verification struct {
			Unverified []struct{} `json:"unverified"`
		} `json:"verification"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	return unverifiedCounted{n: len(payload.Verification.Unverified), tokens: approxTokens(out)}
}

// divergedRefDir makes a throwaway repository with one tracked file at HEAD,
// then diverges the working tree from it without touching history — the
// state a pre-commit review leaves behind, and the one this feature exists
// to make checkable at all (refinery-9rh).
func divergedRefDir(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) []byte {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return out
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line one\nline two\nline three\n"), 0o644))
	run("init", "--quiet")
	run("add", "-A")
	run("-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "--quiet", "-m", "seed")
	ref := strings.TrimSpace(string(run("rev-parse", "HEAD")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line one, edited\nline two\nline three\n"), 0o644))
	return dir, ref
}

// unverifiedAnchorsDoc is a valid review with n anchors into the one file
// divergedRefDir diverges, so every anchor is reported anchor-worktree-diverged
// regardless of the line numbers named.
func unverifiedAnchorsDoc(ref string, n int) string {
	anchors := make([]string, n)
	for i := range anchors {
		anchors[i] = fmt.Sprintf(`{"file":"file.txt","line":%d}`, i+1)
	}
	return `{"version":"1","verdict":"comment","summary":"The retry loop is sound and the deadline propagates to every call it makes.","ref":"` + ref + `",` +
		`"comments":[{"id":"a-1","priority":5,"category":"correctness","body":"a body long enough for the schema to accept it","anchors":[` +
		strings.Join(anchors, ",") + `],"suggestions":[]}]}`
}

// Outside a repository, verification cannot run at all, so SkipAll reports
// three skipped checks — real content the in-repository case never has to
// print (docs/cli.md §6.1). This is a common case, not a rare edge one, and
// it was the other validate row nothing measured.
func TestValidateOutsideARepositoryStaysWithinBudget(t *testing.T) {
	t.Parallel()
	const budget = 140
	dir := t.TempDir()
	clean := `{"version":"1","verdict":"approve","summary":"The retry loop is sound and the deadline propagates to every call it makes.","ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f","comments":[]}`
	out, code := runValidate(t, dir, clean)
	require.Equal(t, ExitValid, code, out)
	assert.LessOrEqual(t, approxTokens(out), budget,
		"a clean validate outside a repository costs about %d tokens; its ceiling is %d", approxTokens(out), budget)
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

// The reviews rows in docs/cli.md §6.1 are a fixed envelope plus a per-row
// cost, not a flat ceiling: a limit on the whole response would stop meaning
// anything once the store holds more rows than the ceiling allows. Each of
// these tests measures the envelope alone, then the marginal cost of a row
// across two different row counts, so a constant-plus-linear shape is
// actually being checked rather than one lucky sample.

func TestReviewsStaysWithinBudgetPerRow(t *testing.T) {
	t.Parallel()
	const (
		baseBudget   = 60
		perRowBudget = 150
	)
	const repo = "example.com/org/reviews-budget"
	emptyTokens := approxTokens(runReviews(t, reviewsOf(t, newRealStore(t)), "reviews", "--repo="+repo, "--limit=0"))
	assert.LessOrEqual(t, emptyTokens, baseBudget,
		"an empty reviews index costs about %d tokens; the fixed overhead ceiling is %d", emptyTokens, baseBudget)
	// Pinned exactly, not just bounded: a field added to reviewsEnvelope
	// grows the empty and non-empty cases by the same amount, which
	// cancels out of every perRow calculation below and would otherwise
	// ship unnoticed (refinery-a96.34).
	assert.Equal(t, 29, emptyTokens, "the empty envelope's fixed overhead must not grow or shrink silently")
	small := newRealStore(t)
	seedReviews(t, small, repo, 3)
	smallTokens := approxTokens(runReviews(t, reviewsOf(t, small), "reviews", "--repo="+repo, "--limit=0"))
	large := newRealStore(t)
	seedReviews(t, large, repo, 10)
	largeTokens := approxTokens(runReviews(t, reviewsOf(t, large), "reviews", "--repo="+repo, "--limit=0"))
	perRowSmall := (smallTokens - emptyTokens) / 3
	perRowLarge := (largeTokens - smallTokens) / 7
	assert.LessOrEqual(t, perRowSmall, perRowBudget,
		"each of the first 3 reviews rows costs about %d tokens; the per-row ceiling is %d", perRowSmall, perRowBudget)
	assert.LessOrEqual(t, perRowLarge, perRowBudget,
		"each of rows 4 through 10 costs about %d tokens; the per-row ceiling is %d", perRowLarge, perRowBudget)
}

func TestReviewsFailedStaysWithinBudgetPerRow(t *testing.T) {
	t.Parallel()
	const (
		baseBudget   = 60
		perRowBudget = 120
	)
	const repo = "example.com/org/reviews-failed-budget"
	emptyTokens := approxTokens(runReviews(t, reviewsOf(t, newRealStore(t)), "reviews", "--repo="+repo, "--failed", "--limit=0"))
	assert.LessOrEqual(t, emptyTokens, baseBudget,
		"an empty --failed index costs about %d tokens; the fixed overhead ceiling is %d", emptyTokens, baseBudget)
	// See TestReviewsStaysWithinBudgetPerRow: a marginal-cost calculation
	// cannot see fixed overhead that grows, so it has to be pinned exactly
	// here too (refinery-a96.34).
	assert.Equal(t, 30, emptyTokens, "the empty failedEnvelope's fixed overhead must not grow or shrink silently")
	small := newRealStore(t)
	seedFailedRuns(t, small, repo, 3)
	smallTokens := approxTokens(runReviews(t, reviewsOf(t, small), "reviews", "--repo="+repo, "--failed", "--limit=0"))
	large := newRealStore(t)
	seedFailedRuns(t, large, repo, 10)
	largeTokens := approxTokens(runReviews(t, reviewsOf(t, large), "reviews", "--repo="+repo, "--failed", "--limit=0"))
	perRowSmall := (smallTokens - emptyTokens) / 3
	perRowLarge := (largeTokens - smallTokens) / 7
	assert.LessOrEqual(t, perRowSmall, perRowBudget,
		"each of the first 3 --failed rows costs about %d tokens; the per-row ceiling is %d", perRowSmall, perRowBudget)
	assert.LessOrEqual(t, perRowLarge, perRowBudget,
		"each of rows 4 through 10 costs about %d tokens; the per-row ceiling is %d", perRowLarge, perRowBudget)
}

func TestReviewsListStaysWithinBudgetPerRepository(t *testing.T) {
	t.Parallel()
	const (
		baseBudget    = 60
		perRepoBudget = 25
	)
	emptyTokens := approxTokens(runReviews(t, reviewsOf(t, newRealStore(t)), "reviews", "--list"))
	assert.LessOrEqual(t, emptyTokens, baseBudget,
		"an empty repository index costs about %d tokens; the fixed overhead ceiling is %d", emptyTokens, baseBudget)
	// See TestReviewsStaysWithinBudgetPerRow: pinned exactly so growth in
	// listEnvelope's fixed overhead cannot hide inside the marginal
	// per-repository calculation below (refinery-a96.34).
	assert.Equal(t, 4, emptyTokens, "the empty listEnvelope's fixed overhead must not grow or shrink silently")
	small := newRealStore(t)
	seedRepos(t, small, 4)
	smallTokens := approxTokens(runReviews(t, reviewsOf(t, small), "reviews", "--list"))
	large := newRealStore(t)
	seedRepos(t, large, 10)
	largeTokens := approxTokens(runReviews(t, reviewsOf(t, large), "reviews", "--list"))
	perRepoSmall := (smallTokens - emptyTokens) / 4
	perRepoLarge := (largeTokens - smallTokens) / 6
	assert.LessOrEqual(t, perRepoSmall, perRepoBudget,
		"each of the first 4 repositories costs about %d tokens; the per-repository ceiling is %d", perRepoSmall, perRepoBudget)
	assert.LessOrEqual(t, perRepoLarge, perRepoBudget,
		"each of repositories 5 through 10 costs about %d tokens; the per-repository ceiling is %d", perRepoLarge, perRepoBudget)
}

// reviews --content is the one call docs/cli.md §6.1 gives no ceiling at
// all, and that has to be a recorded decision rather than a gap: it returns
// a document the caller already wrote, verbatim, at whatever size the
// caller wrote it. This pins the exemption by proving both halves of it —
// the bytes come back unmodified, and a single row can cost more than every
// other row's ceiling in this table.
func TestReviewsContentIsExemptFromEveryBudget(t *testing.T) {
	t.Parallel()
	const repo = "example.com/org/reviews-content"
	st := newRealStore(t)
	padding := strings.Repeat("a", 20000)
	ref := hexRef(0)
	doc := `{"version":"1","verdict":"approve","summary":"` + padding + `","ref":"` + ref + `","comments":[]}`
	digest, _, err := st.WriteReview(repo, ref, []byte(doc))
	require.NoError(t, err)
	require.NoError(t, st.Record(t.Context(), store.RunInput{
		Repo: repo, Ref: ref, Digest: digest, ExitCode: 0, Verdict: "approve",
		NumComments: intPtr(0), NumErrors: intPtr(0), NumAdvisories: intPtr(0), NumSkipped: intPtr(0),
		ToolVersion: "test", SchemaVersion: "1",
	}))
	out := runReviews(t, reviewsOf(t, st), "reviews", "--repo="+repo, "--content", "--limit=1")
	assert.Contains(t, out, padding, "a stored document is returned verbatim, not summarized down to fit a budget")
	const everyOtherRowCeiling = 150
	tokens := approxTokens(out)
	assert.Greater(t, tokens, everyOtherRowCeiling,
		"one --content row alone costs about %d tokens, past every other row's ceiling of %d in this table — docs/cli.md §6.1 states this call has none",
		tokens, everyOtherRowCeiling)
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
		noopReviewStore(),
		panickyProfileSource(),
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
	code := app.Run(t.Context(), []string{"submit-review"})
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
		noopReviewStore(),
		panickyProfileSource(),
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

// realReviewsAdapter answers the reviewStore interface from a real
// *store.Store, the same way cmd/loam-refinery's reviewsAdapter does, minus
// the config-file indirection: these budget tests need real content on
// disk, not config resolution, and every call in this suite passes --repo
// explicitly, so RepoName is never actually exercised.
type realReviewsAdapter struct {
	st *store.Store
}

// reviewsOf wraps st as a reviewStore for runReviews.
func reviewsOf(t *testing.T, st *store.Store) *realReviewsAdapter {
	t.Helper()
	return &realReviewsAdapter{st: st}
}

func (a *realReviewsAdapter) RepoName(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (a *realReviewsAdapter) Known(ctx context.Context, repo string) (bool, error) {
	return a.st.Known(ctx, repo)
}

func (a *realReviewsAdapter) ListReviews(ctx context.Context, repo, ref string, limit int) ([]store.Review, int, error) {
	return a.st.ListReviews(ctx, repo, ref, limit)
}

func (a *realReviewsAdapter) ListFailedRuns(ctx context.Context, repo, ref string, limit int) ([]store.FailedRun, int, error) {
	return a.st.ListFailedRuns(ctx, repo, ref, limit)
}

func (a *realReviewsAdapter) ListRepos(ctx context.Context) ([]store.RepoCount, error) {
	return a.st.ListRepos(ctx)
}

func (a *realReviewsAdapter) ReadContent(path string) ([]byte, error) {
	return a.st.ReadContent(path)
}

// fixedStoreRootLen is the length every store root newRealStore creates is
// padded out to. Every stored review's path field is rooted there
// (internal/store/files.go's ReviewPath and RejectedPath), so a per-row
// token budget measured against t.TempDir() directly is really measuring
// $TMPDIR's length, not the output shape: a 65-character TMPDIR — an
// ordinary length on a CI runner — pushed both TestReviewsStaysWithinBudgetPerRow
// and TestReviewsFailedStaysWithinBudgetPerRow over their ceilings on a
// machine where nothing else changed (refinery-a96.34). Padding every root
// out to the same fixed length, well past anything t.TempDir() plus a long
// TMPDIR plus a long test name produces, makes the row cost measured here a
// property of the output shape again rather than of the machine running the
// test.
const fixedStoreRootLen = 80

// fixedLengthStoreRoot creates and returns a store root whose absolute path
// is always exactly fixedStoreRootLen characters. It is rooted at /tmp
// directly rather than at t.TempDir(): t.TempDir() resolves through
// $TMPDIR, and $TMPDIR's length is a property of the machine running the
// test, not of the reviews output a budget is meant to measure
// (refinery-a96.34) — a CI runner whose $TMPDIR happens to be longer than
// this developer's must measure the same row cost this test already
// passed on that machine. /tmp is used instead of os.TempDir() for the
// same reason: os.TempDir() also honors $TMPDIR, and bypassing it is the
// point.
func fixedLengthStoreRoot(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp("/tmp", "loam-refinery-budget-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	require.Greater(t, fixedStoreRootLen, len(base),
		"/tmp base %q is already too long for the fixed %d-character budget root", base, fixedStoreRootLen)
	const segmentLen = 40
	root := base
	for fixedStoreRootLen-len(root) > segmentLen {
		root = filepath.Join(root, strings.Repeat("a", segmentLen))
	}
	switch diff := fixedStoreRootLen - len(root); {
	case diff == 0:
		// already exactly fixedStoreRootLen; nothing left to add.
	case diff == 1:
		// One character short of the target with no room for both a "/"
		// and a new segment; extend the last component by one instead of
		// starting another.
		root += "a"
	default:
		root = filepath.Join(root, strings.Repeat("a", diff-1))
	}
	require.NoError(t, os.MkdirAll(root, 0o700))
	require.Len(t, root, fixedStoreRootLen)
	return root
}

// newRealStore opens a fresh review store rooted at a fixed-length
// directory (refinery-a96.34), for budget tests that need real store
// content rather than a mock.
func newRealStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(t.Context(), fixedLengthStoreRoot(t), store.NewClock())
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func intPtr(n int) *int { return &n }

// hexRef returns a distinct, validly-shaped 40-character ref for row n, so
// seeded rows look like distinct commits rather than one string repeated.
func hexRef(n int) string {
	return fmt.Sprintf("%040x", n+1)
}

// seedReviews writes n passing runs for repo to st, each with a distinct
// ref, digest, and counts, so the reviews index measures realistic per-row
// content rather than n copies of one row.
func seedReviews(t *testing.T, st *store.Store, repo string, n int) {
	t.Helper()
	for i := range n {
		ref := hexRef(i)
		doc := fmt.Sprintf(`{"version":"1","verdict":"approve","summary":"Row %d passed every check the tool runs.","ref":"%s","comments":[]}`, i, ref)
		digest, _, err := st.WriteReview(repo, ref, []byte(doc))
		require.NoError(t, err)
		require.NoError(t, st.Record(t.Context(), store.RunInput{
			Repo: repo, Ref: ref, Digest: digest, ExitCode: 0, Verdict: "approve",
			NumComments: intPtr(2), NumErrors: intPtr(0), NumAdvisories: intPtr(1), NumSkipped: intPtr(0),
			ToolVersion: "test", SchemaVersion: "1",
		}))
	}
}

// seedFailedRuns writes n failing runs for repo to st, each with a distinct
// ref and a kept rejected input, so --failed measures a realistic index.
func seedFailedRuns(t *testing.T, st *store.Store, repo string, n int) {
	t.Helper()
	for i := range n {
		ref := hexRef(i)
		doc := fmt.Sprintf(`{"verdict":"comment","summary":"Row %d needs work.","ref":"%s","comments":[]}`, i, ref)
		digest, _, err := st.WriteRejected(repo, []byte(doc))
		require.NoError(t, err)
		require.NoError(t, st.Record(t.Context(), store.RunInput{
			Repo: repo, Ref: ref, Digest: digest, ExitCode: 1,
			NumComments: intPtr(1), NumErrors: intPtr(2), NumAdvisories: intPtr(0), NumSkipped: intPtr(0),
			ToolVersion: "test", SchemaVersion: "1",
		}))
	}
}

// seedRepos writes one review row to each of n distinct repositories, so
// --list measures a realistic per-repository cost.
func seedRepos(t *testing.T, st *store.Store, n int) {
	t.Helper()
	for i := range n {
		seedReviews(t, st, fmt.Sprintf("example.com/org/repo-%d", i), 1)
	}
}

// runReviews drives a real reviews command against reviews, a reviewStore
// backed by real store content, and returns stdout.
func runReviews(t *testing.T, reviews reviewStore, args ...string) string {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := New(
		&documentValidatorMock{},
		noopStore(t),
		reviews,
		panickyProfileSource(),
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
		quietLog(),
	)
	require.Equal(t, ExitValid, app.Run(t.Context(), args), "stderr: %s", stderr.String())
	return stdout.String()
}
