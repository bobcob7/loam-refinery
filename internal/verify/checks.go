package verify

import "github.com/bobcob7/loam-refinery/internal/review"

// Checks returns the verification check registry: anchor claims checked against
// the repository. They run whenever one is found, are reported as skipped when
// none is, and cannot be disabled or demoted — every verification failure is
// unconditional (docs/features/combined-reviews.md §3.3).
func Checks() []review.Check {
	return []review.Check{
		{
			Name:    "verification-required",
			Tier:    review.TierVerification,
			Summary: "the anchors could not be confirmed against a repository",
			Title:   "Verification required but unavailable",
			Body: `Fires whenever an anchor claim went unchecked, for any of three reasons: no
repository, one that could not be asked, or a file git could not read. It runs
unconditionally now — there is no flag to accept the gap, because nothing
enters the store unverified. A document is not wrong for being checked
somewhere that could not check it, but the run still fails: a review nobody
verified is worth no more than no review.

The verification block carries the cause. "none" means the run was outside a
repository: cd to the one the review names. "unavailable" means a repository
existed but could not answer — git missing, a bare repository, a checkout git
refuses on ownership grounds — and the reason is git's own words. A file git
could not read is rarer still.

The fix in every case is a repository that can actually answer — not a
command-line option. A document with no anchors is never affected; there is
nothing here for it to ask about.`,
			Related: []string{"ref-unknown", "anchor-file-missing", "tiers"},
		},
		{
			Name:    "ref-unknown",
			Tier:    review.TierVerification,
			Summary: "the ref does not resolve in the repository loam-refinery was run in",
			Title:   "Unresolvable ref",
			Body: `Fires when the document ref names no commit in the repository the tool was run
in. Every anchor is read at that ref, so the whole review becomes a set of claims
nobody can check — which is why it is reported once for the document rather than
once per anchor.

There are two ordinary causes. Either you are in the wrong repository, in which
case the SHA in the diagnostic will look foreign and the fix is to cd; or the
commit is genuinely absent — a shallow clone that never fetched it, or a branch
deleted and garbage collected since the review was written. That second case
has no flag to work around anymore: the fix is a deeper fetch, git fetch
--deepen, or an unshallow clone, before resubmitting.

If the ref is wrong rather than absent, take it from the checkout the review was
performed against: git rev-parse HEAD. Fetching the commit is the other fix;
loam-refinery never touches the network itself.`,
			Related: []string{"ref", "ref-format", "anchor-file-missing"},
		},
		{
			Name:    "anchor-file-missing",
			Tier:    review.TierVerification,
			Summary: "the anchored path does not exist at the document ref",
			Title:   "Anchored file missing",
			Body: `Fires when an anchor's path does not exist at the document ref, or names a
directory rather than a file. The diagnostic names the ref it looked
in, so the claim can be checked by hand:

  git ls-tree -r --name-only 4f2c1a9 | grep client

The usual causes are a path invented from memory, a path relative to a
subdirectory rather than the repository root, and a file that exists in the
working tree but not at the reviewed commit. Anchors are repository-relative
from the root, always — internal/fetch/client.go, never fetch/client.go because
that is where you happened to be standing.

An anchor pointing at nothing makes its comment unactionable, which is why this
is an error rather than an advisory. A comment with no location at all is
legitimate: file it with an empty anchors list.`,
			Related: []string{"file", "anchors", "ref-unknown"},
		},
		{
			Name:    "anchor-line-out-of-range",
			Tier:    review.TierVerification,
			Summary: "line or end_line exceeds the file's line count at the document ref",
			Title:   "Anchored line out of range",
			Body: `Fires when an anchor's line or end_line is past the end of the file at the
document ref. The diagnostic names the file's real length and the ref, because
"line 88 is out of range" invites an argument and "line 88 is out of range in a
61-line file at 4f2c1a9" ends it.

This is the check that catches the failure this format exists for: a
hallucinated line number is well-formed, plausibly numbered, and wrong, and no
amount of care elsewhere in the review makes it inspectable. Confirm the span
before filing it:

  git show 4f2c1a9:internal/fetch/client.go | sed -n '88,94p'

loam-refinery only confirms the line exists. Whether it is the right line is your
job — that is why a file-level anchor, with no line at all, is a legitimate
answer when you are not certain.`,
			Related: []string{"line", "end_line", "anchors"},
		},
		{
			Name:    "anchor-worktree-diverged",
			Tier:    review.TierVerification,
			Summary: "the anchored file's working-tree copy exists and differs from ref",
			Title:   "Anchored file diverged from ref",
			Body: `Fires when ref is the checked-out commit, an anchored file exists there, a
working-tree copy exists too, and git says the two differ — the ordinary
state of a checkout somebody is actively editing. It is read off
verification's own pass as a precondition: once found, that pass's other
results are discarded and the run stops before structural checks and
advisories. One diagnostic covers the whole document, however many anchors
diverged — exit 3, not exit 1.

The reviewed state is not a commit, and revising the document cannot fix
that. Commit what was reviewed — even to a throwaway branch — so ref and the
working tree agree again, or run git stash create: it builds a real commit
object out of the working tree and the index without touching either, giving
a resolvable SHA to submit against instead.

This is not a check to retry. Resubmitting the identical document against
the identical dirty tree cannot succeed — an agent that treats exit 3 like
exit 1 and just resubmits will loop until something outside the review
changes.`,
			Related: []string{"anchor-file-missing", "anchor-line-out-of-range", "verification-required"},
		},
	}
}
