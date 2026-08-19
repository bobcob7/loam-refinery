package cli

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
)

//go:embed prime.txt
var primeText string

// prime teaches the workflow, not the contract. It is the one call that may be
// pinned into a system prompt for a whole session, so it stays small.
func (a *App) prime(args []string) int {
	set := a.flagSet("prime", "usage: loam-refinery prime\n")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("prime takes no arguments"))
		return ExitUsage
	}
	fmt.Fprint(a.stdout, primeText)
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
