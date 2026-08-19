title: Anchors as checkable claims
aliases: anchor-claims, checking
related: anchors, file, line, ref, anchor-file-missing
---
An anchor says: this path, at this commit, has this line. That is a factual
claim, and the one an agent gets wrong most confidently: a hallucinated line
number is well-formed, plausibly numbered, and wrong.

So the document carries a ref: a full 40-character commit SHA, never a branch.
A branch names a moving target — an anchor recorded against main means whatever
main points at when someone looks, stale without ever changing.

validate checks those claims against the repository it is run in, by default and
with no flag, by object lookup — a commit that is not checked out still
resolves. With no repository the tier is skipped, and reported as skipped.

refinery confirms the anchor points somewhere; whether it points at the right
line is the reviewer's job. What the SHA buys is that anyone can check:

  git show 4f2c1a9:internal/fetch/client.go | sed -n '88,94p'
