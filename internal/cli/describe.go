package cli

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/bobcob7/loam-refinery/internal/entry"
)

// isSet reports whether a flag was given, distinguishing --lens= from no --lens
// at all: the empty form is a usage error, the absent form prints the summary.
func isSet(set *flag.FlagSet, name string) bool {
	given := false
	set.Visit(func(f *flag.Flag) {
		if f.Name == name {
			given = true
		}
	})
	return given
}

//go:embed describe.txt
var describeText string

const describeUsage = `usage: loam-refinery describe [--lens=NAME,...] [--list] [--format json]
`

// describe explains the contract, disclosed progressively: the summary, one
// entry in full, or the index alone.
func (a *App) describe(args []string) int {
	set := a.flagSet("describe", describeUsage)
	lens := set.String("lens", "", "open one entry in full, comma separated")
	list := set.Bool("list", false, "print the lens index, no bodies")
	format := set.String("format", "json", "output format: json")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	if err := a.checkFormat(*format); err != nil {
		a.fail(err)
		return ExitUsage
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("describe takes no arguments; did you mean --lens=%s?", set.Arg(0)))
		return ExitUsage
	}
	if *list {
		if isSet(set, "lens") {
			a.fail(fmt.Errorf("--list prints the index and --lens opens an entry; use one"))
			return ExitUsage
		}
		if err := a.renderer.Index(a.stdout, a.registry.Index()); err != nil {
			a.fail(err)
			return ExitUsage
		}
		return ExitValid
	}
	if !isSet(set, "lens") {
		return a.summary()
	}
	entries, code := a.resolveLenses(*lens)
	if code != ExitValid {
		return code
	}
	if err := a.renderer.Entries(a.stdout, entries); err != nil {
		a.fail(err)
		return ExitUsage
	}
	return ExitValid
}

// resolveLenses turns a --lens value into entries, deduplicated and in the
// order given. An unknown name prints the index; an ambiguous one prints the
// qualified candidates. Neither is ever guessed at.
func (a *App) resolveLenses(value string) ([]entry.Entry, int) {
	names, err := splitNames(value)
	if errors.Is(err, errNoNames) {
		a.fail(fmt.Errorf("--lens needs at least one name"))
		return nil, ExitUsage
	}
	if err != nil {
		a.fail(fmt.Errorf("--lens: %w", err))
		return nil, ExitUsage
	}
	entries := []entry.Entry{}
	seen := map[string]bool{}
	for _, name := range names {
		resolved, err := a.registry.Resolve(name)
		var ambiguous *entry.AmbiguousLensError
		var unknown *entry.UnknownLensError
		switch {
		case errors.As(err, &ambiguous):
			a.fail(fmt.Errorf("ambiguous lens %q; ask for one of", ambiguous.Name))
			fmt.Fprintf(a.stderr, "%s\n", strings.Join(ambiguous.Candidates, "\n"))
			return nil, ExitUsage
		case errors.As(err, &unknown):
			// The index used to be dumped here. With one format that would put a
			// JSON object on the same stream as the prose above it, leaving
			// neither parseable; the name of the command that prints it costs a
			// caller one more call and keeps both streams honest.
			a.fail(fmt.Errorf("unknown lens %q; loam-refinery describe --list names every lens", unknown.Name))
			return nil, ExitUsage
		case err != nil:
			a.fail(err)
			return nil, ExitUsage
		}
		if seen[resolved.Qualified()] {
			continue
		}
		seen[resolved.Qualified()] = true
		entries = append(entries, resolved)
	}
	return entries, ExitValid
}

// summary prints the fixed document shape, then the lens index. describeText
// stays prose inside its field: it is the teaching surface, written to be read,
// and encoding it does not make it a data structure.
func (a *App) summary() int {
	if err := a.renderer.Summary(a.stdout, describeText, a.registry.Index()); err != nil {
		a.fail(err)
		return ExitUsage
	}
	return ExitValid
}
