package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/bobcob7/loam-refinery/internal/store"
)

const reviewsUsage = `usage: loam-refinery reviews [--repo=NAME] [--ref=SHA] [--limit=N] [--content] [--failed] [--list] [--format json]
`

// reviewsExclusiveFlags are every flag docs/config.md §6 forbids alongside
// --list: it answers a different question from every other form and takes
// no company.
var reviewsExclusiveFlags = []string{"repo", "ref", "limit", "content", "failed"}

// repoInfo names the repository a reviews query answered about, and whether
// the store has ever heard of it (docs/config.md §6.2).
type repoInfo struct {
	Name  string `json:"name"`
	Known bool   `json:"known"`
}

// countsJSON mirrors store.Counts field for field, so the two convert
// directly: a nil counter is omitted rather than printed as null, the same
// absence-is-the-signal rule docs/config.md §6.1 applies to ref and path.
type countsJSON struct {
	Comments   *int `json:"comments,omitempty"`
	Errors     *int `json:"errors,omitempty"`
	Advisories *int `json:"advisories,omitempty"`
	Skipped    *int `json:"skipped,omitempty"`
}

// reviewRow is one row of the default index (docs/config.md §6.1). Every
// passing run has a ref, a digest, a verdict, and a computed path, so none
// of those fields is ever omitted; Review is added only under --content.
type reviewRow struct {
	At      string          `json:"at"`
	Ref     string          `json:"ref"`
	Digest  string          `json:"digest"`
	Verdict string          `json:"verdict"`
	Counts  countsJSON      `json:"counts"`
	Path    string          `json:"path"`
	Review  json.RawMessage `json:"review,omitempty"`
}

// failedRow is one row of --failed. Ref is omitted when the run has none;
// Path is set for every exit-1 row, since every rejected input is now kept,
// truncated to its first 1 MiB when it was larger (docs/config.md §4.4.1).
// Both are plain strings so a null never appears in either case, and Path
// stays omittable only in defense of a future exit code that records a row
// with no file to point at.
type failedRow struct {
	At       string     `json:"at"`
	Ref      string     `json:"ref,omitempty"`
	ExitCode int        `json:"exit_code"`
	Counts   countsJSON `json:"counts"`
	Path     string     `json:"path,omitempty"`
	Review   *string    `json:"review,omitempty"`
}

// repoCountRow is one repository's line in --list.
type repoCountRow struct {
	Name    string `json:"name"`
	Reviews int    `json:"reviews"`
	Failed  int    `json:"failed"`
}

type reviewsEnvelope struct {
	Repo       repoInfo    `json:"repo"`
	Total      int         `json:"total"`
	Unreadable *int        `json:"unreadable,omitempty"`
	Reviews    []reviewRow `json:"reviews"`
}

type failedEnvelope struct {
	Repo       repoInfo    `json:"repo"`
	Total      int         `json:"total"`
	Unreadable *int        `json:"unreadable,omitempty"`
	Failed     []failedRow `json:"failed"`
}

type listEnvelope struct {
	Repos []repoCountRow `json:"repos"`
}

// reviews answers docs/config.md §6: every form reads store.db and writes
// nothing, including on a machine that has no store yet.
func (a *App) reviews(ctx context.Context, args []string) int {
	set := a.flagSet("reviews", reviewsUsage)
	repoFlag := set.String("repo", "", "which repository's reviews")
	refFlag := set.String("ref", "", "which commit; the full 40-char SHA")
	limit := set.Int("limit", 10, "most recent N; 0 for all")
	content := set.Bool("content", false, "include each stored file, not just its row")
	failed := set.Bool("failed", false, "list runs that stored no review")
	list := set.Bool("list", false, "print the repositories the store knows")
	format := set.String("format", "json", "output format: json")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	if err := a.checkFormat(*format); err != nil {
		a.fail(err)
		return ExitUsage
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("reviews takes no arguments; did you mean --repo=%s?", set.Arg(0)))
		return ExitUsage
	}
	if *list {
		return a.reviewsCheckedList(ctx, set)
	}
	repo, code := a.resolveRepo(ctx, *repoFlag, isSet(set, "repo"))
	if code != ExitValid {
		return code
	}
	ref := ""
	if isSet(set, "ref") {
		if err := store.ValidateRef(*refFlag); err != nil {
			a.fail(fmt.Errorf("--ref: %w", err))
			return ExitUsage
		}
		ref = *refFlag
	}
	known, err := a.reviewStore.Known(ctx, repo)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	if *failed {
		return a.reviewsFailed(ctx, repo, ref, *limit, known, *content)
	}
	return a.reviewsDefault(ctx, repo, ref, *limit, known, *content)
}

// reviewsCheckedList rejects --list combined with any other reviews flag
// before doing anything else (docs/config.md §6: "flags do not silently
// compose"), then prints the repository index.
func (a *App) reviewsCheckedList(ctx context.Context, set *flag.FlagSet) int {
	for _, name := range reviewsExclusiveFlags {
		if isSet(set, name) {
			a.fail(fmt.Errorf("--list prints the repositories the store knows and takes no other flag; --%s was also given", name))
			return ExitUsage
		}
	}
	return a.reviewsList(ctx)
}

// resolveRepo turns --repo into a repository name: a given value is
// validated as written and never normalized (docs/config.md §4.8); an
// absent one is inferred from the working directory, and outside a
// repository with nothing given, reviews has no default to offer.
func (a *App) resolveRepo(ctx context.Context, value string, given bool) (string, int) {
	if given {
		if err := store.ValidateName(value); err != nil {
			a.fail(fmt.Errorf("--repo: %w", err))
			return "", ExitUsage
		}
		return value, ExitValid
	}
	name, ok, err := a.reviewStore.RepoName(ctx, a.dir)
	if err != nil {
		a.fail(err)
		return "", ExitTool
	}
	if !ok {
		a.fail(errors.New("not inside a repository; pass --repo=NAME, or loam-refinery reviews --list to see what the store knows"))
		return "", ExitUsage
	}
	return name, ExitValid
}

// reviewsDefault prints the index of passing reviews (docs/config.md §6.1).
func (a *App) reviewsDefault(ctx context.Context, repo, ref string, limit int, known bool, content bool) int {
	rows, total, err := a.reviewStore.ListReviews(ctx, repo, ref, limit)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	out := reviewsEnvelope{Repo: repoInfo{Name: repo, Known: known}, Total: total, Reviews: []reviewRow{}}
	unreadable := 0
	opened := false
	for _, r := range rows {
		row := reviewRow{
			At:      r.At.UTC().Format(time.RFC3339),
			Ref:     r.Ref,
			Digest:  r.Digest,
			Verdict: r.Verdict,
			Counts:  countsJSON(r.Counts),
			Path:    r.Path,
		}
		if content {
			opened = true
			// A read failure and a file that is present but empty or not
			// valid JSON are both unreadable (docs/config.md §6.3): a
			// stored review is never zero bytes, and embedding invalid
			// JSON verbatim would fail the whole encode below, taking
			// every other row down with it.
			data, err := a.reviewStore.ReadContent(r.Path)
			if err != nil || !json.Valid(data) {
				unreadable++
			} else {
				row.Review = json.RawMessage(data)
			}
		}
		out.Reviews = append(out.Reviews, row)
	}
	if opened {
		out.Unreadable = &unreadable
	}
	return a.writeReviews(out)
}

// reviewsFailed prints the index of runs that stored no review
// (docs/config.md §6.1, --failed).
func (a *App) reviewsFailed(ctx context.Context, repo, ref string, limit int, known bool, content bool) int {
	rows, total, err := a.reviewStore.ListFailedRuns(ctx, repo, ref, limit)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	out := failedEnvelope{Repo: repoInfo{Name: repo, Known: known}, Total: total, Failed: []failedRow{}}
	unreadable := 0
	opened := false
	for _, r := range rows {
		row := failedRow{
			At:       r.At.UTC().Format(time.RFC3339),
			Ref:      r.Ref,
			ExitCode: r.ExitCode,
			Counts:   countsJSON(r.Counts),
			Path:     r.Path,
		}
		// Every exit-1 row has a path today (docs/config.md §4.4.1); the
		// guard stays for a future exit code that might record a row with
		// no file to read — that case is not unreadable, it simply has
		// nothing to add.
		if content && r.Path != "" {
			opened = true
			data, err := a.reviewStore.ReadContent(r.Path)
			if err != nil {
				unreadable++
			} else {
				// A stored string, unlike the embedded document above,
				// never fails to marshal, so an empty read is kept as an
				// empty string rather than folded into unreadable — its
				// presence (as opposed to the omitted field on a row with
				// no path at all) is the signal that it was read.
				text := string(data)
				row.Review = &text
			}
		}
		out.Failed = append(out.Failed, row)
	}
	if opened {
		out.Unreadable = &unreadable
	}
	return a.writeReviews(out)
}

// reviewsList prints the repository index (docs/config.md §6.1, --list).
func (a *App) reviewsList(ctx context.Context) int {
	repos, err := a.reviewStore.ListRepos(ctx)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	out := listEnvelope{Repos: []repoCountRow{}}
	for _, r := range repos {
		out.Repos = append(out.Repos, repoCountRow{Name: r.Name, Reviews: r.Reviews, Failed: r.Failed})
	}
	return a.writeReviews(out)
}

// writeReviews writes one of the reviews envelope shapes to stdout.
func (a *App) writeReviews(payload any) int {
	if err := writeReviewsJSON(a.stdout, payload); err != nil {
		a.fail(err)
		return ExitUsage
	}
	return ExitValid
}

// writeReviewsJSON matches internal/render's JSON conventions: two-space
// indent, and HTML escaping off so a path or a digest is never rewritten.
func writeReviewsJSON(w io.Writer, payload any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("encoding json output: %w", err)
	}
	return nil
}
