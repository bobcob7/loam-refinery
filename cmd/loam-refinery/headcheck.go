package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bobcob7/loam-refinery/internal/cli"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/verify"
)

// headCheckAdapter is the concrete implementation of internal/cli's
// headChecker interface (docs/features/combined-reviews.md §4.3.2, routes
// 2+3). It discovers the repository and constructs a Verifier once per
// Discover call — once per collect-reviews invocation, not once per
// submission (refinery-k3h) — and hands both back in the returned
// resolvedHeadCheck for every submission's own Diverged call to reuse.
type headCheckAdapter struct {
	log *slog.Logger
}

// newHeadCheckAdapter returns a headChecker.
func newHeadCheckAdapter(log *slog.Logger) *headCheckAdapter {
	return &headCheckAdapter{log: log}
}

// Discover implements internal/cli's headChecker. source mirrors
// reviewsAdapter.RepoName's own discovery outcomes onto verification.source's
// three values: "none" outside a repository, "unavailable" when one exists
// but discovery itself failed, "repo" otherwise. isHead is asked through
// verify.Verifier.RefIsHEAD rather than re-run with a second git call per
// submission, so this and Verify's own internal isHEAD decision can never
// drift apart. Both are resolved here, once, and carried on the returned
// resolvedHeadCheck so no later Diverged call repeats repository discovery
// or the HEAD check.
func (a *headCheckAdapter) Discover(ctx context.Context, dir, ref string) (cli.HeadCheck, error) {
	repository, err := verify.Discover(ctx, dir)
	if errors.Is(err, verify.ErrNoRepository) {
		return &resolvedHeadCheck{source: "none"}, nil
	}
	if err != nil {
		a.log.Debug("head check unavailable", "error", err)
		return &resolvedHeadCheck{source: "unavailable"}, nil
	}
	verifier := verify.New(repository, a.log)
	if !verifier.RefIsHEAD(ctx, ref) {
		return &resolvedHeadCheck{source: "repo"}, nil
	}
	return &resolvedHeadCheck{source: "repo", isHead: true, verifier: verifier}, nil
}

// resolvedHeadCheck is one collect-reviews invocation's resolved
// repository/HEAD state (internal/cli.HeadCheck): Source and IsHead answer
// from fields Discover already fixed; Diverged is the only method that does
// per-submission work, reusing the one Verifier Discover built rather than
// constructing its own.
type resolvedHeadCheck struct {
	source   string
	isHead   bool
	verifier *verify.Verifier
}

// Source implements internal/cli.HeadCheck.
func (h *resolvedHeadCheck) Source() string { return h.source }

// IsHead implements internal/cli.HeadCheck.
func (h *resolvedHeadCheck) IsHead() bool { return h.isHead }

// Diverged implements internal/cli.HeadCheck. It is left nil whenever
// isHead is false, so a caller can tell "does not apply" from "checked,
// found nothing" without inspecting Source() at all.
func (h *resolvedHeadCheck) Diverged(ctx context.Context, doc *review.Document, qualifiedIDs map[string]string) ([]cli.DivergedAnchor, error) {
	if !h.isHead {
		return nil, nil
	}
	_, _, verification := h.verifier.Verify(ctx, doc)
	return translateDiverged(doc, verification.Unverified, qualifiedIDs), nil
}

// translateDiverged turns verify.Verifier's per-anchor Unverified list into
// head_check.diverged entries (combined-reviews.md §4.3.1). It is never nil:
// a non-nil, possibly empty, slice is how CheckDivergence tells "checked,
// nothing diverged" apart from "the check does not apply here" (nil).
func translateDiverged(doc *review.Document, unverified []review.Unverified, qualifiedIDs map[string]string) []cli.DivergedAnchor {
	diverged := make([]cli.DivergedAnchor, 0, len(unverified))
	for _, u := range unverified {
		diverged = append(diverged, cli.DivergedAnchor{
			Name:    u.Name,
			Comment: qualifiedIDs[u.Comment],
			File:    anchorFile(doc, u.Path),
			Message: u.Message,
		})
	}
	return diverged
}

// anchorFile resolves a JSON Pointer of the shape review.Anchor.Path always
// produces - "/comments/<i>/anchors/<j>" - against doc, returning the
// anchor's own file field: the translation §4.3.1 requires from a pointer
// into the one document that was checked to the anchored path it names
// directly, since there is no single document in the combined output for a
// pointer to point into. verify.Verifier always sets an Unverified entry's
// Path to exactly this shape, so a pointer this cannot resolve against doc
// means doc is not the document the Unverified list came from, and "" is
// reported rather than a guess.
func anchorFile(doc *review.Document, pointer string) string {
	var commentIndex, anchorIndex int
	if _, err := fmt.Sscanf(pointer, "/comments/%d/anchors/%d", &commentIndex, &anchorIndex); err != nil {
		return ""
	}
	if pointer != fmt.Sprintf("/comments/%d/anchors/%d", commentIndex, anchorIndex) {
		return ""
	}
	if commentIndex < 0 || commentIndex >= len(doc.Comments) {
		return ""
	}
	anchors := doc.Comments[commentIndex].Anchors
	if anchorIndex < 0 || anchorIndex >= len(anchors) {
		return ""
	}
	return anchors[anchorIndex].File.Value
}
