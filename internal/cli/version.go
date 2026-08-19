package cli

import "fmt"

// version prints the build and the format version it enforces, one per line.
func (a *App) version(args []string) int {
	set := a.flagSet("version", "usage: refinery version\n")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	fmt.Fprintf(a.stdout, "refinery %s\ncommit %s\nschema %s\n", a.build.Version, a.build.Commit, a.build.Schema)
	return ExitValid
}
