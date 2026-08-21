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

// TestReviewsFlagSetIsExactlyTheSevenDocumentedFlags pins docs/config.md
// §6's flag table: reviews accepts --repo, --ref, --limit, --content,
// --failed, --list, and --format, and nothing else. Individually testing
// that each of the seven works — internal/cli/reviews_test.go already
// does — does not notice an eighth flag added beside them; walking the
// actual FlagSet and comparing the whole set does.
func TestReviewsFlagSetIsExactlyTheSevenDocumentedFlags(t *testing.T) {
	t.Parallel()
	want := []string{"content", "failed", "format", "limit", "list", "ref", "repo"}
	sort.Strings(want)
	assert.Equal(t, want, flagNames(t, "reviews"), "docs/config.md §6: reviews accepts exactly these seven flags")
}

// TestSubmitReviewFlagSetIsExactlyTheDocumentedFlags pins docs/cli.md §3's
// table for submit-review: --strict, --warn-only, --disable,
// --require-verification, and --format — the same five whether read from
// §3's table or from §2.3's prose, and, per §2.3, "no flag for it" about
// storing: nothing here names a store setting.
func TestSubmitReviewFlagSetIsExactlyTheDocumentedFlags(t *testing.T) {
	t.Parallel()
	want := []string{"disable", "format", "require-verification", "strict", "warn-only"}
	sort.Strings(want)
	assert.Equal(t, want, flagNames(t, "submit-review"), "docs/cli.md §3: submit-review accepts exactly these five flags, and none about storing")
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
