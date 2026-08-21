package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/bobcob7/loam-refinery/internal/render"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/store"
)

const collectReviewsUsage = `usage: loam-refinery collect-reviews --ref=SHA [--repo=NAME] [--format=json|markdown]
`

// collectReviews answers docs/features/combined-reviews.md §2: every stored
// review for one ref, combined into one JSON envelope (§8.1). --ref is
// required, unlike on reviews, where it defaults to "all refs" — this
// command answers a question that has no sensible meaning across more than
// one commit at a time.
func (a *App) collectReviews(ctx context.Context, args []string) int {
	set := a.flagSet("collect-reviews", collectReviewsUsage)
	refFlag := set.String("ref", "", "which commit; the full 40-char SHA (required)")
	repoFlag := set.String("repo", "", "which repository's reviews")
	formatFlag := set.String("format", "json", "json or markdown")
	if err := set.Parse(args); err != nil {
		return usageOrHelp(err)
	}
	if set.NArg() > 0 {
		a.fail(fmt.Errorf("collect-reviews takes no arguments; did you mean --ref=%s?", set.Arg(0)))
		return ExitUsage
	}
	if !isSet(set, "ref") {
		a.fail(errors.New("--ref is required: which commit to collect reviews for"))
		return ExitUsage
	}
	if err := store.ValidateRef(*refFlag); err != nil {
		a.fail(fmt.Errorf("--ref: %w", err))
		return ExitUsage
	}
	if err := checkCollectReviewsFormat(*formatFlag); err != nil {
		a.fail(err)
		return ExitUsage
	}
	ref := *refFlag
	repo, code := a.resolveRepo(ctx, *repoFlag, isSet(set, "repo"))
	if code != ExitValid {
		return code
	}
	return a.collectReviewsRun(ctx, repo, ref, *formatFlag)
}

// checkCollectReviewsFormat is collect-reviews's own --format validator
// (docs/features/combined-reviews.md §2): json or markdown, nothing else.
func checkCollectReviewsFormat(value string) error {
	switch value {
	case "json", "markdown":
		return nil
	default:
		return fmt.Errorf("--format: must be json or markdown, got %q", value)
	}
}

// collectReviewsRun does the actual work once every flag has validated:
// resolve what the store knows, assemble it (internal/collect), recheck
// divergence (§4.3), and render the envelope (§8.1). known, storeEnabled,
// and the assembled result are independent facts and are gathered in that
// order regardless of one another — §9's table never gates one on another
// except a tool error, which stops the run immediately wherever it occurs.
func (a *App) collectReviewsRun(ctx context.Context, repo, ref, format string) int {
	known, err := a.reviewStore.Known(ctx, repo)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	storeEnabled, err := a.reviewStore.StoreEnabled(ctx)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	digests, err := a.reviewStore.DistinctDigests(ctx, repo, ref)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	result, err := collect.Assemble(ctx, toCollectDigests(digests), &collectReader{store: a.reviewStore, repo: repo, ref: ref})
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	source, isHead, diverged, err := a.collectHeadCheck(ctx, ref, result.Submissions)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	envelope := render.CollectReviewsEnvelope{
		Ref:          ref,
		RepoName:     repo,
		RepoKnown:    known,
		StoreEnabled: storeEnabled,
		HeadCheck: render.CollectReviewsHeadCheck{
			Source:   source,
			IsHead:   headIsHeadPointer(source, isHead),
			Diverged: convertDiverged(diverged),
		},
		Result: result,
	}
	if err := a.renderCollectReviews(format, envelope); err != nil {
		a.fail(err)
		return ExitUsage
	}
	return ExitValid
}

// renderCollectReviews picks the formatter format names (§2, §8.3): json
// goes through a.renderer, the one interface every other command's output
// also goes through; markdown goes through its own, separate formatter
// (§8.3.1's "internal/render gains a second formatter beside the JSON
// one") constructed here rather than threaded through the renderer
// interface, since collect-reviews --format markdown is the only command
// that ever needs it — the interface's own doc comment is explicit that
// two renderers behind one interface is exactly the shape it exists to
// rule out everywhere else. Both formatters take the identical envelope;
// neither computes anything the other does not already have.
func (a *App) renderCollectReviews(format string, envelope render.CollectReviewsEnvelope) error {
	if format == "markdown" {
		return render.NewMarkdown().CollectReviews(a.stdout, envelope)
	}
	return a.renderer.CollectReviews(a.stdout, envelope)
}

// toCollectDigests adapts internal/store's DigestRow shape to
// internal/collect's own, field for field — internal/collect deliberately
// does not import internal/store (its own acceptance criterion), so the
// CLI-wiring layer is where the two meet.
func toCollectDigests(rows []store.DigestRow) []collect.DigestRow {
	out := make([]collect.DigestRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, collect.DigestRow{Digest: r.Digest, At: r.At})
	}
	return out
}

// collectReader adapts reviewStore to internal/collect's own reader
// interface (internal/collect/interfaces.go): ReviewPath resolves a digest
// to the path it is stored at, then ReadContent reads it — composing two
// reviewStore methods rather than collect-reviews (or internal/collect)
// importing internal/store directly, per that package's own doc comment on
// why it takes a reader instead.
type collectReader struct {
	store     reviewStore
	repo, ref string
}

// ReadReview implements internal/collect's reader. Any error here — from
// resolving the path or from reading it — is reported by Assemble as one
// unreadable digest, skipped and counted rather than failing the whole run
// (docs/features/combined-reviews.md §9): a missing or corrupted stored
// file is exactly config.md §6.3's contract, and a config that could not be
// read at all fails the same way reviews --content's own ReadContent
// failures already do — counted, never silently dropped.
func (r *collectReader) ReadReview(ctx context.Context, digest string) ([]byte, error) {
	path, err := r.store.ReviewPath(ctx, r.repo, r.ref, digest)
	if err != nil {
		return nil, err
	}
	return r.store.ReadContent(path)
}

// collectHeadCheck answers §4.3's head_check question for the whole
// combined result: has an anchor already confirmed at submission time
// drifted from ref since. headChecker only ever answers for one parsed
// document at a time, so this loops over every surviving submission's own
// Document and merges what comes back — source and is_head are read from
// the first call (every submission shares the one ref this command was
// asked about, so every call must agree), and diverged is the
// concatenation of every call's own entries, staying nil until the first
// non-nil one arrives so "the check does not apply" survives having zero
// submissions to check against.
//
// With no submissions at all, there is nothing to loop over, but
// head_check is still always present (§4.3.1) — a synthetic document
// carrying only ref, no comments, is enough for headChecker to answer
// source and is_head from, and AnchorCount()==0 means the recheck itself
// has nothing to withhold, so diverged still comes back correctly (empty
// when is_head is true, absent otherwise).
func (a *App) collectHeadCheck(ctx context.Context, ref string, submissions []collect.Submission) (string, bool, []DivergedAnchor, error) {
	if len(submissions) == 0 {
		doc := &review.Document{Ref: review.Field[string]{Value: ref, Present: true, OK: true}}
		return a.headChecker.CheckDivergence(ctx, a.dir, doc, nil)
	}
	var source string
	var isHead bool
	var diverged []DivergedAnchor
	for i, sub := range submissions {
		s, head, d, err := a.headChecker.CheckDivergence(ctx, a.dir, sub.Document, sub.QualifiedIDs)
		if err != nil {
			return "", false, nil, err
		}
		if i == 0 {
			source, isHead = s, head
		}
		if d != nil {
			if diverged == nil {
				diverged = []DivergedAnchor{}
			}
			diverged = append(diverged, d...)
		}
	}
	return source, isHead, diverged, nil
}

// headIsHeadPointer reports is_head only when source == "repo" (§4.3.1: "a
// caller cannot tell whether --ref is HEAD without a repository to ask, so
// the field is absent rather than guessing false").
func headIsHeadPointer(source string, isHead bool) *bool {
	if source != "repo" {
		return nil
	}
	v := isHead
	return &v
}

// convertDiverged carries cli.DivergedAnchor through to render's own,
// package-local shape unchanged, field for field, preserving nil (§4.3.1's
// "absent" case) versus non-nil-but-empty (its "checked, found nothing"
// case) exactly as headChecker reported it.
func convertDiverged(in []DivergedAnchor) []render.CollectReviewsDiverged {
	if in == nil {
		return nil
	}
	out := make([]render.CollectReviewsDiverged, 0, len(in))
	for _, d := range in {
		out = append(out, render.CollectReviewsDiverged{Name: d.Name, Comment: d.Comment, File: d.File, Message: d.Message})
	}
	return out
}
