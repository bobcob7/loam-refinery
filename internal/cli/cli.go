// Package cli implements the subcommands. Flag parsing, wiring and exit codes
// live in main; everything a subcommand does lives here.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Exit codes. 0 says the document is usable, 1 says revise the review, 2 says
// fix the invocation, and a caller must be able to tell 1 from 2 without
// parsing prose.
const (
	ExitValid   = 0
	ExitInvalid = 1
	ExitUsage   = 2
)

// Build is what version reports.
type Build struct {
	Version string
	Commit  string
	Schema  string
}

// CheckNames are the registered check names, by tier, for validating
// --disable and --warn-only.
type CheckNames struct {
	Structural   []string
	Verification []string
	Advisory     []string
}

// App holds the wired dependencies for one process.
type App struct {
	validator  documentValidator
	registry   entryRegistry
	renderers  map[string]renderer
	names      CheckNames
	build      Build
	dir        string
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	log        *slog.Logger
	schemaText func(annotated bool) ([]byte, error)
}

// New wires an App. Everything it needs is passed in; nothing is package state.
func New(
	validator documentValidator,
	registry entryRegistry,
	text renderer,
	structured renderer,
	names CheckNames,
	build Build,
	schemaText func(annotated bool) ([]byte, error),
	dir string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	log *slog.Logger,
) *App {
	return &App{
		validator:  validator,
		registry:   registry,
		renderers:  map[string]renderer{"text": text, "json": structured},
		names:      names,
		build:      build,
		schemaText: schemaText,
		dir:        dir,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		log:        log,
	}
}

const usage = `refinery — check a review document

  refinery prime                       the workflow, in one small call
  refinery describe [--lens=NAME,...]  the contract, disclosed on demand
  refinery validate [path]             check a review (- or omitted: stdin)
  refinery schema [--annotated]        JSON Schema, for machines
  refinery version

exit 0 valid, 1 revise the review, 2 fix the invocation
`

// Run dispatches one invocation and returns its exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.stderr, usage)
		return ExitUsage
	}
	command, rest := args[0], args[1:]
	switch command {
	case "prime":
		return a.prime(rest)
	case "describe":
		return a.describe(rest)
	case "validate":
		return a.validate(ctx, rest)
	case "schema":
		return a.schema(rest)
	case "version":
		return a.version(rest)
	case "-h", "--help", "help":
		fmt.Fprint(a.stdout, usage)
		return ExitValid
	default:
		a.fail(fmt.Errorf("unknown command %q", command))
		fmt.Fprint(a.stderr, usage)
		return ExitUsage
	}
}

func (a *App) flagSet(name, use string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(a.stderr)
	set.Usage = func() {
		fmt.Fprint(a.stderr, use)
		set.PrintDefaults()
	}
	return set
}

func (a *App) fail(err error) {
	fmt.Fprintf(a.stderr, "refinery: %v\n", err)
}

// renderer returns the renderer for a --format value.
func (a *App) renderer(format string) (renderer, error) {
	chosen, ok := a.renderers[format]
	if !ok {
		return nil, fmt.Errorf("unknown format %q: expected text or json", format)
	}
	return chosen, nil
}

// splitNames parses a comma-separated flag value, rejecting an empty element so
// a typo surfaces instead of being ignored.
func splitNames(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("empty value")
	}
	names := []string{}
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("empty name in %q", value)
		}
		names = append(names, trimmed)
	}
	return names, nil
}

func contains(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}
