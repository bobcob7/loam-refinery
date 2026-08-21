package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/bobcob7/loam-refinery/internal/render"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/store"
)

// errDigestMismatch marks a stored review whose bytes do not hash to the
// digest they are filed under (refinery-qs2). Content addressing
// (docs/config.md §4.4) makes this detectable in the first place; checking
// for it is what makes it detected. It is stronger than re-running
// submit-review's own structural and schema checks against the bytes read
// back — a mismatch means these are not the bytes that passed those checks,
// which no amount of re-validating the bytes actually present can fix, so
// this package checks identity instead of re-deriving validity.
var errDigestMismatch = errors.New("stored content does not match its digest")

const collectReviewsUsage = `usage: loam-refinery collect-reviews --ref=SHA [--repo=NAME] [--format=json|markdown|html]
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
	formatFlag := set.String("format", "json", "json, markdown, or html")
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
// (docs/features/combined-reviews.md §2, docs/features/html-report.md
// §2.3): json, markdown, or html, nothing else.
func checkCollectReviewsFormat(value string) error {
	switch value {
	case "json", "markdown", "html":
		return nil
	default:
		return fmt.Errorf("--format: must be json, markdown, or html, got %q", value)
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
	reader := &collectReader{store: a.reviewStore, repo: repo, ref: ref, log: a.log}
	result, err := collect.Assemble(ctx, toCollectDigests(digests), reader)
	if err != nil {
		a.fail(err)
		return ExitTool
	}
	if reader.pathErr != nil {
		a.fail(fmt.Errorf("resolving stored review path: %w", reader.pathErr))
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

// renderCollectReviews picks the formatter format names (§2, §8.3 of
// docs/features/combined-reviews.md; §2.3 of docs/features/html-report.md):
// json goes through a.renderer, the one interface every other command's
// output also goes through; markdown and html each go through their own,
// separate formatter (§8.3.1's "internal/render gains a second formatter
// beside the JSON one", extended a third time by html-report.md §2.3)
// constructed here rather than threaded through the renderer interface,
// since collect-reviews is the only command that ever needs either — the
// interface's own doc comment is explicit that two renderers behind one
// interface is exactly the shape it exists to rule out everywhere else,
// and adding an HTML method (or a format parameter) would reopen that
// shape for a command that has already demonstrated it does not need to.
// All three formatters take the identical envelope; none computes
// anything the others do not already have.
func (a *App) renderCollectReviews(format string, envelope render.CollectReviewsEnvelope) error {
	switch format {
	case "markdown":
		return render.NewMarkdown().CollectReviews(a.stdout, envelope)
	case "html":
		return render.NewHTML().CollectReviews(a.stdout, envelope)
	default:
		return a.renderer.CollectReviews(a.stdout, envelope)
	}
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
	// pathErr is the first error ReviewPath returned, if any. ReviewPath's
	// only failure mode is a config load, the same tool-state failure
	// Known, StoreEnabled, and DistinctDigests already exit ExitTool for
	// elsewhere in this run, not a per-digest content problem — so it must
	// not be folded into Assemble's Unreadable count, which would report a
	// tool error as a lost review with no cause an operator could read.
	// collectReviewsRun checks it once Assemble returns and fails the run
	// with the real cause instead.
	pathErr error
	// log records what Assemble's own error return cannot: a digest
	// mismatch and a missing file both become the identical
	// skip-and-count against Result.Unreadable (refinery-qs2's own
	// instinct — a corrupt store entry is the tool's own state, not
	// something about the review, so it gets the same treatment a review
	// that never made it to the store gets), but an operator watching this
	// run's logs must still be able to tell "file missing" from "file
	// present and not what it claims to be" — one is routine, the other is
	// the store having been written to outside submit-review's checks. nil
	// is valid and simply skips logging, so tests that build a
	// collectReader directly need not supply one.
	log *slog.Logger
}

// ReadReview implements internal/collect's reader. A ReadContent failure —
// a missing file — is exactly what Assemble's reader contract expects:
// reported through the returned error, skipped and counted against
// Result.Unreadable. So is a digest mismatch, checked here against the
// digest Assemble is asking for by name (config.md §4.4: the filename is
// the digest, so verifying is comparing content already in hand against a
// value already in hand, not a second read or a second store call) — but
// logged first, distinctly from a plain read failure, so the two remain
// tellable apart outside the envelope itself (refinery-qs2). A ReviewPath
// failure is not treated as either: it is recorded on pathErr rather than
// left for Assemble to count, since collectReviewsRun treats it as the tool
// failure it is.
func (r *collectReader) ReadReview(ctx context.Context, digest string) ([]byte, error) {
	path, err := r.store.ReviewPath(ctx, r.repo, r.ref, digest)
	if err != nil {
		if r.pathErr == nil {
			r.pathErr = err
		}
		return nil, err
	}
	data, err := r.store.ReadContent(path)
	if err != nil {
		return nil, err
	}
	if actual := store.Digest(data); actual != digest {
		if r.log != nil {
			r.log.Warn("stored review does not match its digest", "repo", r.repo, "ref", r.ref, "path", path, "want_digest", digest, "got_digest", actual)
		}
		return nil, fmt.Errorf("%s: %w", path, errDigestMismatch)
	}
	return data, nil
}

// collectHeadCheck answers §4.3's head_check question for the whole
// combined result: has an anchor already confirmed at submission time
// drifted from ref since. Repository discovery and the HEAD check are
// per-invocation facts, not per-submission ones, so headChecker.Discover is
// called exactly once regardless of how many submissions survived
// (refinery-k3h); only the returned HeadCheck's Diverged call — which of
// one submission's own anchors have drifted — is repeated per submission,
// since that answer genuinely differs from one document to the next.
//
// With no submissions at all, there is nothing to loop over, but
// head_check is still always present (§4.3.1) — a synthetic document
// carrying only ref, no comments, is enough for Diverged to answer from,
// and AnchorCount()==0 means the recheck itself has nothing to withhold, so
// diverged still comes back correctly (empty when is_head is true, absent
// otherwise).
func (a *App) collectHeadCheck(ctx context.Context, ref string, submissions []collect.Submission) (string, bool, []DivergedAnchor, error) {
	head, err := a.headChecker.Discover(ctx, a.dir, ref)
	if err != nil {
		return "", false, nil, err
	}
	source, isHead := head.Source(), head.IsHead()
	if len(submissions) == 0 {
		doc := &review.Document{Ref: review.Field[string]{Value: ref, Present: true, OK: true}}
		diverged, err := head.Diverged(ctx, doc, nil)
		if err != nil {
			return "", false, nil, err
		}
		return source, isHead, diverged, nil
	}
	var diverged []DivergedAnchor
	for _, sub := range submissions {
		d, err := head.Diverged(ctx, sub.Document, sub.QualifiedIDs)
		if err != nil {
			return "", false, nil, err
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
