package verify

import "github.com/bobcob7/loam-refinery/internal/review"

// Checks returns the verification check registry: anchor claims checked against
// the repository. They run whenever one is found, are reported as skipped when
// none is, and can be demoted with --warn-only but never disabled.
func Checks() []review.Check {
	return []review.Check{
		{
			Name:    "verification-required",
			Tier:    review.TierVerification,
			Summary: "the anchors were not checked, and --require-verification asked that they be",
			Title:   "Verification required but unavailable",
			Body: `Fires only under --require-verification, and says one thing: nobody confirmed
these anchors. A repository that did not answer, a commit this checkout does
not have, a file git could not read — different causes, same answer.

Without the flag that is reported and the run passes, because a document is not
wrong for being checked somewhere that could not check it. The flag is for the
caller who cannot accept that: a merge gate whose whole purpose is confirming
the line numbers are real, where a review nobody verified is worth no more than
no review.

The verification block carries the cause: "not a git repository" means cd, and
an unavailable source means the machine rather than the review. --warn-only on the
check that explains the gap excuses it — ref-unknown for a commit this checkout
never had. Nothing excuses a file git could not read.`,
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
commit is genuinely absent — a shallow clone, or a branch deleted and garbage
collected since the review was written. That second case is what --warn-only is
for: it demotes the verification checks to advisories so a legitimate gap does
not fail the run.

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
	}
}
