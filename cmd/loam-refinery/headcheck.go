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
// 2+3). It discovers the repository and constructs a Verifier fresh on every
// call, the same way reviewsAdapter resolves the store fresh on every call,
// rather than holding either across a run.
type headCheckAdapter struct {
	log *slog.Logger
}

// newHeadCheckAdapter returns a headChecker.
func newHeadCheckAdapter(log *slog.Logger) *headCheckAdapter {
	return &headCheckAdapter{log: log}
}

// CheckDivergence implements internal/cli's headChecker. source mirrors
// reviewsAdapter.RepoName's own discovery outcomes onto verification.source's
// three values: "none" outside a repository, "unavailable" when one exists
// but discovery itself failed, "repo" otherwise. isHead is asked through
// verify.Verifier.RefIsHEAD rather than re-run with a second git call here,
// so this and Verify's own internal isHEAD decision can never drift apart.
// diverged is left nil whenever isHead is false, so a caller can tell "does
// not apply" from "checked, found nothing" without inspecting source at all.
func (a *headCheckAdapter) CheckDivergence(ctx context.Context, dir string, doc *review.Document, qualifiedIDs map[string]string) (string, bool, []cli.DivergedAnchor, error) {
	repository, err := verify.Discover(ctx, dir)
	if errors.Is(err, verify.ErrNoRepository) {
		return "none", false, nil, nil
	}
	if err != nil {
		a.log.Debug("head check unavailable", "error", err)
		return "unavailable", false, nil, nil
	}
	verifier := verify.New(repository, a.log)
	if !verifier.RefIsHEAD(ctx, doc.Ref.Value) {
		return "repo", false, nil, nil
	}
	_, _, verification := verifier.Verify(ctx, doc)
	return "repo", true, translateDiverged(doc, verification.Unverified, qualifiedIDs), nil
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
