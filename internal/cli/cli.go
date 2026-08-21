// Package cli implements the subcommands. Flag parsing, wiring and exit codes
// live in main; everything a subcommand does lives here.
package cli

import (
	"context"
	"errors"
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
	// ExitTool marks the tool-error band: the tool's own state could not be
	// read or written, and neither revising the review nor fixing the
	// command line will help. It covers a config file that could not be
	// read or parsed, a store that could not be created or written, and a
	// store directory that could not be read. Everything the caller typed
	// still exits ExitUsage; ExitTool is for what the caller did not type.
	// Codes 102-125 are reserved for finer distinctions within the same
	// band and are unassigned today.
	ExitTool = 101
)

// Build is what version reports.
type Build struct {
	Version string
	Commit  string
	Schema  string
}

// CheckNames are the registered check names, by tier — used to enumerate
// every check as skipped when a document never parsed (see unparseable).
type CheckNames struct {
	Structural   []string
	Verification []string
	Advisory     []string
}

// App holds the wired dependencies for one process.
type App struct {
	validator   documentValidator
	store       documentStore
	reviewStore reviewStore
	profiles    profileSource
	registry    entryRegistry
	renderer    renderer
	names       CheckNames
	build       Build
	dir         string
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	log         *slog.Logger
	schemaText  func(annotated bool) ([]byte, error)
}

// New wires an App. Everything it needs is passed in; nothing is package state.
func New(
	validator documentValidator,
	store documentStore,
	reviewStore reviewStore,
	profiles profileSource,
	registry entryRegistry,
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
		validator:   validator,
		store:       store,
		reviewStore: reviewStore,
		profiles:    profiles,
		registry:    registry,
		renderer:    structured,
		names:       names,
		build:       build,
		schemaText:  schemaText,
		dir:         dir,
		stdin:       stdin,
		stdout:      stdout,
		stderr:      stderr,
		log:         log,
	}
}

const usage = `loam-refinery — check a review document

  loam-refinery prime [--profile=NAME] [--list]  the workflow, in one small call
  loam-refinery describe [--lens=NAME,...]       the contract, disclosed on demand
  loam-refinery submit-review [path]             check a review (- or omitted: stdin)
  loam-refinery reviews [--repo=NAME]            what an earlier submit-review stored
  loam-refinery schema [--annotated]             JSON Schema, for machines
  loam-refinery version

exit 0 valid, 1 revise the review, 2 fix the invocation, 101 the tool failed
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
	case "submit-review":
		return a.submitReview(ctx, rest)
	case "reviews":
		return a.reviews(ctx, rest)
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
	fmt.Fprintf(a.stderr, "loam-refinery: %v\n", err)
}

// checkFormat accepts the one format there is. The flag outlived the choice it
// used to make: keeping it means every caller already passing --format=json
// keeps working, and a caller passing --format=text is told what happened
// rather than being handed an unknown-flag error to guess at.
func (a *App) checkFormat(format string) error {
	if format == "json" {
		return nil
	}
	if format == "text" {
		return fmt.Errorf("the text format is gone; json is the only format, and --format=json or no flag at all selects it")
	}
	return fmt.Errorf("unknown format %q: json is the only format", format)
}

// errNoNames marks a flag given with nothing in it, which each caller names in
// its own vocabulary. An empty element inside a list is a different mistake and
// keeps its own message.
var errNoNames = errors.New("the list is empty")

// parseAnywhere parses flags that appear after the positional argument, which
// the flag package otherwise stops at. "submit-review review.json --strict" is
// the order a person writes, and rejecting it teaches nothing.
func parseAnywhere(set *flag.FlagSet, args []string) ([]string, error) {
	positional := []string{}
	for {
		if err := set.Parse(args); err != nil {
			return nil, err
		}
		if set.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, set.Arg(0))
		args = set.Args()[1:]
	}
}

// splitNames parses a comma-separated flag value, rejecting an empty element so
// a typo surfaces instead of being ignored.
func splitNames(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, errNoNames
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
