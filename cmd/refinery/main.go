// Command refinery checks a review document: is it well-formed, and where does
// it fall short of being worth acting on.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/bobcob7/refinery/internal/advisory"
	"github.com/bobcob7/refinery/internal/cli"
	"github.com/bobcob7/refinery/internal/entry"
	"github.com/bobcob7/refinery/internal/render"
	"github.com/bobcob7/refinery/internal/review"
	"github.com/bobcob7/refinery/internal/schema"
	"github.com/bobcob7/refinery/internal/structural"
	"github.com/bobcob7/refinery/internal/validate"
	"github.com/bobcob7/refinery/internal/verify"
)

// version is set at build time with -ldflags.
var version = "0.1.0-dev"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	log := newLogger(stderr)
	app, err := wire(log, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "refinery: %v\n", err)
		return cli.ExitUsage
	}
	return app.Run(ctx, args)
}

// newLogger keeps output terse by default; REFINERY_DEBUG turns on tracing.
func newLogger(stderr io.Writer) *slog.Logger {
	if os.Getenv("REFINERY_DEBUG") == "" {
		return slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// wire builds the whole dependency graph. Nothing here is package state.
func wire(log *slog.Logger, stdin io.Reader, stdout, stderr io.Writer) (*cli.App, error) {
	schemaValidator, err := schema.NewValidator()
	if err != nil {
		return nil, err
	}
	schemaProvider, err := entry.NewSchemaProvider(schema.Annotated())
	if err != nil {
		return nil, err
	}
	registry, err := entry.NewRegistry(
		schemaProvider,
		entry.NewChecksProvider(structural.Checks(), verify.Checks(), advisory.Checks()),
		entry.NewTopicsProvider(),
	)
	if err != nil {
		return nil, err
	}
	validator := validate.New(
		structural.New(schemaValidator, log),
		advisory.New(log, advisory.All()),
		validate.NewGitFinder(log),
		log,
	)
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("finding the working directory: %w", err)
	}
	return cli.New(
		validator,
		registry,
		render.NewText(),
		render.NewJSON(),
		checkNames(),
		cli.Build{Version: version, Commit: commit(), Schema: schema.Version()},
		schemaText,
		dir,
		stdin,
		stdout,
		stderr,
		log,
	), nil
}

func checkNames() cli.CheckNames {
	return cli.CheckNames{
		Structural:   names(structural.Checks()),
		Verification: names(verify.Checks()),
		Advisory:     names(advisory.Checks()),
	}
}

func names(checks []review.Check) []string {
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		out = append(out, check.Name)
	}
	return out
}

func schemaText(annotated bool) ([]byte, error) {
	if annotated {
		return schema.Annotated(), nil
	}
	return schema.Minimal()
}

func commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return "unknown"
}
