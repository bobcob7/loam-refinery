package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/bobcob7/refinery/internal/review"
	"github.com/bobcob7/refinery/internal/validate"
)

const validateUsage = `usage: refinery validate [path] [--strict] [--warn-only=NAME,...] [--disable=NAME,...] [--format text|json]
`

// validate checks one review document. Every check runs: a failure in one tier
// never gates the next, so one call reports everything findable.
func (a *App) validate(ctx context.Context, args []string) int {
	set := a.flagSet("validate", validateUsage)
	strict := set.Bool("strict", false, "treat advisories as errors")
	warnOnly := set.String("warn-only", "", "demote the named verification checks")
	disable := set.String("disable", "", "skip the named advisories")
	format := set.String("format", "text", "output format: text or json")
	paths, err := parseAnywhere(set, args)
	if err != nil {
		return usageOrHelp(err)
	}
	renderer, err := a.renderer(*format)
	if err != nil {
		a.fail(err)
		return ExitUsage
	}
	if len(paths) > 1 {
		a.fail(fmt.Errorf("validate takes at most one path, got %d", len(paths)))
		return ExitUsage
	}
	options := validate.Options{Strict: *strict, Dir: a.dir}
	if options.Disabled, err = a.checkNames(*disable, "--disable", a.names.Advisory, isSet(set, "disable")); err != nil {
		a.fail(err)
		return ExitUsage
	}
	if options.WarnOnly, err = a.checkNames(*warnOnly, "--warn-only", a.names.Verification, isSet(set, "warn-only")); err != nil {
		a.fail(err)
		return ExitUsage
	}
	path := ""
	if len(paths) == 1 {
		path = paths[0]
	}
	source, err := a.read(path)
	if err != nil {
		a.fail(err)
		return ExitUsage
	}
	result, err := a.validator.Validate(ctx, source, options)
	if err != nil {
		if !review.IsDocumentError(err) {
			a.fail(err)
			return ExitUsage
		}
		result = unparseable(err)
	}
	if err := renderer.Result(a.stdout, a.stderr, result); err != nil {
		a.fail(err)
		return ExitUsage
	}
	if result.Valid {
		return ExitValid
	}
	return ExitInvalid
}

// unparseable turns a document that never parsed into a result the renderer can
// express. The alternative is prose written past the renderer, which leaves
// --format=json exiting 1 with nothing on stdout: a caller unmarshalling that
// sees a crashed tool rather than a document to repair. Going through the
// renderer also keeps the promise prime makes, that an exit 1 names a check and
// hands back the describe command for it.
func unparseable(err error) *review.Result {
	return &review.Result{
		Diagnostics: []review.Diagnostic{{
			Severity: review.SeverityError,
			Name:     "document-unparseable",
			Message:  err.Error(),
		}},
		Verification: review.Verification{Source: "none", Reason: "the input is not a review document"},
	}
}

// checkNames validates a comma-separated list of check names against the tier
// that accepts them. A typo is a usage error rather than a silent no-op.
func (a *App) checkNames(value, flagName string, allowed []string, given bool) (map[string]bool, error) {
	if !given {
		return nil, nil
	}
	names, err := splitNames(value)
	if errors.Is(err, errNoNames) {
		return nil, fmt.Errorf("%s needs at least one check name", flagName)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", flagName, err)
	}
	selected := map[string]bool{}
	for _, name := range names {
		switch {
		case contains(allowed, name):
			selected[name] = true
		case contains(a.names.Structural, name):
			return nil, fmt.Errorf("%s: structural checks cannot be disabled or demoted (%s)", flagName, name)
		case contains(a.names.Advisory, name):
			return nil, fmt.Errorf("%s: %s is an advisory; advisories never fail a run, use --disable to silence it", flagName, name)
		case contains(a.names.Verification, name):
			return nil, fmt.Errorf("%s: %s is a verification check; it cannot be disabled, use --warn-only to demote it", flagName, name)
		default:
			return nil, fmt.Errorf("%s: unknown check %q", flagName, name)
		}
	}
	return selected, nil
}

// read takes the document from a path, or from stdin for "-" and no path.
func (a *App) read(path string) ([]byte, error) {
	if path == "" || path == "-" {
		source, err := io.ReadAll(a.stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		return source, nil
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return source, nil
}
