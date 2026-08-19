package cli

import "fmt"

const schemaUsage = `usage: refinery schema [--annotated]
`

// schema writes the grammar for machines: minimal by default, annotated for
// codegen that wants doc comments on generated types.
func (a *App) schema(args []string) int {
	set := a.flagSet("schema", schemaUsage)
	annotated := set.Bool("annotated", false, "emit the schema with descriptions intact")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("schema takes no arguments"))
		return ExitUsage
	}
	text, err := a.schemaText(*annotated)
	if err != nil {
		a.fail(err)
		return ExitUsage
	}
	if _, err := a.stdout.Write(text); err != nil {
		a.fail(err)
		return ExitUsage
	}
	return ExitValid
}
