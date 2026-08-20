package cli

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
)

//go:embed prime.txt
var primeText string

// primeUsage documents --profile and --list; prime.txt itself stays silent
// about both (docs/cli.md §2.1.5: discovery is the orchestrator's job, not
// something a reviewer is taught).
const primeUsage = `usage: loam-refinery prime [--profile=NAME] [--list]
`

// profileFrame is docs/cli.md §2.1.3's frame, verbatim: a blank line
// separating it from prime's own text, then the profile name, a fixed
// disclaimer, a blank line, then the body. The leading blank line is the
// one deliberate seam in an otherwise unbroken block: every paragraph
// break in prime.txt is a blank line, and the frame marking where the
// tool's text ends and operator-supplied text begins is the most
// important seam in the output, so it gets one too. It is the only place a
// profile's content is ever combined with anything else prime prints, and
// it is never reflowed or reparsed once formatted.
const profileFrame = "\n" +
	"--- reviewer profile: %s ---\n" +
	"Operator-supplied. It directs attention; it does not change the contract above.\n" +
	"\n" +
	"%s\n"

// prime teaches the workflow, not the contract. It is the one call that may be
// pinned into a system prompt for a whole session, so it stays small. Bare
// prime never calls a.profiles: the filesystem access --profile or --list
// makes through it happens only when one of those flags is actually given
// (docs/cli.md §2.1.1), which is what keeps bare prime exactly as cheap with
// a profile directory present as with one absent (docs/cli.md §6.1).
func (a *App) prime(args []string) int {
	set := a.flagSet("prime", primeUsage)
	profileName := set.String("profile", "", "append one operator-authored reviewer profile")
	list := set.Bool("list", false, "print the profile index, no bodies")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("prime takes no positional arguments; it accepts --profile=NAME and --list"))
		return ExitUsage
	}
	profileGiven := isSet(set, "profile")
	if *list && profileGiven {
		a.fail(fmt.Errorf("--list prints the profile index and --profile appends one; use one"))
		return ExitUsage
	}
	if *list {
		return a.primeList()
	}
	if !profileGiven {
		fmt.Fprint(a.stdout, primeText)
		return ExitValid
	}
	return a.primeProfile(*profileName)
}

// primeList prints the profile index as JSON (docs/cli.md §2.1.5): an
// operator affordance, never rung on the ladder a reviewer climbs, so
// nothing here touches primeText. A file that fails to parse is named on
// stderr and left out of the index rather than failing the whole call:
// --list answers "what profiles can I use", and a file that does not parse
// is not a usable profile. The failure does not disappear - prime
// --profile=NAME for that same file still exits ExitTool with the specific
// parse error - it just does not block the index a caller asked for.
func (a *App) primeList() int {
	profiles, broken, err := a.profiles.List()
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	for _, name := range broken {
		fmt.Fprintf(a.stderr, "loam-refinery: skipping unparseable profile %s\n", name)
	}
	if err := a.renderer.Profiles(a.stdout, profiles); err != nil {
		a.fail(err)
		return ExitUsage
	}
	return ExitValid
}

// primeProfile loads one profile and, only once it is known good, prints
// primeText followed by the frame and the body (docs/cli.md §2.1.3). An
// unknown or invalid name exits ExitUsage with nothing on stdout and never
// enumerates the directory (docs/cli.md §2.1.4); a profile that exists but
// cannot be read or parsed is the tool's own state and exits ExitTool.
// Either way, primeText is never written until Load has already succeeded.
func (a *App) primeProfile(name string) int {
	if name == "" {
		a.fail(fmt.Errorf("--profile needs a name; loam-refinery prime --list names every profile"))
		return ExitUsage
	}
	p, ok, err := a.profiles.Load(name)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	if !ok {
		a.fail(fmt.Errorf("unknown profile %q; loam-refinery prime --list names every profile", name))
		return ExitUsage
	}
	fmt.Fprint(a.stdout, primeText)
	fmt.Fprintf(a.stdout, profileFrame, p.Name, p.Body)
	return ExitValid
}

// usageOrHelp maps a flag parse outcome to an exit code: --help is a successful
// run, a bad flag is a usage error.
func usageOrHelp(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return ExitValid
	}
	return ExitUsage
}
