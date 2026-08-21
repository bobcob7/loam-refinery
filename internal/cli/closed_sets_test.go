package cli

import (
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flagLine matches one flag entry in flag.FlagSet.PrintDefaults' stable
// output format:
//
//	-content
//	  	include each stored file, not just its row
var flagLine = regexp.MustCompile(`(?m)^  -([A-Za-z][A-Za-z0-9-]*)`)

// flagNames walks the FlagSet a subcommand actually registers by reading
// its own -h output, rather than trusting a hand-kept list of what a
// person believes it registers: -h's usage text comes from
// flag.FlagSet.PrintDefaults, which visits every flag on the set, so a
// flag added to a command's flagSet call and never named here is exactly
// the addition this is supposed to catch (refinery-a96.23). -h always
// exits ExitValid (see usageOrHelp), never the command body, so this never
// touches the reviewStore or documentValidator mocks either.
func flagNames(t *testing.T, command string) []string {
	t.Helper()
	h := newHarness(t, "")
	code := h.app.Run(t.Context(), []string{command, "-h"})
	require.Equal(t, ExitValid, code, h.stderr.String())
	matches := flagLine.FindAllStringSubmatch(h.stderr.String(), -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}

// TestReviewsFlagSetIsExactlyTheDocumentedFlags pins docs/cli.md §3's flag
// table: reviews accepts --repo, --ref, --limit, --content, --failed, and
// --list, and nothing else — --format is gone (refinery-uyb.4;
// docs/cli.md §5.1). Individually testing that each flag works —
// internal/cli/reviews_test.go already does — does not notice an extra
// flag added beside them; walking the actual FlagSet and comparing the
// whole set does.
func TestReviewsFlagSetIsExactlyTheDocumentedFlags(t *testing.T) {
	t.Parallel()
	want := []string{"content", "failed", "limit", "list", "ref", "repo"}
	sort.Strings(want)
	assert.Equal(t, want, flagNames(t, "reviews"), "docs/cli.md §3: reviews accepts exactly these six flags")
}

// TestSubmitReviewFlagSetIsExactlyTheDocumentedFlags pins docs/cli.md §3's
// table for submit-review: --strict alone. refinery-uyb.5 dropped
// --warn-only, --disable, and --require-verification (docs/cli.md §2.3.1,
// §3; docs/features/combined-reviews.md §3.3, "No disable, no warn-only");
// refinery-uyb.4 dropped --format, the flag's only remaining companion
// (docs/cli.md §5.1, "--format exists where there is a format to
// choose").
func TestSubmitReviewFlagSetIsExactlyTheDocumentedFlags(t *testing.T) {
	t.Parallel()
	want := []string{"strict"}
	assert.Equal(t, want, flagNames(t, "submit-review"), "docs/cli.md §3: submit-review accepts exactly --strict, and nothing about storing or format")
}

// TestDescribeFlagSetIsExactlyTheDocumentedFlags pins docs/cli.md §3's
// table for describe: --lens and --list, and nothing else — no --format
// (refinery-uyb.4; docs/cli.md §5.1), the same removal made to
// submit-review and reviews.
func TestDescribeFlagSetIsExactlyTheDocumentedFlags(t *testing.T) {
	t.Parallel()
	want := []string{"lens", "list"}
	sort.Strings(want)
	assert.Equal(t, want, flagNames(t, "describe"), "docs/cli.md §3: describe accepts exactly --lens and --list")
}

// TestPrimeFlagSetIsExactlyTheDocumentedFlags pins docs/cli.md §3's table
// for prime: --profile and --list, and nothing else - no --format, since
// prime is prose rather than the one JSON renderer (docs/cli.md §5.1).
func TestPrimeFlagSetIsExactlyTheDocumentedFlags(t *testing.T) {
	t.Parallel()
	want := []string{"list", "profile"}
	sort.Strings(want)
	assert.Equal(t, want, flagNames(t, "prime"), "docs/cli.md §3: prime accepts exactly --profile and --list")
}
