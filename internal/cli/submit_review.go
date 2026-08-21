package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/validate"
)

const submitReviewUsage = `usage: loam-refinery submit-review [path] [--strict] [--require-verification] [--warn-only=NAME,...] [--disable=NAME,...] [--format json]
`

// submitReview checks one review document. Every check runs: a failure in one
// tier never gates the next, so one call reports everything findable.
func (a *App) submitReview(ctx context.Context, args []string) int {
	set := a.flagSet("submit-review", submitReviewUsage)
	strict := set.Bool("strict", false, "treat advisories as errors")
	warnOnly := set.String("warn-only", "", "demote the named verification checks")
	disable := set.String("disable", "", "skip the named advisories")
	require := set.Bool("require-verification", false, "fail if the anchors were not checked")
	format := set.String("format", "json", "output format: json")
	paths, err := parseAnywhere(set, args)
	if err != nil {
		return usageOrHelp(err)
	}
	if err := a.checkFormat(*format); err != nil {
		a.fail(err)
		return ExitUsage
	}
	if len(paths) > 1 {
		a.fail(fmt.Errorf("submit-review takes at most one path, got %d", len(paths)))
		return ExitUsage
	}
	options := validate.Options{Strict: *strict, RequireVerification: *require, Dir: a.dir}
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
		result = a.unparseable(err, *strict)
	}
	// Storing happens before rendering, and its failure preempts rendering
	// entirely: a store that could not be established, written, or recorded
	// to exits ExitTool with nothing on stdout (docs/config.md §5.1), and an
	// object printed first would already have said something.
	if err := a.store.Save(ctx, a.storeInput(source, result)); err != nil {
		a.fail(err)
		return ExitTool
	}
	if err := a.renderer.Result(a.stdout, result); err != nil {
		a.fail(err)
		return ExitUsage
	}
	if result.Valid {
		return ExitValid
	}
	return ExitInvalid
}

// storeInput builds what documentStore needs to place and record this run
// (docs/config.md §5). Ref and Verdict are not on result — re-parsing source
// is what reads them, which is cheap and safe because Parse is pure: it
// already succeeded once inside the validator for every result reaching
// this call, or it fails again the same way and leaves both empty.
func (a *App) storeInput(source []byte, result *review.Result) StoreInput {
	in := StoreInput{
		Dir:           a.dir,
		Source:        source,
		Valid:         result.Valid,
		Comments:      result.Comments,
		Errors:        result.Errors(),
		Advisories:    result.Advisories(),
		Skipped:       len(result.Skipped),
		ToolVersion:   a.build.Version,
		SchemaVersion: a.build.Schema,
	}
	doc, err := review.Parse(source)
	if err != nil {
		return in
	}
	if doc.Ref.OK {
		in.Ref = doc.Ref.Value
	}
	if doc.Verdict.OK {
		in.Verdict = doc.Verdict.Value
	}
	return in
}

// checkDocumentUnparseable is the check this diagnostic reports under. It is
// the registry's name, asserted against it in the tests, because prime promises
// the run hands back a describe command that works and a name that drifts from
// the registry turns that promise into a lens lookup that exits 2.
const checkDocumentUnparseable = "document-unparseable"

// unparseable turns a document that never parsed into a result the renderer can
// express. The alternative is prose written past the renderer, which leaves
// --format=json exiting 1 with nothing on stdout: a caller unmarshalling that
// sees a crashed tool rather than a document to repair. Going through the
// renderer also keeps the promise prime makes, that an exit 1 names a check and
// hands back the describe command for it.
func (a *App) unparseable(err error, strict bool) *review.Result {
	const reason = "the input is not a review document"
	result := &review.Result{
		Strict: strict,
		Diagnostics: []review.Diagnostic{{
			Severity: review.SeverityError,
			Name:     checkDocumentUnparseable,
			Message:  err.Error(),
		}},
		Verification: review.Verification{Source: "unavailable", Reason: reason},
	}
	// Every other check is reported as skipped rather than left out, because a
	// caller counting "registered minus reported" would otherwise read silence
	// as twenty-odd checks passing on a document that never parsed.
	for _, tier := range [][]string{a.names.Structural, a.names.Verification, a.names.Advisory} {
		for _, name := range tier {
			if name == checkDocumentUnparseable {
				continue
			}
			result.Skipped = append(result.Skipped, review.Skipped{Name: name, Reason: reason})
		}
	}
	return result
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
