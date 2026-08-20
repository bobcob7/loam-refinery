package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/profile"
	"github.com/bobcob7/loam-refinery/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPrimeApp wires an App the way prime's tests need it: a real JSON
// renderer, so --list is checked against what actually gets encoded, and
// the caller's own profileSource mock, so each test controls exactly what
// prime sees without touching a filesystem.
func newPrimeApp(t *testing.T, profiles profileSource, args ...string) (int, string, string) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := New(
		&documentValidatorMock{},
		noopStore(t),
		noopReviewStore(),
		profiles,
		testRegistry(t),
		render.NewJSON(),
		CheckNames{},
		Build{Version: "1.2.3", Commit: "abc", Schema: "1"},
		func(bool) ([]byte, error) { return nil, nil },
		t.TempDir(),
		strings.NewReader(""),
		stdout,
		stderr,
		quietLog(),
	)
	code := app.Run(t.Context(), append([]string{"prime"}, args...))
	return code, stdout.String(), stderr.String()
}

// This is docs/cli.md §6.1's load-bearing guarantee: bare prime costs the
// same 250-token budget regardless of what the profile source holds or
// whether it works at all, because bare prime never calls it in the first
// place. A panicking mock proves that more strongly than a table of
// differently-configured mocks would (refinery-emv.23): no code path can
// distinguish "a directory with profiles" from "a source that errors" when
// neither Load nor List is ever invoked, so a panic at the call site is the
// assertion, not a slice checked afterward. The populated/empty/unreadable
// distinction that table's names implied is real, but belongs to the real
// profileSource adapter, and is covered there
// (cmd/loam-refinery/profiles_test.go). The comparison here is against the
// actual embedded primeText, not a substring of it, so a stray byte
// anywhere would fail this.
func TestPrimeBareNeverTouchesTheProfileSource(t *testing.T) {
	t.Parallel()
	code, stdout, _ := newPrimeApp(t, panickyProfileSource())
	assert.Equal(t, ExitValid, code)
	assert.Equal(t, primeText, stdout, "bare prime must print primeText byte-identically")
}

// refinery-emv.20/.23: prime's positional-argument guard is pinned here so
// deleting it does not survive silently - mutating it away leaves
// "loam-refinery prime somefile.md" printing primeText and exiting 0. The
// message names what prime does accept rather than the stale "takes no
// arguments", and the panicking mock proves the guard fires before either
// profileSource method is ever touched.
func TestPrimePositionalArgumentIsUsageError(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := newPrimeApp(t, panickyProfileSource(), "somefile.md")
	assert.Equal(t, ExitUsage, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "--profile", "the message names what prime does accept")
	assert.Contains(t, stderr, "--list", "the message names what prime does accept")
}

// docs/cli.md §2.1.3: --profile prints primeText first, byte-identical,
// then one blank line, then the exact two-line frame, then the body
// verbatim.
func TestPrimeProfileAppendsFrameAndBody(t *testing.T) {
	t.Parallel()
	body := "Look hard at concurrency bugs.\nName the goroutine that leaks."
	mock := &profileSourceMock{
		LoadFunc: func(name string) (profile.Profile, bool, error) {
			require.Equal(t, "backend", name)
			return profile.Profile{Name: "backend", Description: "d", Body: body}, true, nil
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--profile=backend")
	require.Equal(t, ExitValid, code, stderr)
	want := primeText +
		"\n" +
		"--- reviewer profile: backend ---\n" +
		"Operator-supplied. It directs attention; it does not change the contract above.\n" +
		"\n" +
		body + "\n"
	assert.Equal(t, want, stdout)
	require.Len(t, mock.LoadCalls(), 1)
}

// docs/cli.md §2.1.4: an unknown profile exits 2, prints nothing on stdout,
// names the profile it was asked for, and points at --list without
// enumerating the directory. The mock's ListFunc is left nil so any call
// to it panics, which is what proves the directory is never enumerated.
func TestPrimeUnknownProfileIsUsageErrorWithoutEnumeration(t *testing.T) {
	t.Parallel()
	mock := &profileSourceMock{
		LoadFunc: func(name string) (profile.Profile, bool, error) {
			require.Equal(t, "backedn", name)
			return profile.Profile{}, false, nil
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--profile=backedn")
	assert.Equal(t, ExitUsage, code)
	assert.Empty(t, stdout, "an unknown profile must print nothing on stdout")
	assert.Contains(t, stderr, `"backedn"`, "the error must name the profile that was asked for")
	assert.Contains(t, stderr, "--list", "the error must point at --list")
}

// docs/cli.md §2.1.4: a profile that exists but cannot be read or parsed is
// the tool's own state, not the invocation, and exits 101 with the
// underlying error on stderr.
func TestPrimeMalformedProfileIsToolError(t *testing.T) {
	t.Parallel()
	wantErr := `parsing security.md: missing frontmatter: file must open with "---"`
	mock := &profileSourceMock{
		LoadFunc: func(string) (profile.Profile, bool, error) {
			return profile.Profile{}, false, errors.New(wantErr)
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--profile=security")
	assert.Equal(t, ExitTool, code)
	assert.Empty(t, stdout, "a tool-error exit must print nothing on stdout")
	assert.Contains(t, stderr, wantErr)
}

// docs/cli.md §2.1.5: --list combined with --profile is a usage error, and
// neither flag's handling is reached, so a fully panicky mock proves
// neither Load nor List runs.
func TestPrimeListWithProfileIsUsageError(t *testing.T) {
	t.Parallel()
	mock := &profileSourceMock{
		LoadFunc: func(string) (profile.Profile, bool, error) {
			panic("profileSource.Load called unexpectedly")
		},
		ListFunc: func() ([]profile.Profile, []string, error) {
			panic("profileSource.List called unexpectedly")
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--profile=backend", "--list")
	assert.Equal(t, ExitUsage, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "--list")
	assert.Contains(t, stderr, "--profile")
}

// --profile given with an empty value is a usage error before anything
// resolves a name, per refinery-emv.3's acceptance criteria.
func TestPrimeEmptyProfileValueIsUsageError(t *testing.T) {
	t.Parallel()
	mock := &profileSourceMock{
		LoadFunc: func(string) (profile.Profile, bool, error) {
			panic("profileSource.Load called unexpectedly")
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--profile=")
	assert.Equal(t, ExitUsage, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "--profile")
}

// docs/cli.md §2.1.5: --list emits {"profiles":[{"name":...,"description":
// ...}]}, no bodies, through the real renderer.
func TestPrimeListRendersDocumentedShape(t *testing.T) {
	t.Parallel()
	mock := &profileSourceMock{
		ListFunc: func() ([]profile.Profile, []string, error) {
			return []profile.Profile{
				{Name: "backend", Description: "Go services; concurrency, error wrapping, context handling", Body: "unused"},
				{Name: "security", Description: "Security-sensitive code paths", Body: "also unused"},
			}, nil, nil
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--list")
	require.Equal(t, ExitValid, code, stderr)
	// The full-object equality below already proves "unused" and "also
	// unused" never appear (refinery-emv.23): a separate NotContains on the
	// same stdout could never fail differently than this one already would.
	want := "{\n" +
		"  \"profiles\": [\n" +
		"    {\n" +
		"      \"name\": \"backend\",\n" +
		"      \"description\": \"Go services; concurrency, error wrapping, context handling\"\n" +
		"    },\n" +
		"    {\n" +
		"      \"name\": \"security\",\n" +
		"      \"description\": \"Security-sensitive code paths\"\n" +
		"    }\n" +
		"  ]\n" +
		"}\n"
	assert.Equal(t, want, stdout)
}

// docs/cli.md §2.1.5: a missing or empty profile directory is an empty
// list and exit 0, rendered as "profiles":[] and never "profiles":null
// (the deliberate fix internal/profile.Reader.List makes, pinned here on
// the CLI side too).
func TestPrimeListEmptySourceRendersEmptyArrayNotNull(t *testing.T) {
	t.Parallel()
	mock := &profileSourceMock{
		ListFunc: func() ([]profile.Profile, []string, error) { return []profile.Profile{}, nil, nil },
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--list")
	require.Equal(t, ExitValid, code, stderr)
	// The exact-equality assertion below already proves "null" never
	// appears (refinery-emv.23): a separate NotContains on the same stdout
	// could never fail differently than this one already would.
	assert.Equal(t, "{\n  \"profiles\": []\n}\n", stdout)
}

// TestPrimeListMatchesCliDocShape reproduces docs/cli.md §2.1.5's --list
// example for real and compares it, by shape, against the doc itself
// (docs_shape_test.go's pattern): an edited example and a drifted renderer
// both fail here, rather than only whatever this file happened to assert.
func TestPrimeListMatchesCliDocShape(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.profiles.ListFunc = func() ([]profile.Profile, []string, error) {
		return []profile.Profile{{Name: "backend", Description: "Go services; concurrency, error wrapping, context handling"}}, nil, nil
	}
	require.Equal(t, ExitValid, h.app.Run(t.Context(), []string{"prime", "--list"}))
	assertShapeMatchesDoc(t, realJSON(t, h.stdout.String()), cliDoc, "#### 2.1.5", 1,
		"docs/cli.md §2.1.5: --list's shape must match the documented example")
}

// docs/cli.md §2.1.5 (refinery-emv.18): a file that fails to parse is left
// out of the index rather than failing the whole call - --list answers
// "what profiles can I use", and a file that does not parse is not usable -
// but the failure is not silent: it is named on stderr, and the call still
// exits 0 with the valid profiles it did find.
func TestPrimeListOmitsBrokenProfilesButNamesThemOnStderr(t *testing.T) {
	t.Parallel()
	mock := &profileSourceMock{
		ListFunc: func() ([]profile.Profile, []string, error) {
			return []profile.Profile{{Name: "backend", Description: "d"}}, []string{"wip.md"}, nil
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--list")
	require.Equal(t, ExitValid, code, stderr)
	assert.Contains(t, stdout, `"backend"`, "a valid profile still appears in the index")
	assert.NotContains(t, stdout, "wip", "a broken profile must never appear in the index")
	assert.Contains(t, stderr, "wip.md", "a broken profile must be named on stderr")
}

// docs/cli.md §4: an unreadable profile directory is the tool's own state,
// not a usage error, and exits 101 with nothing on stdout - the split §2.1.4
// draws for one malformed profile applies the same way to the directory
// List reads. This replaces the "a source that errors" row from the old
// TestPrimeBareIsUnaffectedByProfileSource table (refinery-emv.22), whose
// configured error was unreachable by construction because bare prime never
// called List in the first place.
func TestPrimeListPropagatesADirectoryErrorAsExitTool(t *testing.T) {
	t.Parallel()
	mock := &profileSourceMock{
		ListFunc: func() ([]profile.Profile, []string, error) {
			return nil, nil, errors.New("reading /profiles: permission denied")
		},
	}
	code, stdout, stderr := newPrimeApp(t, mock, "--list")
	assert.Equal(t, ExitTool, code)
	assert.Empty(t, stdout, "a tool-error exit must print nothing on stdout")
	assert.Contains(t, stderr, "permission denied")
}

// docs/cli.md §6.1: prime --list's budget is an envelope plus a per-profile
// cost, the same shape as the reviews rows, and it is measured against
// synthetic profiles rather than the shipped profiles/ directory so that
// adding or trimming a shipped profile does not change what this test
// measures.
func TestPrimeListStaysWithinBudgetPerProfile(t *testing.T) {
	t.Parallel()
	const (
		baseBudget       = 10
		perProfileBudget = 40
	)
	emptyTokens := approxTokens(primeListOutput(t))
	assert.LessOrEqual(t, emptyTokens, baseBudget,
		"an empty profile index costs about %d tokens; the fixed overhead ceiling is %d", emptyTokens, baseBudget)
	assert.Equal(t, 5, emptyTokens, "the empty envelope's fixed overhead must not grow or shrink silently")
	smallTokens := approxTokens(primeListOutput(t, syntheticProfiles(3)...))
	largeTokens := approxTokens(primeListOutput(t, syntheticProfiles(10)...))
	perSmall := (smallTokens - emptyTokens) / 3
	perLarge := (largeTokens - smallTokens) / 7
	assert.LessOrEqual(t, perSmall, perProfileBudget,
		"each of the first 3 profiles costs about %d tokens; the per-profile ceiling is %d", perSmall, perProfileBudget)
	assert.LessOrEqual(t, perLarge, perProfileBudget,
		"each of profiles 4 through 10 costs about %d tokens; the per-profile ceiling is %d", perLarge, perProfileBudget)
}

// primeListOutput drives a real prime --list through the real JSON renderer
// and returns stdout, so the budget test above measures actual encoding
// rather than a hand-built string.
func primeListOutput(t *testing.T, profiles ...profile.Profile) string {
	t.Helper()
	mock := &profileSourceMock{ListFunc: func() ([]profile.Profile, []string, error) { return profiles, nil, nil }}
	code, stdout, stderr := newPrimeApp(t, mock, "--list")
	require.Equal(t, ExitValid, code, stderr)
	return stdout
}

// syntheticProfiles returns n profiles with a name and description
// representative of a real profile's length, so the budget test above
// measures a realistic per-profile cost rather than a best case.
func syntheticProfiles(n int) []profile.Profile {
	out := make([]profile.Profile, 0, n)
	for i := range n {
		out = append(out, profile.Profile{
			Name:        fmt.Sprintf("profile-%d", i),
			Description: fmt.Sprintf("A representative profile description, long enough to look real, item %d", i),
		})
	}
	return out
}
