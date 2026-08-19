package cli

import "fmt"

// version prints the build and the format version it enforces, one per line.
func (a *App) version(args []string) int {
	set := a.flagSet("version", "usage: loam-refinery version\n")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("version takes no arguments"))
		return ExitUsage
	}
	fmt.Fprintf(a.stdout, "loam-refinery %s\ncommit %s\nschema %s\n", a.build.Version, a.build.Commit, a.build.Schema)
	return ExitValid
}
