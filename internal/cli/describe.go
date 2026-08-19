package cli

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/bobcob7/refinery/internal/entry"
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

const describeUsage = `usage: refinery describe [--lens=NAME,...] [--list] [--format text|json]
`

// describe explains the contract, disclosed progressively: the summary, one
// entry in full, or the index alone.
func (a *App) describe(args []string) int {
	set := a.flagSet("describe", describeUsage)
	lens := set.String("lens", "", "open one entry in full, comma separated")
	list := set.Bool("list", false, "print the lens index, no bodies")
	format := set.String("format", "text", "output format: text or json")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	renderer, err := a.renderer(*format)
	if err != nil {
		a.fail(err)
		return ExitUsage
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("describe takes no arguments; did you mean --lens=%s?", set.Arg(0)))
		return ExitUsage
	}
	if *list {
		if err := renderer.Index(a.stdout, a.registry.Index()); err != nil {
			a.fail(err)
			return ExitUsage
		}
		return ExitValid
	}
	if !isSet(set, "lens") {
		return a.summary(*format, renderer)
	}
	entries, code := a.resolveLenses(*lens)
	if code != ExitValid {
		return code
	}
	if err := renderer.Entries(a.stdout, entries); err != nil {
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
	if err != nil {
		a.fail(fmt.Errorf("--lens needs at least one name"))
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
			a.fail(fmt.Errorf("unknown lens %q", unknown.Name))
			if err := a.renderers["text"].Index(a.stderr, a.registry.Index()); err != nil {
				a.fail(err)
			}
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

// summary prints the fixed document shape, then the lens index.
func (a *App) summary(format string, renderer renderer) int {
	if format == "json" {
		payload := struct {
			Summary string              `json:"summary"`
			Index   map[string][]string `json:"index"`
		}{Summary: describeText, Index: map[string][]string{}}
		for _, group := range a.registry.Index() {
			payload.Index[string(group.Namespace)] = group.Names
		}
		encoder := json.NewEncoder(a.stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			a.fail(err)
			return ExitUsage
		}
		return ExitValid
	}
	fmt.Fprint(a.stdout, describeText)
	fmt.Fprintln(a.stdout)
	if err := renderer.Index(a.stdout, a.registry.Index()); err != nil {
		a.fail(err)
		return ExitUsage
	}
	return ExitValid
}
