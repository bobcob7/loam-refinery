# Combined reviews

Feature design. Draft, pending implementation and pending the `cli.md`,
`config.md`, and `review-document.md` edits it calls for but does not make.

Companion to [../cli.md](../cli.md), which specifies the commands this feature
adds two of, [../config.md](../config.md), which specifies the store this
feature reads from and nothing else, and [../review-document.md](../review-document.md),
which specifies the format this feature's output deliberately does not
conform to ([§8](#8-the-output-is-not-a-review-document)).

This document has been revised twice since its first draft. The first
revision responded to three independent reviews that found a misquote, an
inverted store-mechanics claim, a real qualified-id collision, and an
incomplete amendments section. This revision responds to four direct user
decisions that narrow scope in one place (no migration plan), tighten it in
two (no disable flags, verification required to submit), and widen it in one
(markdown as a day-one output format for `collect-reviews`). Some of what
follows deletes work the previous revision did; that is noted where it
happens rather than quietly dropped.

## 1. What this adds

An orchestrator that wants several reviewers' opinions of one commit today
pays N calls to `loam-refinery reviews` — or reads N files off disk itself —
and does the work of stitching them into one picture by hand, every time, in
every orchestrator that wants this. That is exactly the gap `reviews`
already declined to close: it is "deliberately minimal... not a query API"
([../cli.md §2.6](../cli.md#26-reviews)), and combining what it returns was
explicitly left to the caller.

This feature closes it with two changes, not one:

- **`collect-reviews`**, a new command that takes a `ref` and returns every
  stored review for it as one attributed document, in JSON for a machine
  caller and, day one, in Markdown for a human one
  ([§2](#2-the-command), [§8.3](#83-the-markdown-projection)).
- **`submit-review`**, `validate` renamed and, as of this revision,
  meaningfully stricter — not a pure rename
  ([§3](#3-the-rename-to-submit-review)).

The two are one feature because the rename is what the new command's name
has to agree with: a caller that *submits* a review *collects* reviews back,
not the reverse. Building `collect-reviews` without the rename would leave
the pair unnamed as a pair.

This also closes a loop opened by the reviewer-profile feature
([../cli.md §2.1.1](../cli.md#211-reviewer-profiles)): a profile answers *who
is reviewing, and what they are looking hardest at*, and until now the answer
never made it back into anything machine-readable — a profile shaped what a
reviewer wrote, and the document it wrote carried no trace of which profile
shaped it. `collect-reviews` is what reads that trace back out, which is why
it needs a new, optional document field to carry it
([§7](#7-the-profile-field)).

Everything here is design, not implementation. No schema changes, no code,
and — per the acceptance criteria this document exists to satisfy — no edits
to `cli.md`, `config.md`, or `review-document.md`. Where this feature
requires amending one of those three, [§11](#11-amendments) quotes exactly
what it would amend; making that edit is a later bead.

## 2. The command

```
loam-refinery collect-reviews --ref=SHA [--repo=NAME] [--format json|markdown]
```

`--ref` is **required**, unlike on `reviews`, where it defaults to "all
refs." `reviews` answers "what does the store hold"; `collect-reviews`
answers a narrower question — "what did every reviewer conclude about *this*
commit" — and that question has no sensible answer across more than one
commit at a time. A caller that omits `--ref` gets a usage error naming it as
required, not a combined view spanning every ref the store has ever seen.
That boundary matters enough to restate plainly: combining reviews across
profiles for **one** ref is the whole of what this feature does. Combining
across **refs** — what changed between two reviews of the same file, what a
reviewer said last week versus what it says now — is a different feature,
already recorded as deferred
([../config.md §8, "reviews --diff"](../config.md#8-future-considerations)),
and nothing here moves it closer.

`--repo` behaves exactly as it does on `reviews`
([../config.md §6](../config.md#6-reading-the-store)): inferred by walking up
from the working directory when omitted, a usage error when it cannot be
inferred and was not given. It selects which store bucket to query — a
label, per [../config.md §4.2](../config.md#42-repository-identity) — and
that is a different thing from *standing inside a git checkout right now*,
which matters once more below ([§4.3](#43-the-head-check)): passing
`--repo=NAME` explicitly does not manufacture a working tree to check
divergence against.

`--format` defaults to `json`, unchanged from every other command's
posture. `markdown` is the one new accepted value, and it exists on this
command alone — [§8.3](#83-the-markdown-projection) specifies exactly what
it is and is not, and [§11.10](#1110-climd-51-the-markdown-exception)
amends [cli.md §5.1](../cli.md#51-one-format) narrowly to allow it.
`--format=text` remains an error on `collect-reviews` too, the same as
everywhere else — `markdown` is a distinct, named value, not a synonym for
the one `cli.md §5.1` already rejects, so accepting one does not quietly
change the meaning of the other.

No other flag. `--content`, `--limit`, `--failed`, `--list` are all
`reviews`-shaped questions about the store as a log; `collect-reviews`
answers a `submit-review`-shaped question about one commit, and does not
inherit `reviews`'s surface just because it reads the same tables. There is
deliberately no flag to turn the check in [§4.3](#43-the-head-check) off,
for the same reason verification itself has no opt-out on `submit-review`
([§3.4](#34-verification-is-required-to-submit)): a check that a caller can
silently skip is a check that will, on the run that most needed it, be
skipped.

### 2.1 Rejected names

**`merge`** — [../cli.md §8](../cli.md#8-future-considerations)'s own
sketch. Rejected because it collides with git's vocabulary in a tool that
sits beside git commands on every invocation: `loam-refinery merge` reads,
at a glance, like it does something to the checkout, and this command never
writes anywhere near one.

**`combine-reviews`** — closer, and the word this document's own filename
uses. Rejected because it names the *algorithm* rather than the *act*, and
every other command in this ladder is named for what the caller is doing —
`prime`, `describe`, `submit-review`, `schema`, `reviews` — not for what
happens inside the tool. `merge` has the same problem one layer down.

**`--combined` on `reviews`** — considered as a flag rather than a new verb,
and rejected because it would silently widen `reviews`'s own contract.
`reviews` answers exactly two shapes today, index and content, both **rows**
— "what was stored, not what it said"
([../config.md §6.1](../config.md#61-output)). A combined view is a third
shape, an attributed cross-submission document, not a third way to render a
row. Bolting it onto `reviews` would make that command's own specification
say less than it does.

**`aggregate-reviews`** — rejected on the word alone. `aggregate` is the
exact term [§11](#11-amendments) amends out of two out-of-scope bullets;
naming the command with the word being carefully un-said would undo the
carefulness in the first breath.

**`collect-reviews`** — chosen. It reads naturally after `submit-review` in
the same way `reviews` reads naturally after `submit-review`'s predecessor,
without claiming the more contested "combine" or "merge," and it says
plainly that the command's job is gathering something that already exists,
not deciding anything about it.

### 2.2 Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--ref=SHA` | none — required | Which commit. The full 40-character SHA; a usage error otherwise, exactly as on `reviews`. |
| `--repo=NAME` | inferred from the CWD's repository | Which repository's reviews. |
| `--format json\|markdown` | json | Output format. `markdown` is unique to this command — see [§8.3](#83-the-markdown-projection). |

### 2.3 Where it sits in the ladder

Neither a rung nor `reviews`. [../cli.md §2](../cli.md#2-commands) already
says `reviews` "is not a rung on that ladder... a caller that never stores
anything never needs it." `collect-reviews` inherits exactly that framing —
it reads the store `reviews` reads, so it belongs beside `reviews` in the
command list, in the same paragraph that carves both out of the disclosure
ladder `prime` / `describe` / `describe --lens` / `schema` climbs.

## 3. The rename to submit-review

`validate` becomes `submit-review`, and — this revision's most significant
change from the first draft — it stops being *only* a rename. Three
separate decisions land on the same command at once: no migration path
([§3.2](#32-no-alias-no-migration-plan)), no way to soften the checks that
already exist ([§3.3](#33-no-disable-no-warn-only)), and a materially
stricter default ([§3.4](#34-verification-is-required-to-submit)). The
operation stopped being "does this document hold up" the moment a second
command started reading what it stored, and the new name is meant to carry
all of that, not just the new spelling.

### 3.1 What changes (the name)

The rename touches every place `validate` appears as a **command name** —
the proper noun, usually backticked or following `loam-refinery` — and
leaves untouched every place "validate" appears as an ordinary English verb
about the act of checking a document. Concretely, and by way of the
representative set rather than an exhaustive one — the full audit belongs to
the edit bead, not this one:

- [../cli.md §2](../cli.md#2-commands)'s usage synopsis renames, and shrinks
  at the same time as [§3.3](#33-no-disable-no-warn-only) and
  [§3.4](#34-verification-is-required-to-submit) remove three of its four
  flags — see [§11.8](#118-climd-2-23-and-the-flag-table).
- The `### 2.3` heading itself, `` `validate` ``, becomes `` `submit-review` ``.
  Unlike the mechanical renames in this list, its **body** needs substantive
  edits, not just a name swap: today it says verification runs "if a source
  is supplied," which described an optional tier; under
  [§3.4](#34-verification-is-required-to-submit) a missing or unreachable
  source is no longer a tolerated gap, it is a failing submission. This is
  the one heading in the synopsis where the edit bead is rewriting
  paragraphs, not substituting a word.
- [../cli.md §3](../cli.md#3-flags)'s flag table loses three rows outright —
  `--warn-only`, `--require-verification`, `--disable` do not survive as
  `submit-review` flags under any name, because
  [§3.3](#33-no-disable-no-warn-only) and
  [§3.4](#34-verification-is-required-to-submit) remove what they did. The
  shared `--format json ... describe validate reviews` row renames its
  `validate` to `submit-review` like everything else. `--strict`'s row is
  the one genuinely open item here — [§13](#13-open-questions) — so its fate
  is not decided by this document.
- [../cli.md §4](../cli.md#4-exit-codes)'s narrative prose does not name
  `validate` at all — it says "the review," "the invocation," "the tool" —
  so nothing there moves on the name. Its content is affected by
  [§3.4](#34-verification-is-required-to-submit): "unverified anchors under
  `--require-verification`" in the exit-1 row's description names a flag
  that no longer exists, and needs rewording to describe the new
  unconditional behavior.
- [../cli.md §6.1](../cli.md#61-budgets)'s budget table rows, `` `validate`,
  clean ``, `` `validate`, clean, no repository ``, and the two beside them,
  rename to `` `submit-review`, clean `` and so on. Whether the *numbers*
  survive is a separate question this document does not answer: the "clean,
  no repository" row specifically describes a case
  [§3.4](#34-verification-is-required-to-submit) may have just made
  unreachable for any document with anchors — see
  [§3.4.4](#344-the-unreachable-ref-consequence).
- [../config.md](../config.md)'s usage line in
  [§5](../config.md#5-storing-a-review), `loam-refinery validate [path]`,
  becomes `loam-refinery submit-review [path]`. The rest of that document
  names `` `validate` `` as a backticked command name 21 times and renames
  the same way; 10 further occurrences use "validate" as an ordinary
  English verb and do not move — 21 and 10 sum to the file's 31 total
  occurrences.
- `internal/cli/cli.go`'s `usage` const, printed on `--help` and on the
  unknown-command path, names `validate` twice today and renames both.
- `internal/cli/prime.txt`, the budget-pinned text
  ([../cli.md §2.1](../cli.md#21-prime)). **This now moves.** An earlier
  revision of this document argued for leaving it alone, on the strength of
  a permanent alias that made `validate` correct against every binary,
  old and new. [§3.2](#32-no-alias-no-migration-plan) deletes that alias.
  With no alias, `prime.txt` teaching `loam-refinery validate review.json`
  would be teaching a command that, on an upgraded binary, no longer
  exists — so it has to teach `submit-review` instead, and the reasoning
  that previously argued against touching it no longer applies to anything.
  This is a real edit to a golden-tested, budget-pinned file, and it is
  worth measuring rather than asserting it fits:

  Both occurrences of `validate` in the current text —
  `` loam-refinery validate review.json `` and `` fix, validate again `` —
  become `` loam-refinery submit-review review.json `` and
  `` fix, submit-review again ``. `submit-review` is five characters longer
  than `validate` (13 versus 8), twice, for ten additional characters
  total. Measured with this repository's own token approximation
  ([`internal/cli/budget_test.go`](../../internal/cli/budget_test.go),
  `approxTokens`: `len(text) / 4`) against the actual current `prime`
  output — `internal/cli/prime.txt` and the binary's own stdout agree,
  byte for byte, exactly as the byte-identity guarantee requires — 967
  characters, 241 tokens today; 977 characters, 244 tokens after the
  rename. Both are comfortably under the 250-token ceiling
  ([../cli.md §6.1](../cli.md#61-budgets)), with 6 tokens of headroom left
  rather than the 9 there is today. The golden file
  ([../cli.md §6.1](../cli.md#61-budgets), "The golden-file test pins
  `prime`'s own text against the embedded `prime.txt`") is exactly the
  mechanism that will catch it if this estimate is wrong once the actual
  edit is made.

### 3.2 No alias, no migration plan

**Decision.** `validate` does not survive the rename in any form — no
permanent alias, no deprecation window, no transitional stderr note telling
a caller where the command went. A caller still typing `validate` after the
binary upgrades gets exit 2, "unknown command," the same as any other typo.

This reverses the previous revision's own design, which built a permanent
alias specifically to avoid that failure, on the reasoning that a command
name is API the same way a lens name is
([../cli.md §2.2.1](../cli.md#221-lens-names)). That reasoning was not
wrong; it was answering a question this project is not old enough to be
asked yet. There is no installed base of callers depending on `validate`
today whose breakage has to be engineered around, and building permanent
alias machinery for a project at this stage is solving a problem that does
not exist in exchange for a real one — a second name to keep correct forever,
documented nowhere as deprecated, indistinguishable from the primary one to
anyone reading the command switch later. **The cost is accepted outright:**
every existing caller of `validate` breaks the moment this ships, and no
flag, alias, or grace period softens that. It is a one-time cost paid once,
by whoever is running this tool today, and it is smaller now than it will
ever be again.

### 3.3 No disable, no warn-only

**Decision.** `submit-review` drops `--disable` and `--warn-only` entirely.
Advisories always run and are always reported — the ability to
`--disable=body-thin`, say, and have the tool stop looking, is gone.
Verification checks always run and can never be individually demoted — the
ability to `--warn-only=ref-unknown` and have a shallow clone's gap excused
is gone, folded into the unconditional behavior
[§3.4](#34-verification-is-required-to-submit) specifies.

What survives structural checks was already true — they were never
demotable — so this section changes the *advisory* and *verification* tiers
specifically, the two that used to have a caller-facing knob. Advisory
findings remain non-fatal by construction
([../review-document.md §11](../review-document.md#11-validity), "Advisory
checks ask *is this a good review?*... never hard") — removing `--disable`
does not make an advisory an error, it only removes the ability to make the
tool stop mentioning one a caller has decided it does not want to hear
about. That is a real loss of configurability, stated plainly rather than
minimized: a caller that found `suggestion-no-cons` noise for its use case
previously had a way to quiet it, and now does not.

### 3.4 Verification is required to submit

#### 3.4.1 Why: nothing enters the store unverified

This is the reason for the strictness, not a side effect of it, and it is
worth stating that way before tracing what it costs. `ref`
([§4](#4-ref-as-the-join-key)) only became the join key `collect-reviews`
depends on because [§7.2](#72-caller-authored-and-unverifiable) already
established it as a claim the tool cannot independently confirm — the SHA is
real, but whether it names the commit the reviewer actually read is taken
entirely on faith. Verification is the one mechanism that closes any part of
that gap: it cannot confirm `ref` is *honest*, but it can and does confirm
that every anchor the reviewer wrote actually resolves against it. Making
that confirmation optional — as `submit-review`'s predecessor did, running
it by default but tolerating a skip — meant the store could hold reviews
nobody had ever checked, silently mixed in with reviews that had been. A
combined view built on top of a store like that inherits the gap: two
submissions for one ref, one fully verified and one never checked at all,
sit side by side in `collect-reviews`'s output with nothing distinguishing
them beyond a field a caller has to know to look for.

**Decision.** Any anchor `submit-review` cannot confirm fails the
submission, exit 1, unconditionally. This is the load-bearing property
`collect-reviews` needs to be trustworthy at all: everything in the store
was verified at the moment it was stored, full stop, and a caller reading a
combined output never has to ask "was this one actually checked" the way it
would have had to under the previous, optional design.

#### 3.4.2 The check and the policy are not the same thing

`anchor-worktree-diverged` ([../cli.md §2.3.1](../cli.md#231-verifying-anchors))
is unchanged by this decision, and it is worth being exact about why,
because conflating the two would undo a distinction that section just
finished establishing. The **check** is still monotonic: it can only
withhold a verification, never grant one, and firing it never counts an
anchor as *wrong*, only as *unconfirmed*. That property is untouched — this
document is not proposing that a diverged anchor becomes evidence the
comment is incorrect, which would be a different and unjustified claim.

What changed is the **policy** wrapped around the check's result. Previously,
an unconfirmed anchor was tolerated by default and only became fatal under
`--require-verification`; now every unconfirmed anchor is fatal, always, with
no flag to ask for anything softer. The check still only withholds; the
submission, as of this decision, treats a withheld verification the same way
it always treated a wrong one — a document not fit to store — even though
those remain two different findings about the world. Keeping this seam
visible in the prose matters more here than anywhere else in this document:
it is the difference between "the check got stricter" (false) and "the tool
got stricter about what it accepts from the check" (true).

#### 3.4.3 The uncommitted-work consequence

The sharpest instance of [§3.4.2](#342-the-check-and-the-policy-are-not-the-same-thing)
is [§4.2](#42-head-and-diverging-working-trees)'s own subject: a review of
uncommitted work. `anchor-worktree-diverged` fires whenever `ref` is `HEAD`
and an anchored file's working-tree copy no longer matches the blob at
`ref` — which is exactly the ordinary state of a checkout somebody is
actively editing. Under the previous policy, that fired, was reported, and
the submission still stored, unverified anchor and all. Under this one, it
fails the submission outright.

**A review of a dirty working tree therefore cannot be submitted.** This is
a real constraint on any orchestrator that wants a reviewer to look at
in-progress, uncommitted changes, and stating it plainly is better than
letting it be discovered as a surprising wall of exit-1 failures. The
workflow it forces: commit what was reviewed — even to a throwaway branch —
or use `git stash create`, which builds a real commit object out of the
working tree and the index without touching either, giving the reviewer a
genuine, resolvable SHA to anchor against instead of `HEAD` plus a promise
that nothing will move. Either way, `submit-review` now requires the
reviewed state to be a commit, not a moment.

#### 3.4.4 The unreachable-ref consequence

The same policy shift has a second consequence [§3.4.3](#343-the-uncommitted-work-consequence)
does not cover, worth tracing because it was the other legitimate use
`--warn-only` served. `ref-unknown` fires when the document's `ref` does
not resolve in the repository at all — a shallow clone that never fetched
the reviewed commit, or a branch deleted and garbage-collected since.
[../cli.md §2.3.1](../cli.md#231-verifying-anchors) previously named this
explicitly as the case `--warn-only=ref-unknown` existed for: "a repository
legitimately lacking the reviewed commit." That escape hatch is gone along
with the rest of `--warn-only`.

**A reviewer working from a shallow clone that does not contain the
reviewed commit can no longer submit a document with any anchors in it, and
there is no flag to work around it.** The only fix is a deeper fetch before
submitting — `git fetch --deepen` or an unshallow clone — not a command-line
option. The same holds for running `submit-review` **outside a repository
entirely**: previously "ordinary," reporting `source: none` and passing
regardless
([../cli.md §2.3.1](../cli.md#231-verifying-anchors), "Running outside a
repository is ordinary... Neither fails the run"); now, any document that
carries at least one anchor fails, because nothing can confirm an anchor
with no repository to ask. The one case this does not touch is a document
with **no anchors at all** — an `approve` verdict with no comments, or
comments that are architectural and carry none
([../review-document.md §5](../review-document.md#5-anchor-object),
"Architectural findings that have no single location may carry no anchors
at all") — there is nothing for verification to confirm, so nothing about
this policy applies to it, the same carve-out the old
`--require-verification` flag already made explicit.

#### 3.4.5 What `verification-required` now means

No new check name is introduced. `verification-required` already existed as
the name `--require-verification` produced when any anchor claim went
unchecked — "no repository, an unreachable one, a single file git could not
read, or an anchor into a file that has diverged"
([../cli.md §2.3.1](../cli.md#231-verifying-anchors)). Its trigger condition
is unchanged; what disappears is the flag that used to gate whether it ran
at all. `verification-required` simply always runs now, the way every other
verification check always ran — no repurposing, per
[../review-document.md §11.5](../review-document.md#115-name-stability)'s
own rule that a check that outlives a piece of its context is removed or
left alone, never quietly redefined.

## 4. Ref as the join key

Today `ref` decides one thing: what an anchor resolves against
([../review-document.md §5.1](../review-document.md#51-refs)). Under this
feature it decides a second thing — which reviews belong in the same
combined output — and that is a heavier job than the first one, because
nothing has ever checked whether `ref` is *honest*, only whether it is
*well-formed and resolvable*. `ref-format`
([../review-document.md §11.1](../review-document.md#111-structural-checks--hard))
checks the shape. `ref-unknown`
([../review-document.md §11.2](../review-document.md#112-verification-checks--conditional))
checks that the SHA exists in the repository. Neither checks that it is the
commit the reviewer actually read, and nothing else in the format could:
`ref` is caller-authored, exactly the same way `id` is
([../review-document.md §4](../review-document.md#4-comment-object)) and the
same way `profile` will be ([§7.2](#72-caller-authored-and-unverifiable)).
Say that plainly rather than pretend `collect-reviews` adds provenance it
does not have — [§3.4.1](#341-why-nothing-enters-the-store-unverified)
narrows what verification can confirm about a submission, but it was never
going to be able to confirm this.

### 4.1 A ref that resolves to the wrong commit

A reviewer that computes `ref` wrong — a script bug, a stale variable, an
agent that reviewed one commit but resolved `HEAD` a moment after something
else advanced it — produces a document that is structurally perfect and
verifies cleanly against the *wrong* tree. Nothing in `submit-review` can
catch it, even now: the ref it names is real, the anchors it carries
genuinely exist at that ref, and every check this format defines passes.
[§3.4](#34-verification-is-required-to-submit) makes verification
unconditional; it does not make it able to ask a question it was never
designed to answer.

Under `reviews`, that mistake is contained: the review files under a ref
nobody asked about, and a caller has to go looking for it to be misled by
it. Under `collect-reviews`, the same mistake actively *recruits* other
reviews into its error — a wrong-but-valid ref silently files a review into
whatever combined output that ref produces, sitting attributed and
plausible beside reviews that got it right, and nothing about the combined
output's shape distinguishes them. This is not a gap this feature can close;
it is a gap this feature inherits and should not pretend to close. The
honest statement is the one this document keeps returning to: `ref`, like
`id` and `profile`, is a claim the tool cannot verify, only take at face
value.

### 4.2 HEAD and diverging working trees

The sharper version of the same problem is what happens to reviews of
**uncommitted** work — which, as of [§3.4.3](#343-the-uncommitted-work-consequence),
can no longer reach the store at all in their dirty form. That closes one
gap this section used to describe and leaves a narrower one open, and it is
worth being precise about which is which.

A reviewer looking at a dirty working tree has no commit to name, so it
resolves `git rev-parse HEAD` and names the nearest real commit there is.
Two reviewers launched against the same checkout — a `go` profile and a
`security` profile, say — can both resolve the same `HEAD` SHA while the
working tree itself changes in between: a file edited after the first
reviewer read it and before the second one started. Under
[§3.4](#34-verification-is-required-to-submit), each individual submission
is now guaranteed to have matched `ref` cleanly **at the moment it was
submitted** — a submission that did not match no longer reaches the store
at all. What is **not** guaranteed is that the two submissions matched
*each other*: the tree can still have moved between the first reviewer's
submit call and the second one's, in a way that happens not to touch either
one's own anchored files, and both would still submit cleanly while having
read genuinely different states of the same nominal `ref`.

This is the interaction the `anchor-worktree-diverged` check
([../cli.md §2.3.1](../cli.md#231-verifying-anchors)) exists for, and it
remains monotonic — it can only *withhold* a verification, never *grant*
one — even though [§3.4](#34-verification-is-required-to-submit) now makes
withholding fatal at submit time rather than merely reported. A clean
`submit-review` run is evidence that, **at the moment that run executed**,
the working tree matched `ref` for every file that run's anchors touched.
It says nothing about a *different*, later-verified run against the same
nominal `ref`. The check certifies each submission against itself. It was
never designed to certify two submissions against each other, and nothing
about making it mandatory changes that.

### 4.3 The head check

[§3.4](#34-verification-is-required-to-submit) narrows this section's job
considerably from the first draft. Under the previous, optional-verification
design, `collect-reviews` had to assume a stored review's anchors might
never have been checked at all, so its own recheck had to answer "is this
anchor sound, full stop" from scratch, with no help from what submission
time had already established. Under the current design, that assumption is
gone: **every stored review was fully verified against `ref` at the moment
it was submitted.** `head_check`'s job at collection time is therefore just
one narrower question — **has the tree moved since** — not the broader one
the first draft had to ask.

**Decision.** When `--ref` names the repository's current `HEAD`, and a
repository is available to ask, `collect-reviews` re-runs the same
divergence check — `anchor-worktree-diverged`, the identical check
[§3.4.2](#342-the-check-and-the-policy-are-not-the-same-thing) leaves
untouched, not a new one — against every anchor carried by every comment in
the combined output, fresh, at collection time. This answers "does this
anchor, confirmed good at submission, still match `ref` right now" — a
drift check, not a from-scratch confirmation. The property that licenses it
is the same monotonicity [§3.4.2](#342-the-check-and-the-policy-are-not-the-same-thing)
preserves: the recheck can only remove an anchor from "still matches," never
add one, so running it a second time, at a second call site, withholds
confidence rather than manufacturing it.

#### 4.3.1 The `head_check` shape

Modeled directly on `verification.unverified`
([../cli.md §5.2](../cli.md#52-the-result-object)), because it is the same
kind of fact reported at a second call site, and that section already
specifies every case this one needs to answer.

`head_check` is a top-level object, **always present**, on the same
"never silently pass, never silently omit" posture `verification` already
commits to — "A caller that treats a missing `verification` block as
'verified' is reading an older version."

| Field | Present when | Meaning |
| --- | --- | --- |
| `source` | always | `"repo"` when the check ran, `"none"` when `collect-reviews` was not run inside a checkout, `"unavailable"` when a repository exists but could not be asked — the same three values `verification.source` uses, for the same reasons. |
| `is_head` | `source == "repo"` | Whether `--ref` names the repository's current `HEAD`. Absent under `"none"` and `"unavailable"`: a caller cannot tell whether `--ref` is `HEAD` without a repository to ask, so the field is absent rather than guessing `false`. |
| `diverged` | `source == "repo"` **and** `is_head == true` | An array, one entry per anchor the recheck found has drifted from `ref` since it was confirmed at submit time — `[]` when the recheck ran and found nothing. Absent, not `[]`, when `is_head` is `false`: the check does not apply to a non-`HEAD` ref, so there is nothing to report. Absent under `"none"`/`"unavailable"` for the same reason `is_head` is. |

Each entry in `diverged`:

```json
{
  "name": "anchor-worktree-diverged",
  "comment": "backend:dropped-context-1",
  "file": "internal/fetch/client.go",
  "message": "internal/fetch/client.go differs from ref in the working tree"
}
```

`name` is always `anchor-worktree-diverged` — carried anyway, the same way
`verification.unverified[].name` is, so a consumer never has to special-case
a field that happens to be constant today. `comment` is the **qualified**
id ([§6.1](#61-the-qualified-id)), not the origin id: `verification.unverified[].comment`
is scoped to one submitted document and can use the plain id safely, but
`head_check.diverged` spans every surviving submission, so it has to use the
same identifier the rest of the combined output does. `file` is the
anchored path directly, not a JSON Pointer: `verification.unverified[].path`
points into the one document `submit-review` is checking, and there is no
single document here for a pointer to point into.

**A diverged anchor does not remove its comment from `comments`.** The
comment appears in full — every other field, every other anchor — with this
one anchor's entry in `diverged` as the only trace that it no longer matches
`ref` right now. It was confirmed once, at submission — that confirmation is
not retracted, only flagged as possibly stale.

#### 4.3.2 Implementation route

Three routes exist for producing `diverged`, and the document should name
one rather than leave the choice open, because they have different costs to
the codebase's own architecture.

1. **Export new API on `internal/verify`.** `worktreeDiverged` is
   unexported on `*verify.Repository` and `refIsHEAD` is unexported on
   `*verify.Verifier`; exporting narrow equivalents of both would let a
   caller ask the one question `head_check` needs directly. The cost: today
   `internal/verify` is imported only by `cmd/loam-refinery` (`main.go`,
   `reviews.go`) — `internal/cli` has never imported it. Wiring `head_check`
   this way would be the first time `internal/cli` depended on
   `internal/verify` in production, a new edge in the dependency graph for
   one feature.
2. **Re-parse and filter by check name.** The only exported door on
   `*verify.Verifier` is `Verify(ctx, doc) ([]review.Diagnostic, []review.Skipped, review.Verification)`,
   which also runs `ref-unknown`, `anchor-file-missing`, and
   `anchor-line-out-of-range` — checks whose answers cannot have changed
   since submission, because `ref` is an immutable SHA and, as of
   [§3.4](#34-verification-is-required-to-submit), every stored document
   already passed them once. Calling it anyway and reading only
   `Verification.Unverified` — "one entry per anchor a dirty working tree
   kept from being checked: `anchor-worktree-diverged`, and nothing else,
   can populate this," per that type's own comment in
   `internal/review/result.go` — gets `diverged` without exporting anything
   new.
3. **Put it on a `cmd/loam-refinery` adapter.** `collect-reviews`'s own
   command logic — like `reviews`'s — lives in `internal/cli`, behind a
   small interface `internal/cli/interfaces.go` defines; the concrete
   implementation, in `cmd/loam-refinery`, is free to import
   `internal/verify` the way `reviewsAdapter` already does for
   `verify.Discover`.

**Decision.** Routes 2 and 3 together: `internal/cli` gains a small
interface asking exactly the one question it needs, and `cmd/loam-refinery`
implements it by calling the existing, already-exported `Verifier.Verify()`
on the same parsed `review.Document` `collect-reviews` already has to parse
to build `comments` in the first place, reading `Verification.Unverified`
back out. Rejected: route 1, the only one that changes production's own
dependency graph. The cost of the chosen route: every collection re-runs
`ref-unknown`, `anchor-file-missing`, and `anchor-line-out-of-range` for
every anchor in every surviving comment, computing answers already
confirmed once at submit time, to reach the one that might have changed
since. That is bounded by the same per-anchor cost `submit-review`'s own
verification already pays once per call
([../cli.md §6.1](../cli.md#61-budgets)), paid a second time here.

**What this does not solve, stated plainly.** Even a fully clean
`head_check` — `diverged: []` — does not prove the two submissions in
[§4.2](#42-head-and-diverging-working-trees) reviewed the same uncommitted
state as each other, only that neither has drifted from `ref` since each was
confirmed. Whether the tree looked the same at the two *submission* times is
a question this design has no way to answer, because nothing records what
the working tree looked like at either moment beyond the anchors each
reviewer happened to touch. This residual gap is real and is left explicitly
open in [§13](#13-open-questions) rather than papered over with a check that
would look more reassuring than it is.

## 5. Merge semantics

### 5.1 Why not by slug

Two different profiles, launched with no shared naming convention between
them, can each independently choose the same slug for two unrelated
findings — `unchecked-error` is a plausible slug for a `go` profile and for
a `security` profile alike, chosen with no coordination and no knowledge of
the other. Fusing two comments into one entry on the strength of that
coincidence — keeping only the higher priority, unioning suggestions that
answer two different problems — would silently discard whichever of the two
lost the priority comparison, on evidence that amounts to nothing more than
two reviewers happening to reach for the same word. That argument does not
need support from anywhere else in the format; two reviewers with no shared
vocabulary landing on the same slug is reason enough on its own.

It is a different question, and one this document does not need to answer
the same way, whether a *single* reviewer's own reuse of a slug across
several of its own comments is a problem — it is not. `id-unique`
guarantees uniqueness within one document; the slug is what
[../review-document.md §4](../review-document.md#4-comment-object) calls
"the grouping mechanism," letting `missing-context-1` through
`missing-context-4` collapse into one theme for a reader who wants that —
"a consumer can collapse [them] into one theme without reading four
bodies." That is a controlled vocabulary one reviewer authored end to end,
and nothing about cross-profile fusion's problem applies to it. Two
different reviewers landing on the same slug by coincidence is not that
case, and this document does not claim otherwise.

This document takes a different mechanism from
[../cli.md §8](../cli.md#8-future-considerations)'s sketch for identifying
which comments to fuse across submissions — not a repair of a design that
was unsound, a different choice. `§8`'s own closing sentence, "Suffixes are
renumbered on merge, so IDs are stable within a document but not across
them," already anticipates more than one surviving entry per slug, which
sits closer to what [§6.1](#61-the-qualified-id) actually builds than a
lossy per-slug collapse would. What changes here is the disambiguator: `§8`
reached for a renumbered suffix because nothing else was available to it;
[§5.2](#52-grouping-by-profile-not-by-slug) reaches for `profile` instead,
because `profile` did not exist when `§8` was written and gives a strictly
better one — legible on sight, and the reason [§6.1](#61-the-qualified-id)
can recover the original id by splitting a string instead of remembering an
assignment.

### 5.2 Grouping by profile, not by slug

**Decision.** `collect-reviews` never fuses two comments from different
profiles into one entry, regardless of slug. Every surviving comment
([§5.3](#53-multiple-submissions-one-ref-enumeration-and-survival)) appears
in the combined output as its own entry, carrying its own profile, its own
priority, its own body, its own anchors, its own suggestions — untouched
from what its submission recorded.

This is not a retreat from "merged" in the sense this feature promises. What
"merged" means here is that N stored documents become one combined document
— the orchestrator's whole ask, stated in the bead this feature comes from,
is "an orchestrator running several reviewers... collects their findings in
one call instead of N." It never required that two reviewers' *findings*
about arguably the same code be fused into fewer findings, and fusing them
would require the tool to judge that two independently worded comments are
"the same" — which is adjacent to, and arguably harder than, "assessing the
truth of a review's claims," already out of scope
([../cli.md §1](../cli.md#1-overview)). Two comments from two profiles that
land on the identical anchor — same `file`, `line`, `end_line`, the exact
identity `duplicate-anchor` already uses
([../review-document.md §11.3](../review-document.md#113-advisory-checks--soft))
— are a genuinely useful **signal**: independent convergence is stronger
evidence than either reviewer alone. Surfacing that signal without fusing
the comments it comes from is real, useful work this document does not
attempt to specify, and is recorded as deferred in
[§13](#13-open-questions) rather than built on the strength of one design
pass.

### 5.3 Multiple submissions, one ref: enumeration and survival

[../config.md §5](../config.md#5-storing-a-review) stores a review for every
run that validates clean. An agent that submits, revises, and resubmits
under the same claimed profile leaves more than one document in the store
for the same ref, and this section has to answer two separate questions
about that: what counts as *one* review, and what happens to an *earlier*
one once a later one exists.

#### 5.3.1 One review is one digest, not one row

[../config.md §4.4](../config.md#44-the-stored-files) says a byte-identical
resubmission is "an `O_EXCL` create that fails with `EEXIST`... One of them
creates the file, the other is told it exists, **and both record a run**."
[../config.md §4.5](../config.md#45-the-database) is explicit that the
database "holds a row per **run** — not per stored review." One file, two
rows. A design that enumerated rows would therefore see a byte-identical
resubmission as *two* submissions sharing one digest — and, combined with
[§5.3.2](#532-marking-not-deleting) exempting nothing from survival, every
comment in that document would appear twice under what would then be the
*same* qualified id, exactly the collision [§6.1](#61-the-qualified-id)
exists to prevent.

**Decision.** `collect-reviews` enumerates **distinct digests** under
`<store>/reviews/<repo>/<ref>/` ([../config.md §4.1](../config.md#41-layout)),
not rows in `runs`. This is the identity `reviews` already declines to use
for its own index — that command deliberately reports one row per run,
because a run log is the point of it: repeated identical attempts are
exactly the signal "what does this agent keep getting wrong" wants
([../config.md §4.5](../config.md#45-the-database)). `collect-reviews` is
answering a different question — "what reviews exist for this ref" — and
for that question a resubmission that reproduced the same bytes is the same
review, not a second one. Where a digest has more than one row pointing at
it, its `at` is the **earliest** of them. Neither `digest` nor `at`
appears anywhere in the response — [§8.1](#81-shape) states the principle
this section's own machinery has to respect regardless: the combined output
exposes no storage concepts, so both stay internal, used only to determine
identity and ordering.

#### 5.3.2 Marking, not deleting

Near-identical resubmissions — a revise-and-resubmit that changes the bytes
— do not collapse under [§5.3.1](#531-one-review-is-one-digest-not-one-row):
two distinct digests, two distinct files, both real reviews. Deletion-based
supersession — keeping only the most recent submission per profile and
discarding the rest — was considered and rejected: `profile` is
caller-authored and unverifiable ([§7.2](#72-caller-authored-and-unverifiable)),
this repository ships eight profile files under `profiles/` for an operator
to copy into a shared config directory
([../cli.md §2.1.1](../cli.md#211-reviewer-profiles)), and nothing stops two
genuinely different reviewers from both writing `profile: "go"`. Deleting on
that coincidence would erase one reviewer's findings on no stronger evidence
than a matching string neither side chose with the other in mind — exactly
the failure this format degrades away from everywhere else, toward *reported
but unverified* rather than *silently absent*.

**Decision.** Every distinct digest for the ref survives into
`submissions` and contributes its comments — nothing is ever excluded on
the strength of `profile`. What "supersession" means here: within one
claimed `profile`, submissions are ordered by `at`, used purely as an
internal mechanism and never reported ([§8.1](#81-shape)) — ties broken on
`digest`, also internal only. Every submission other than the most recent
for that profile carries `superseded_by`, naming the **ordinal**
([§8.1](#81-shape)) of the one that is current, since neither `at` nor
`digest` is in the output for it to name instead. A caller that wants only
current opinions filters out anything carrying `superseded_by`. A caller
that wants the full picture reads everything. Nothing is ever silently
gone; the worst outcome of a coincidental profile collision under this
design is a `superseded_by` pointer between two submissions that were not,
in fact, the same reviewer — confusing metadata, not a missing review.

This reopens a cost [§10](#10-budget) concedes for a different reason: a
command with no `--limit` that never excludes anything can grow without
bound as reviewers accumulate submissions against one ref. Accepted here as
the honest trade against silent data loss, the same trade
`reviews --content` already makes in
[../config.md §6.1](../config.md#61-output).

#### 5.3.3 The tie-break is now cosmetic

Two submissions under the same claimed profile with the same stored `at` —
possible under WAL mode's busy-timeout with concurrent writers
([../config.md §4.6](../config.md#46-concurrency-and-locking)) — need a
deterministic answer for which one is "current" for the purpose of
`superseded_by`, and the tie breaks on `digest`, arbitrarily but
consistently, so repeated calls against an unchanged store return the same
output. Since [§5.3.2](#532-marking-not-deleting) never deletes either one,
the tie-break decides only which of two simultaneously-submitted reviews
gets the plain qualifier and which gets `superseded_by` pointing at the
other — both are still fully present in the output either way, so getting
the direction "wrong" costs a caller nothing but which one they notice
first.

That "repeated calls... return the same output" claim is narrower than it
sounds, and the two halves are worth separating. Against an **unchanged**
store it is unconditionally true, ordinals included — nothing in this
section depends on anything that could differ between two calls with no
`submit-review` in between. Against a **growing** one it is not: `ordinal`
([§8.1](#81-shape)) is a position, not a content-derived value, and a new
submission arriving can shift the position — and therefore the
`ordinal` — of a submission that was already there, before this
tie-break even runs. [§8.1](#81-shape) and [§6.1](#61-the-qualified-id)
both say this plainly rather than let the tie-break's own determinism imply
a stronger guarantee than the field it now points at actually carries.

### 5.4 Verdict: reported, never reconciled

**Decision.** The combined output carries no synthesized verdict. Every
surviving submission's own `verdict` appears beside its `profile` (or lack
of one) in `submissions`; nothing computes "the" verdict for the ref by
taking the worst of them, the most recent of them, or a majority.

This is the direct continuation of a rule the format already states:
"[nothing here] relates `verdict` to `priority`... the verdict is a judgment
about whether the change should land... and reconciling them is the
reviewer's job"
([../review-document.md §11.3](../review-document.md#113-advisory-checks--soft)).
Reconciling *across* reviewers is the same judgment one level up, and it is
no more this tool's to make than reconciling within one review is. It is
also the direct continuation of
[../cli.md §1](../cli.md#1-overview)'s sixth design principle, "Advise,
don't police" — a combined verdict would be exactly the kind of policing
that principle already rules out, just applied to a set of reviews instead
of one.

`submissions[].superseded_by` is the honest amount of "current state"
tracking this section allows: a caller can tell which submission per
profile is most recent without the tool ever deciding what the ref's
*overall* verdict is, which stays exactly as unreconciled as the rest of
this section argues it should.

### 5.5 Summary

**Decision.** Carried per submission, verbatim, never concatenated or
rewritten. Each entry in `submissions` carries its own `summary` beside its
own `verdict` and `profile`. A combined output that tried to merge four
reviewers' one-to-three-sentence summaries into one paragraph would produce
exactly the kind of prose this format exists to replace — a summary of
summaries is a new document nobody wrote and nobody can hold accountable for
being accurate, the opposite of what every field in this format is for.

### 5.6 Anchors: why unioning turned out to be moot

[../cli.md §8](../cli.md#8-future-considerations)'s sketch implied
per-group anchor unioning as part of fusing same-slug comments together.
Once [§5.2](#52-grouping-by-profile-not-by-slug) rules out that fusion, the
question dissolves rather than gets answered: there is no group for anchors
to be unioned *into*. Each surviving comment keeps exactly the anchors its
own submission gave it, and `collect-reviews` never combines the anchor
lists of two different comments — the case that would have made unioning
necessary never arises under this design.

### 5.7 Two comments, one slug, two profiles

This is the concrete answer to the hardest sub-question in
[§5](#5-merge-semantics): what happens when two comments share a slug —
even the exact same `id` — but describe substantively different things.
Under [§5.2](#52-grouping-by-profile-not-by-slug), nothing distinguishes
that case from two comments that share a slug and describe the *same*
thing: neither is ever fused. Both appear, distinctly, each under its own
qualified id ([§6.1](#61-the-qualified-id)), each carrying its own body,
priority, and anchors. The same is true, for a different reason, of two
comments that share both a slug *and* a profile because one submission
superseded the other ([§5.3.2](#532-marking-not-deleting)): they are not
fused either, and [§6.1](#61-the-qualified-id) is what keeps their ids from
colliding once both are visible at once. The worked examples in
[§12](#12-worked-example) show both cases.

## 6. Addressing across the merge

### 6.1 The qualified id

Two different profiles can, and eventually will, choose the identical `id`
for two unrelated findings — `id-unique`
([../review-document.md §11.1](../review-document.md#111-structural-checks--hard))
only guarantees uniqueness *within* one document, and nothing coordinates
slug vocabulary *between* reviewers. Worse, once
[§5.3.2](#532-marking-not-deleting) keeps every submission rather than
deleting superseded ones, the *same* profile can carry the identical `id`
twice on its own — a revise-and-resubmit ordinarily keeps the id for a
finding that persisted across the revision. A combined output that carried
submitted ids verbatim could therefore produce two, or more, entries named
`dropped-context-1`, which breaks
[../cli.md §1](../cli.md#1-overview)'s fifth design principle outright:
"Everything is addressable. Every comment carries an ID the reviewer chose.
Diagnostics, downstream agents, and humans all refer to findings by that
ID." An id that does not uniquely name one finding in the output that names
it has stopped being an id.

**Decision.** Every comment's `id` in the combined output is:

- `<profile>:<origin_id>` — when that comment's submission is the
  **current** one for that profile, i.e. it does not carry
  `superseded_by` ([§5.3.2](#532-marking-not-deleting)). This is the common
  case and the readable one: `backend:dropped-context-1`.
- `#<ordinal>:<origin_id>` — otherwise: `profile` is absent, or `profile`
  is present but this submission carries `superseded_by`. `ordinal`
  ([§8.1](#81-shape)) is the submission's own 1-based position in the
  `submissions` array — `#3:dropped-context-1`. There is no `digest` in the
  output for this to name instead ([§8.1](#81-shape) says why), and
  `ordinal` is what replaces it. Note what switching qualifier form does
  **not** do: it does not remove `profile` from the comment. `profile`
  answers who wrote the finding; the qualifier, and `superseded_by` on the
  matching submission, answer whether it still stands
  ([§8.1](#81-shape)) — two questions, two signals, never collapsed into
  one field.

At most one surviving submission per claimed profile name is ever
"current" — [§5.3.2](#532-marking-not-deleting) orders them by `at` and
[§5.3.3](#533-the-tie-break-is-now-cosmetic) breaks ties deterministically
— so two "current" submissions never share a profile name, because only one
per name can hold that status. Two ordinal-qualified submissions never
share an ordinal, because array positions are assigned once and are
distinct by construction. Combined with `id-unique` guaranteeing
`origin_id` is unique *within* the submission that produced it, the full
qualified id is unique across the entire combined output, at the moment it
is produced, in every case — not merely the common one this document leads
with. **"At the moment it is produced" is doing real work in that
sentence**, and [§8.1](#81-shape) is specific about why: `ordinal` is a
position, not a content-derived value like a digest was, and it is not
guaranteed to still name the same submission on a later call once the store
has grown. Uniqueness within one response is unconditional; stability of a
given ordinal-qualified id *across* responses is not, and this document
does not claim otherwise.

The `#` prefix is not decoration — it is what keeps the two qualifier forms
from colliding. `profile-format` ([§11.6](#116-profile-format-a-shape-check))
allows an all-digit name (`^[a-z0-9]+(-[a-z0-9]+)*$` matches `"3"`), so a
bare ordinal like `3:dropped-context-1` would be genuinely ambiguous with a
profile literally named `3`. `#` is outside both the profile grammar and
the comment-id grammar, so `#3:dropped-context-1` can only ever be read one
way. It is also, incidentally, not a valid ATX heading marker when it
appears mid-line the way [§8.3](#83-the-markdown-projection)'s renderer
would show it — CommonMark requires a space after a heading `#`, and
`#3` has none — so the choice costs the Markdown projection nothing either.

This reuses, deliberately, the namespacing idiom `describe --lens` already
establishes for its own entry names — `field:`, `check:`, `topic:`
([../cli.md §2.2.1](../cli.md#221-lens-names)) — rather than inventing a
second one for the common, profile-qualified case. It is also a
refinement, not a repair, of
[../cli.md §8](../cli.md#8-future-considerations)'s original plan to
renumber suffixes ([§5.1](#51-why-not-by-slug)): `#3` is exactly that
renumbered suffix, reached for only when there is no profile to qualify
with instead, rather than the universal mechanism `§8` originally sketched.

Parsing the common case is unambiguous because the two halves come from
disjoint character sets, **provided `profile` is shaped the way
[§11.6](#116-profile-format-a-shape-check) proposes**: a profile name is
lowercase letters, digits, and hyphens, no colon possible
([../cli.md §2.1.2](../cli.md#212-the-profile-file)), and a comment id
matches `^[a-z][a-z0-9]*(-[a-z0-9]+)*-[1-9][0-9]*$`
([../review-document.md §4](../review-document.md#comment-ids)), also no
colon possible. Without that shape check, `profile` is only constrained by
[§7.2](#72-caller-authored-and-unverifiable)'s honesty argument to being a
string — nothing stops `profile: "arch:v2"`, which would produce
`arch:v2:dropped-context-1` and break the very claim this paragraph makes.
[§11.6](#116-profile-format-a-shape-check) closes this with a structural
check — shape only, no claim about honesty, exactly like `ref-format` — and
it is a precondition for this section's own round-trip claim to hold, not
an optional nicety. `origin_id` itself is not a separate field on the
comment ([§8.1](#81-shape)) — it is always the substring after the first
colon in `id`, recoverable by every caller the same way, never carried
twice.

### 6.2 Routing feedback back to a reviewer

The qualified id answers "which finding" inside the combined output. This
section used to answer a harder question with a second call — look up the
exact stored file behind a comment via its `digest`. That recipe is gone
along with the field it depended on, and the reason is the same one
[§8.1](#81-shape) opens with: a feature whose whole purpose is gathering
findings in **one** response should not hand a caller a pointer that costs
a second one to follow.

What survives is what an orchestrator actually needs to route a piece of
feedback — "fix `backend:dropped-context-1`" — back to the reviewer that
raised it, and it turns out to already be in the response, with nothing
further to fetch:

- **`profile`**, when present, names which reviewer to re-invoke — a fresh
  instance of that profile, asked to revise.
- **The origin id**, recoverable from `id` by splitting on the first colon
  ([§6.1](#61-the-qualified-id)), is what that reviewer knows the finding
  as. It was never a separate field to look up; it is always the same
  string, at a fixed position in `id`, for every comment.

That is the whole recipe: `id.split(":", 1)` gives the profile-or-ordinal
qualifier and the origin id in one step, no second call, no file to open.
What this does **not** give back, and could before: the exact original
document, byte for byte — its precise summary wording, every other comment
that reviewer made in the same submission, the verdict it reached alongside
this one finding. `profile` and `verdict` are still in `submissions[]`, so
the reviewer's overall disposition is not lost, only the ability to
retrieve the submission verbatim. An orchestrator that genuinely needs the
original bytes — for an audit trail, say, rather than for routing feedback
— is asking a `reviews`-shaped question, not a `collect-reviews`-shaped
one, and pays the second call `reviews --ref=<ref> --content` was always
the tool for.

**A comment with no `profile`** has no reviewer to route feedback back to
at all — an unprofiled submission is, by definition, one nobody told
`collect-reviews` how to re-invoke. That is not a gap this section can
close; it is the same honesty [§7.2](#72-caller-authored-and-unverifiable)
already extends to a review submitted with no profile in the first place.

## 7. The profile field

**Decision.** A new **optional** root field, `profile`, string, on the
review document. Set by the submitting reviewer, carrying the name it was
primed with — the same `NAME` passed to `prime --profile=NAME`
([../cli.md §2.1.1](../cli.md#211-reviewer-profiles)) — or omitted when the
reviewer ran unprimed.

`collect-reviews` is the reason this field exists. `prime --profile=NAME`
already tells a reviewer what to look for; nothing before this feature told
anything downstream what a reviewer *had been* told, and a document written
under a profile looked byte-for-byte identical to one written without any
profile at all. `profile` is what a submitted document says about itself
that `collect-reviews` later reads back.

**The compatibility direction that matters is not the obvious one.** An
added optional property can never invalidate a document that omits it, in
any schema, always — that direction cannot break and needs no defense. The
direction that actually breaks runs the other way — a **new** document,
carrying `profile`, submitted against an **old** binary.
`additionalProperties: false` sits at the schema root today
([`internal/schema/review.schema.json`](../../internal/schema/review.schema.json)),
alongside five properties, none of them `profile`; an old binary's copy of
that schema rejects any document naming a property it does not know, and a
reviewer that includes `profile` fails `schema` outright — exit 1,
structurally invalid.

That failure lands on a reviewer whose contract-correct response, per
`prime.txt`'s own taught loop, is to delete the field it does not recognize
and resubmit — silently discarding attribution and supersession-eligibility
for a review that was otherwise completely sound, on a failure that is a
rollout ordering problem being misreported as a defect in the review. It
gets worse under [§3.4](#34-verification-is-required-to-submit): the failed
run is now guaranteed to be recorded, since every run records a row
regardless of outcome, and shows up in `reviews --failed` as a false signal
that this reviewer produced a bad review.

**The rollout constraint this implies:** the schema change proposed in
[§11.5](#115-the-root-object-table-and-the-schema) must reach every binary
an orchestrator might run a reviewer against **before** any `prime.txt`,
profile file, or prompt teaches a reviewer to emit `profile` at all. This is
not unique to `profile` — `additionalProperties: false` means every future
field this format ever adds carries the identical exposure — and this
document is not proposing a general fix for it, because there is not an
obvious one that does not weaken the very property "unknown field"
protection exists for
([../review-document.md §3](../review-document.md#3-root-object)).

This is the same discipline [§3.2](#32-no-alias-no-migration-plan) declines
to soften for the opposite reason. There, an old caller typing the old
command name against a new binary simply breaks, on purpose, with no
alias to catch it. Here, a new document field reaching an old binary's
schema breaks the same way, with no fallback to catch it either — but this
one is not an accepted breaking cost the way [§3.2](#32-no-alias-no-migration-plan)'s
is, because it is avoidable by sequencing: ship the schema everywhere before
teaching anyone to use it. The two sections reach different postures — one
accepts breakage outright, one insists on an ordering that prevents it —
because one is a one-time cost paid by whoever runs the tool today, and the
other is an ongoing cost that would be paid by every reviewer that gets
upgraded prompts before an upgraded binary, for as long as that gap persists
on a given machine.

### 7.1 Why not lens

The obvious name loses to a collision. `lens` already names, unambiguously,
`describe --lens`, the `lenses` field on every `submit-review` result
([../cli.md §5.2](../cli.md#52-the-result-object)), and the namespace
resolution `field:` / `check:` / `topic:` in
[../cli.md §2.2.1](../cli.md#221-lens-names) that a future `kb:` namespace
is reserved to extend
([../cli.md §8](../cli.md#8-future-considerations)). A document field
called `lens` would sit in a JSON object one property away from a
`lenses` array meaning something else entirely, and an agent that has just
read `describe`'s explanation of what a lens is would have every reason to
assume the two are related. They are not: a lens is a unit of the tool's
own explanation, pulled on demand; a profile is operator-authored prose
about who is reviewing, pushed into context before the reviewer starts
([../cli.md §2.1.1](../cli.md#211-reviewer-profiles)). Naming the document
field `profile` instead avoids manufacturing that confusion, and it is also
the word already in the caller's own hands — `prime --profile=NAME` — so no
new vocabulary crosses the boundary between what an orchestrator types and
what a reviewer writes back.

### 7.2 Caller authored and unverifiable

`profile` joins `id` and `ref` as a document field the tool takes entirely
on faith. Nothing checks that a reviewer primed with `--profile=backend`
actually wrote `profile: "backend"` into what it submitted, nothing checks
that it did not write a *different* profile's name, and nothing checks that
it wrote one at all when it should have. A reviewer could claim a profile it
was never primed with, or claim one shared with an unrelated reviewer
entirely by coincidence — the exact case [§5.3.2](#532-marking-not-deleting)
now designs around rather than trusts — and the document would validate
exactly as cleanly either way, even under
[§3.4](#34-verification-is-required-to-submit)'s stricter posture:
verification confirms anchors, never the honesty of `profile`.

**A shape check is not a honesty check, and costs this section nothing.**
[§11.6](#116-profile-format-a-shape-check) proposes `profile-format`, a
structural check constraining `profile` to the same character grammar
`cli.md §2.1.2` already defines for the *filename* a profile resolves to. It
rejects `profile: "arch:v2"` for containing a colon, the same way
`ref-format` rejects a branch name for not being 40 hex characters — on
shape, unconditionally, with no claim about whether the value names a real
profile or the profile the reviewer actually ran under.

## 8. The output is not a review document

### 8.1 Shape

The combined output is a JSON object, but it does not satisfy the
review-document schema and was never meant to. **It is also self-contained
and exposes no storage concepts: nothing in it requires a second call to
resolve, and nothing in it describes how or where the tool keeps things.**
That single sentence is the test every field in this section has to pass.
An earlier draft failed it in three places, and this revision removes all
three rather than explain them away.

The envelope reuses vocabulary this format already commits to rather than
re-spelling it: `ref` is a bare string, the same shape it has on every
`reviews` row; `total` keeps the job
[../config.md §6.1](../config.md#61-output) already gives it — meaning here
the number of distinct digests found for the ref, before any are dropped as
unreadable; `unreadable` is
[../config.md §6.3](../config.md#63-missing-and-foreign-files)'s own field.
There is no top-level `counts` object: that word already has a fixed,
different shape everywhere else it appears —
`{comments, errors, advisories, skipped}` on a `submit-review` result
([../cli.md §5.2](../cli.md#52-the-result-object)) — and reusing the name
here for `{submissions, comments}` would be exactly the kind of drift this
whole format exists to prevent. A caller that wants those two counts reads
`len(submissions)` and `len(comments)` directly.

`store.enabled`, read from the same config `submit-review` reads
([../config.md §3](../config.md#3-the-config-file)), is reported on the
envelope for the reason given in [§9](#9-empty-and-failure-cases): a fact
about the run's own condition, reported because a caller cannot otherwise
tell "empty because nobody has reviewed this yet" from "empty, and will
stay empty, because storing is off."

**What is not here, and why.** `path` — an absolute filesystem path — is
gone outright, and would be worth removing on its own even if nothing else
were: it leaks the store's own directory layout and the operator's home
directory name into an artifact an orchestrator may paste straight into a
PR description or a chat channel. `digest` and `at` are gone from both
`submissions[]` and `comments[]` for the same underlying reason: both are
facts about the *store*
([../config.md §4.4](../config.md#44-the-stored-files),
[config.md §4.5.1](../config.md#451-what-it-holds)), not about a finding,
and neither means anything to a reader who does not have the store behind
them. `at` remains a real mechanism —
[§5.3](#53-multiple-submissions-one-ref-enumeration-and-survival) still
uses it internally to order submissions and decide which supersedes
another — it simply stops being reported. `origin_id` is gone from
`comments[]` too, not because it stopped mattering but because it was
redundant with `id`: every qualified id is `<qualifier>:<origin_id>`
([§6.1](#61-the-qualified-id)), and `profile-format`
([§11.6](#116-profile-format-a-shape-check)) is what makes splitting on the
first colon a reliable way to recover it — carrying it a second time would
have been the same fact stated twice.

**`ordinal` is what replaces `digest` as the identity of a submission that
has no clean profile qualifier.** A submission with no `profile`, or one
that is no longer current for its profile
([§5.3.2](#532-marking-not-deleting)), still needs a handle
[§6.1](#61-the-qualified-id) can use to qualify its comments now that
`digest` is not in the output to serve as one. Every submission carries
`ordinal`, its 1-based position in the `submissions` array as ordered
below — always present, whether or not that particular submission ends up
needing it as a qualifier. `superseded_by`, when present, now names the
**ordinal** of the current submission for that profile, not a digest.

**`profile` on a comment answers who wrote it, not whether it still
stands, and it is present under both conditions.** Every comment whose
submission claimed a `profile` carries that field, current or superseded
— it is absent only when the submission genuinely claimed none. Whether a
comment's submission is current is a different question, already answered
twice over without touching this field: by the comment's own `id`, which
is ordinal-qualified rather than profile-qualified exactly when its
submission carries `superseded_by`, and by `superseded_by` itself, on the
matching entry in `submissions[]`. Omitting `profile` on a superseded
comment to signal supersession would collapse it onto the same shape as a
comment whose submission genuinely never claimed a profile — two different
facts rendered identically, which is precisely the failure this format
argues against everywhere else it comes up: "a run that verified nothing
must not look like a run that verified everything"
([../cli.md §2.3.1](../cli.md#231-verifying-anchors)), here about
attribution rather than verification. `profile` is the attribution
channel; the qualifier and `superseded_by` are the currency channel; they
do not share a field.

This is a materially weaker guarantee than a digest gave, and it has to be
said plainly rather than left implied. A digest is content-derived and
permanent — once assigned, nothing that happens later to the store changes
it. An `ordinal` is positional, and positional is only stable **for a given
set of submissions**. Against an unchanged store, repeated
`collect-reviews` calls return an identical array in an identical order, so
an `ordinal` — and every qualified id built from one — is stable for as
long as nothing new is submitted against the ref. The moment a new
submission arrives, that guarantee ends: a new profile name that happens to
sort earlier alphabetically than an existing one, or simply another
unprofiled submission, can shift the array position — and therefore the
`ordinal` — of a submission that was already there, changing that
submission's comments' qualified `id`s between one call and the next even
though nothing about the comments themselves changed. [§6.1](#61-the-qualified-id)
restates this at the point a caller would actually rely on an id staying
put.

```json
{
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "repo": { "name": "github.com/bobcob7/loam-refinery", "known": true },
  "store": { "enabled": true },
  "head_check": { "source": "repo", "is_head": true, "diverged": [] },
  "total": 2,
  "submissions": [
    {
      "ordinal": 1,
      "profile": "backend",
      "verdict": "request_changes",
      "summary": "…"
    }
  ],
  "comments": [
    {
      "id": "backend:dropped-context-1",
      "profile": "backend",
      "priority": 9,
      "category": "correctness",
      "body": "…",
      "anchors": [ /* … */ ],
      "suggestions": [ /* … */ ]
    }
  ]
}
```

`unreadable` is omitted from this sketch because it is 0 in every worked
example in [§12](#12-worked-example) — see [§9](#9-empty-and-failure-cases)
for when it appears and what it counts. `summary` sits beside `verdict` in
`submissions[]` deliberately, and is the one field this revision keeps that
an earlier list also proposed removing: of everything a submission could
carry at that level, `verdict` and `summary` are the only two that are the
reviewer's own prose and judgment rather than storage provenance, and both
stay for that reason — content, not metadata, which is exactly the
distinction the rest of this section draws for everything it removes.

**Ordering.** `submissions` is ordered by `profile` name, alphabetically,
for every submission that carries one — every submission sharing a profile
clusters together, oldest internally-first among them
([§5.3](#53-multiple-submissions-one-ref-enumeration-and-survival)) —
followed by every unprofiled submission, also oldest internally-first.
`ordinal` is that array's 1-based position. `comments` is ordered by its
own `id`, lexicographically — the same reasoning
[../config.md §6.1](../config.md#61-output) already gives for `reviews
--list`'s repositories: "ordered by name, so the output is stable between
runs," true here for as long as the store itself does not change between
two calls.

This is the JSON shape. [§8.3](#83-the-markdown-projection) is the only
other shape this command produces, and it is not a second structure —
[§8.3.1](#831-one-structure-two-renderers) is specific about why.

See [§12](#12-worked-example) for two full worked JSON versions and one
Markdown rendering of the first.

### 8.2 Can it be resubmitted

**No**, and this is not a gap to close later. `additionalProperties: false`
on every object in the review-document schema
([../review-document.md §3](../review-document.md#3-root-object)) means the
combined output — which has no `verdict` at the root, no single `summary`,
comments carrying `profile` (a root-only field on a real review document,
never a comment field), and a qualified `id` that does not even match the
comment-id pattern
([../review-document.md §4](../review-document.md#comment-ids)) once it
carries a colon — fails schema conformance immediately if it is fed back
into `submit-review`, exactly the way a hand-decorated document already
does. That is the correct outcome, not a defect: the combined output is a
**report**, in the same sense `reviews --content`'s output is a report — "a
document `submit-review` accepts" and "an object built to describe several
of them" are different kinds of thing, and `review-document.md §3`'s own
answer for a consumer that wants to carry both is the **wrapper**, never
decoration of the review itself: `{ "review": {...}, "pipeline": {...} }`.
An orchestrator that wants to turn a combined view into a *new* review — a
meta-review of what several reviewers found — writes that document itself,
by hand or by model, the same way any review gets written; it does not
round-trip the combined blob through `submit-review` and hope the shapes
happen to line up. This applies identically to the Markdown form
([§8.3](#83-the-markdown-projection)): it is farther still from a review
document than the JSON is, and nobody should try.

### 8.3 The markdown projection

`collect-reviews --format markdown` is the one exception, on this one
command, to
[cli.md §5.1](../cli.md#51-one-format)'s "everything a command has to say is
one JSON object on stdout... no second renderer and no `--format` choice
left to make." That section is one of the most strongly argued in this
project's documentation, and the exception has to answer its actual
argument, not talk past it.

`§5.1`'s objection is not aesthetic. Quoted in full because the reversal
has to survive exactly this: "two implementations of the same result
disagreed about what a run found three separate times, each disagreement
invisible to a green suite because no fixture pinned the shape that
differed; one of them let an author-supplied comment id forge diagnostic
lines that were never in the document." Two failure modes, named precisely:
**drift** (two renderers computing different answers about the same run,
undetected because nothing pinned them against each other) and **forgery**
(caller-supplied content being interpreted as tool-generated structure).
Markdown for `collect-reviews` has to answer both, and answering only one
would not be a reversal, it would be reintroducing the bug `§5.1` was
written to prevent.

#### 8.3.1 One structure, two renderers

**The rule that prevents drift:** Markdown is a **pure projection** of the
one result value `collect-reviews` already builds — the same Go value
[§8.1](#81-shape)'s JSON is `json.Marshal`ed from. One code path constructs
that value once, from the store; the JSON renderer serializes it, the
Markdown renderer formats it, and **neither computes anything the other
does not already have**. No re-sorting, no re-filtering, no re-deriving a
count, no branch in the Markdown path that asks the store a question the
JSON path did not already ask. `internal/render`
([../cli.md §7.1](../cli.md#71-package-layout), "the json renderer") gains
a second formatter beside the JSON one, taking the identical value the
first one takes — not a second implementation of `collect-reviews`'s own
logic wearing a different output format, which is exactly the shape of
thing `§5.1`'s own war story describes going wrong.

This is a stronger constraint than "render the same fields," and it needs
to be, because "render the same fields" is what a second, independently
reasoning implementation would also claim to do right up until the day it
did not. The constraint that actually holds is architectural: there must be
exactly one function that decides what `collect-reviews` found, and both
renderers are downstream of its return value, never of the store directly.

#### 8.3.2 Escaping and fencing caller-authored text

**The rule that prevents forgery:** every string in the combined output
originated from one of two places, and they get different treatment.

**Structurally-constrained fields** — `id`, `profile`, `ordinal`,
`verdict`, `category`, `effort`, `scope` — are already safe by
construction. `id` is always `<qualifier>:<origin_id>`
([§6.1](#61-the-qualified-id)), and both halves are individually
constrained: the qualifier is either a `profile-format`-shaped name
([§11.6](#116-profile-format-a-shape-check)) or `#` followed by a plain
integer `ordinal`, and `origin_id` matches
`^[a-z][a-z0-9]*(-[a-z0-9]+)*-[1-9][0-9]*$`
([../review-document.md §4](../review-document.md#comment-ids)); enums are
closed sets; `ordinal` is a JSON integer. None of these character sets
contain a markdown special character, so none need escaping to appear as
heading text or table cells — and `#`, the one non-alphanumeric character
`id` can carry, is not even a valid ATX heading marker mid-string, since
CommonMark requires a space after a heading `#` and `#3` has none. This is
also why the *historical* bug `§5.1` describes — "an author-supplied
comment id forge diagnostic lines" — cannot recur through `id` today the
way it evidently once could: the id grammar is already locked down
structurally, independent of this feature. What is not locked down is
everything below.

**Free-text prose fields** — `body`, `summary`, suggestion `summary`,
`pros`, `cons` — have no character restriction beyond length. These are
**backslash-escaped**, one character at a time, using the full set
CommonMark defines as escapable ASCII punctuation
(`` ! " # $ % & ' ( ) * + , - . / : ; < = > ? @ [ \ ] ^ _ ` { | } ~ ``) —
every character in that set, not a hand-picked subset that risks missing
one that matters. A body containing `# Not a real heading` renders as the
literal text `\# Not a real heading`, displaying as `# Not a real heading`
with no heading created; a body containing a bare `` ``` `` cannot open or
close a fence it is not inside, because the backtick is escaped. This keeps
these fields reading as ordinary prose — the whole reason a human-facing
Markdown format is worth having — while making it structurally impossible
for their content to be interpreted as anything but text.

**Verbatim fields** — `anchor.file` and the two `code` fields — are neither
escaped nor left raw; they are wrapped so their content displays exactly as
written instead. `anchor.file` is not shape-constrained the way `id` is (a
real path can contain underscores, brackets, anything POSIX allows short of
a leading `/` or a `..` segment), so it is set in an **inline code span**:
one or more backticks, chosen one longer than the longest backtick run
already inside the path (trivial in practice, but the rule has to be
general to be a rule). `comment.code` and `suggestion.code` are multi-line
and get a **fenced code block** by the same principle at block scope: the
fence is a run of backticks one character longer than the longest run
already present in the content, minimum three — the standard technique for
guaranteeing a fence can never be closed early by content that happens to
contain backticks of its own. Escaping is deliberately **not** applied to
these two categories: fencing already suppresses markdown interpretation of
everything inside it, and escaping on top would corrupt content meant to
display byte-for-byte — an escaped underscore in a variable name would show
a stray backslash the reviewer never wrote.

[§12.3](#123-the-markdown-projection-and-what-escaping-prevents) shows this
concretely, including a body engineered to attempt exactly the forgery
`§5.1` warns about.

#### 8.3.3 The test that pins it

Three tests, in the spirit of "Golden files cover the renderer"
([../cli.md §7.4](../cli.md#74-testing)):

1. **Parity.** Build one fixture value with several submissions and
   comments. Render it through both formatters. Parse the qualified-id
   headings out of the Markdown output (safe to do exactly because ids are
   structurally constrained, per [§8.3.2](#832-escaping-and-fencing-caller-authored-text))
   and assert that set equals the set of `comments[].id` in the JSON —
   same members, same count, same order. This is the test that would have
   caught `§5.1`'s own war story: two renderers disagreeing about what a run
   found becomes a failing assertion instead of an invisible drift, because
   both are asserted against the one value that produced them, not against
   each other's output.
2. **Fidelity.** For each comment, unescape the Markdown body using the
   inverse of [§8.3.2](#832-escaping-and-fencing-caller-authored-text)'s
   rule and assert the result equals the JSON `body` string byte for byte.
   Proves escaping changed encoding, never content.
3. **Forgery.** A second fixture where one comment's `body` contains a
   literal `# FORGED FINDING` and another's `code` contains an embedded run
   of backticks. Render to Markdown and assert: no line in the output is an
   actual level-1 heading reading `FORGED FINDING` — the text appears only
   as escaped, inline prose inside the real comment's body — and the fence
   chosen for the backtick-containing code excerpt is strictly longer than
   every backtick run inside it, so the block closes where it is supposed
   to and not inside the attacker's content.

All three run against the same fixture data the JSON renderer's own golden
tests already use, not a Markdown-specific corpus — another way the "one
structure" constraint in
[§8.3.1](#831-one-structure-two-renderers) stays enforced rather than
merely asserted.

**One more boundary, stated because §5.1's whole argument is about exactly
this:** Markdown is for a human, or for pass-through embedding somewhere a
human will read it — a PR comment body, a chat message. It is **not** a
second machine-interchange format. Nothing downstream should ever parse
`collect-reviews --format markdown`'s output back into structured data;
anything automated that wants `collect-reviews`'s findings uses
`--format json`, unconditionally. That restriction is what keeps this
exception narrow: the moment something starts scraping Markdown headings to
reconstruct comments, this project has quietly grown the second
implementation `§5.1` was written against, just one layer further out.

## 9. Empty and failure cases

The precedent throughout is
[../config.md §6.2](../config.md#62-empty-answers)'s table for `reviews`,
reused directly rather than reinvented, because `collect-reviews` reads the
identical store and an inconsistent answer to "nothing here" between the two
commands would be its own bug:

| Case | Result | Exit |
| --- | --- | --- |
| Repository known, ref has stored submissions | combined output | 0 |
| Repository known, ref has none | `known: true`, empty `submissions`/`comments`, `total: 0` | 0 |
| Repository not in the store | `known: false`, empty `submissions`/`comments`, `total: 0` | 0 |
| Store does not exist at all | `known: false`, empty `submissions`/`comments`, `total: 0` | 0 |
| `--ref` missing, or not 40 lowercase hex | usage error | 2 |
| Malformed `--repo` | usage error | 2 |
| Database exists but cannot be opened or read | tool error on stderr | 101 |

A `--ref` that is well-formed but does not resolve to a real commit is not a
distinct case above, on purpose: `collect-reviews`, like `reviews`, "resolves
nothing, fetches nothing"
([../config.md §6](../config.md#6-reading-the-store)) — it is not this
command's job to know whether a SHA is real, only whether the store holds
anything filed under it, and a SHA nobody ever submitted against looks
identical to a real one nobody reviewed yet: zero rows, `known` reflecting
the *repository*, not the ref.

**A stored file that cannot be opened is not one of the empty rows above.**
`collect-reviews` opens every distinct digest's file to read its comments —
unlike `reviews`'s default index, which answers entirely from the database
([../config.md §6](../config.md#6-reading-the-store)) — so it inherits the
content-reading contract [../config.md §6.3](../config.md#63-missing-and-foreign-files)
already specifies for `reviews --content`: a deleted or corrupted file is
**skipped and counted**, not fatal, in `unreadable` on the envelope. Unlike
`reviews`, where `unreadable` appears only when `--content` was asked for,
`collect-reviews` always reads content, so `unreadable` is always present
here, `0` when nothing was missing.

**`store.enabled: false` does not gate this command, and reporting it is
not the same thing as gating on it.** The flag
([../config.md §3](../config.md#3-the-config-file)) turns off *writing* —
"the only way to turn storing off" — and nothing in
[../config.md §6](../config.md#6-reading-the-store) makes reading depend on
it; a store that accumulated reviews before the flag was set stays fully
readable by `reviews` after it is, and the same holds for `collect-reviews`.
So the empty answer a machine with storing off produces is not a special
case in the table above — it is the same "store does not exist" or "ref has
none" row any other empty store produces.

What *should* change is what the caller can tell from the output, and
[§8.1](#81-shape)'s `store.enabled` field is that fix — the precedent is
`verification.source`, which is not a gate on whether `submit-review` runs
either, only a fact the result object reports about the run's own
condition. [§11.7](#117-a-note-on-storeenabled-and-configmd-52) records the
operational consequence this creates for
[../config.md §5.2](../config.md#52-turning-it-off)'s read-only-image
option.

## 10. Budget

Combined output has no fixed size — it grows with the number of surviving
submissions, the number of comments each carries, and, since
[§5.3.2](#532-marking-not-deleting) keeps every submission rather than
deleting superseded ones, with however many times each reviewer has ever
resubmitted against this ref. The honest form to give it is `reviews`'s
marginal-cost shape — an envelope plus a per-row cost — with the actual
numbers left unmeasured, because inventing them ahead of an implementation
to measure against is exactly the mistake
[../cli.md §6.1](../cli.md#61-budgets) already warns against: "a limit
nothing measures is a limit that erodes." (`prime`, `describe`, `schema`,
and `reviews` all carry real measured figures against their ceilings today;
`collect-reviews` does not exist yet to measure, and this table says so
rather than filling the gap with a plausible-looking number.)

| Call | Budget | Frequency |
| --- | --- | --- |
| `collect-reviews --format json` | *not yet measured* — envelope + *N* per submission + *M* per comment + *D* per diverged anchor | Rare; only when an orchestrator has run more than one reviewer against one ref |
| `collect-reviews --format markdown` | unbudgeted | Rare; human reading, or embedding somewhere a human reads it — see the boundary in [§8.3.3](#833-the-test-that-pins-it) |

Shape, reasoned from what the object actually carries, in four parts for
the JSON form:

- **Envelope.** `ref`, `repo`, `store`, `head_check.source`, `head_check.is_head`,
  `total`, `unreadable` — small, fixed-size fields, structurally similar to
  `verification` (minus `unverified`) and `skipped` together on the
  `submit-review` result object ([../cli.md §5.2](../cli.md#52-the-result-object)),
  paid once per call.
- **Per submission.** Substantially smaller than the previous revision of
  this table estimated, now that `path` (an absolute filesystem path),
  `digest` (64 hex characters), and `at` are gone from it
  ([§8.1](#81-shape)) — the comparison to a `reviews` row paying "for
  identity twice" no longer applies, because this envelope no longer pays
  for identity by string at all. What remains is a `profile` name (or its
  absence), a small integer `ordinal`, an optional integer
  `superseded_by`, `verdict`, and unbudgeted `summary` content — closer in
  shape to a handful of small fields than to a row that has to reproduce a
  path and a hash.
- **Per comment.** Closer to `reviews --content`'s territory than to a
  `submit-review` diagnostic: it carries a full body, full suggestions, and
  is explicitly **unbudgeted** for the same reason `reviews --content` is —
  "it returns [content] the caller wrote, at whatever size the caller wrote
  them" ([../config.md §6.1](../config.md#61-output)). What *is* budgetable
  is the envelope around each comment — now just `id` and `profile`
  ([§8.1](#81-shape) removed `origin_id` and `digest` from this list; `id`
  already carries the origin id as a substring) — on top of a comment's own
  content, smaller than the previous revision's four-field estimate.
- **Per diverged anchor.** `head_check.diverged` is a per-anchor list of
  exactly the shape `verification.unverified` already is, and
  [../cli.md §6.1](../cli.md#61-budgets) has already measured that
  identical check at "about 50 tokens per anchor... the shape is the same
  as a diagnostic." `head_check`'s cost is fixed only when nothing has
  diverged; the moment more than one anchor has, it scales the same way
  `verification.unverified` already does.

The Markdown form is unbudgeted outright rather than priced with a shape,
for a reason `--content`-style unbudgeted rows do not have to argue but this
one does: it is never on the loop a token ceiling exists to protect. Every
other row in every budget table in this project's docs is priced because an
agent might pay it repeatedly, in a session, against a ceiling that
compounds. `--format markdown` is read by a human, once, or piped into a
comment body a human will read once — it is closer in spirit to `schema
--annotated`'s "rare, machine-facing calls that no loop pays"
([../cli.md §6.1](../cli.md#61-budgets)) than to anything paid per
iteration, just facing a person instead of a codegen tool.

`--limit` is not offered here ([§2.2](#22-flags)) the way it is on
`reviews`, because there is no ordering to limit by that would not silently
drop one reviewer's findings from a call whose whole purpose is not doing
that — this is recorded as a real cost of the design, not an oversight, and
is why the JSON row above has no ceiling to give, only a shape.

## 11. Amendments

Design principles are revised in the open here, the same posture
[../config.md §1.1](../config.md#11-what-this-changes-about-the-design-principles)
already uses for its own promises and [../cli.md §1](../cli.md#1-overview)
uses for verification's working-tree read.

### 11.1 The out-of-scope bullet

[../cli.md §1](../cli.md#1-overview) currently reads, under "Explicitly out
of scope":

> Aggregating reviews. Passing reviews can be kept ([config.md](../config.md)),
> but they are kept side by side — never merged, ranked, reconciled, or
> summarized across runs.

**Amended, narrowly.** `collect-reviews` combines the stored reviews for one
ref into one attributed output. What survives, stated as precisely as the
amendment this bullet already models for reading file contents: reviews are
never fused into fewer findings across profiles
([§5.2](#52-grouping-by-profile-not-by-slug)), never ranked against each
other, and never reduced to a single reconciled verdict
([§5.4](#54-verdict-reported-never-reconciled)). What changes is narrower
than the bullet's own words suggest: within one claimed profile, a later
submission is marked current relative to an earlier one
([§5.3.2](#532-marking-not-deleting)), without deleting the earlier one's
findings. And aggregation across **refs** remains entirely out of scope;
`collect-reviews` combines within one ref and refuses to run without one
([§2](#2-the-command)).

### 11.2 The standing "no aggregation" bullet

Separately from the out-of-scope list, [../config.md §1](../config.md#1-what-this-adds)
carries its own commitment, in a list that document itself calls "the
boundary that keeps this addition from turning the tool into something
else":

> No aggregation. Reviews are stored side by side, not merged, ranked, or
> reconciled. Merging is still [cli.md §8](cli.md#8-future-considerations).

**Amended, the same way as [§11.1](#111-the-out-of-scope-bullet).** The
store itself is unchanged by this feature — reviews are still written side
by side. What changes is that a second command now reads several of them
back as one attributed view, under the same narrow scope
[§11.1](#111-the-out-of-scope-bullet) states. The bullet's own final
sentence should now point at this document instead of at the
future-considerations entry [§11.4](#114-the-review-merging-future-consideration)
retires.

### 11.3 The amendment table row

[../config.md §1.1](../config.md#11-what-this-changes-about-the-design-principles)
currently carries this row:

> | Aggregating across runs is out of scope | Unchanged. |

**Amended.** The row should read:

| Principle | Standing |
| --- | --- |
| Aggregating across runs is out of scope | **Amended, narrowly.** `loam-refinery collect-reviews` combines the reviews stored for one ref into one attributed output — see [docs/features/combined-reviews.md](../features/combined-reviews.md). Findings are combined, never fused across profiles, ranked, or reduced to one verdict; a later submission under the same claimed profile is marked current relative to an earlier one, without deleting the earlier one's findings. Aggregation across *refs* is unchanged and still out of scope. |

### 11.4 The review-merging future consideration

[../cli.md §8](../cli.md#8-future-considerations) currently carries:

> **Review merging.** A `loam-refinery merge` subcommand combining several
> subagent reviews into one. Comment IDs and slug grouping are what make
> this tractable: merge by slug, keep the highest priority per group, and
> union the suggestions. Suffixes are renumbered on merge, so IDs are
> stable within a document but not across them.

**Retired, in favor of this document.** Left standing, this bullet would
have `cli.md` specifying a `merge` command with a slug-fusion mechanism this
document declines to build ([§5.1](#51-why-not-by-slug),
[§2.1](#21-rejected-names)), alongside a real, differently-named command
that does something related but more specific. Replace with: "**Combined
reviews.** Specified in
[docs/features/combined-reviews.md](../features/combined-reviews.md):
`collect-reviews`, not `merge`; grouped by profile, not by slug."

### 11.5 The root object table and the schema

Until `profile` is a recognized field,
[../review-document.md §3](../review-document.md#3-root-object)'s own
`additionalProperties: false` means `submit-review` **rejects** the
documents `collect-reviews` is specified to read. That table's five fields
— `version`, `verdict`, `summary`, `ref`, `comments` — all required.
**Amended** to add a sixth, optional, row:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `profile` | string | no | The reviewer profile ([cli.md §2.1.1](../cli.md#211-reviewer-profiles)) this review was written under, if any — the same `NAME` as `prime --profile=NAME`. Caller-authored and unverifiable, like `id`. Shape constrained by `profile-format` ([§11.6](#116-profile-format-a-shape-check)); honesty is not. Read by `loam-refinery collect-reviews` for attribution. |

`internal/schema/review.schema.json` needs the matching property, added to
`properties` and left out of `required`:

```json
"profile": {
  "title": "Reviewer profile",
  "description": "The name of the reviewer profile (cli.md §2.1.1) this review was written under, if any — the same NAME passed to `prime --profile=NAME`. Optional: omitted when the reviewer ran unprimed. Caller-authored and unverifiable, like id.",
  "$comment": "related: profile-format",
  "type": "string",
  "pattern": "^[a-z0-9]+(-[a-z0-9]+)*$",
  "examples": ["backend"]
}
```

[§7](#7-the-profile-field) states the rollout constraint this amendment
implies.

### 11.6 `profile-format`, a shape check

A companion amendment to [§11.5](#115-the-root-object-table-and-the-schema).
[../review-document.md §11.1](../review-document.md#111-structural-checks--hard)'s
structural checks table gains a row, directly beside `ref-format`:

| Check | Rule |
| --- | --- |
| `profile-format` | `profile`, where present, matches `^[a-z0-9]+(-[a-z0-9]+)*$` — the same grammar [cli.md §2.1.2](../cli.md#212-the-profile-file) already defines for the filename a profile resolves to. Checkable without a repository, exactly like `ref-format`. |

This costs [§7.2](#72-caller-authored-and-unverifiable) nothing: it
constrains shape, not honesty. It exists because
[§6.1](#61-the-qualified-id)'s qualified-id round-trip claim depends on
`profile` never containing a colon.

### 11.7 A note on `store.enabled` and config.md §5.2

[../config.md §5.2](../config.md#52-turning-it-off) currently presents
`{"version":"1","store":{"enabled":false}}` as one of two supported ways to
run in an environment where `submit-review` must not write, with no
qualification. **Amended, by addition rather than contradiction:** a note
that an operator choosing this option, once `collect-reviews` exists, gets
no output from it — ever, for that machine — and that `collect-reviews`
reports `store.enabled` on its own envelope
([§8.1](#81-shape)) precisely so a caller in that position can tell "off"
from "nobody has reviewed this yet." An operator who needs both — a
read-only image *and* `collect-reviews` reading something back — has to use
[../config.md §5.2](../config.md#52-turning-it-off)'s other option instead,
`LOAM_REFINERY_HOME` pointed at somewhere writable.

### 11.8 cli.md §2, §2.3, and the flag table

Beyond the mechanical renames [§3.1](#31-what-changes-the-name) lists,
three passages change **meaning**, not just spelling, and belong here rather
than in that inventory:

- [../cli.md §2.3](../cli.md#23-validate)'s opening sentence, "Runs the
  structural checks, then verification **if a source is supplied**, then
  the advisories," describes verification as conditional. **Amended**: under
  [§3.4](#34-verification-is-required-to-submit), a missing or unreachable
  source is no longer tolerated — it is a failing submission for any
  document with anchors.
- [../cli.md §2.3.1](../cli.md#231-verifying-anchors)'s paragraph beginning
  "A caller who cannot accept an unchecked document passes
  `--require-verification`..." and the paragraph after it describing
  `--warn-only=ref-unknown`, `--warn-only=anchor-worktree-diverged`, and
  "A caller that wants the other three non-fatal demotes them explicitly."
  **Removed outright**, per [§3.3](#33-no-disable-no-warn-only) and
  [§3.4](#34-verification-is-required-to-submit) — there is no flag left for
  either paragraph to describe. [§3.4.4](#344-the-unreachable-ref-consequence)
  is what a reader loses by their removal and should probably be folded into
  the surrounding prose in their place.
- [../cli.md §3](../cli.md#3-flags)'s flag table: the `--warn-only`,
  `--require-verification`, and `--disable` rows are deleted, not renamed.
  The shared `--format json` row's `validate` becomes `submit-review`.
  `--strict`'s row is undecided — [§13](#13-open-questions).
- [../cli.md §4](../cli.md#4-exit-codes)'s exit-1 row, "unverified anchors
  under `--require-verification`," needs rewording to describe the new
  unconditional behavior rather than a flag that no longer exists.

### 11.9 config.md §3.1's flag-only-keys sentence

[../config.md §3.1](../config.md#31-what-configuration-may-not-do) currently
reads:

> **No key may change whether a document is valid.** `strict`, `disable`,
> `warn_only`, and `require_verification` are flags and only flags. They are
> named explicitly in the rejected-key message rather than being reported
> as unknown, so the caller learns where they went.

**Amended.** Three of those four names no longer correspond to a flag at
all — [§3.3](#33-no-disable-no-warn-only) and
[§3.4](#34-verification-is-required-to-submit) remove `disable`,
`warn_only`, and `require_verification` outright, and `strict`'s survival is
undecided ([§13](#13-open-questions)). The sentence's own claim — that these
are "flags and only flags," named specially so a caller learns where they
went — stops being accurate the moment three of the four things it points at
cease to exist anywhere to be pointed at. The rejected-key message this
sentence describes would, unedited, tell a caller that `disable` "is a flag,
not a config key," which is no longer true — it is not a flag either.
Replace with: "`strict` is a flag and only a flag [if it survives]. A key
named `disable`, `warn_only`, or `require_verification` is simply unknown —
those are not flags a caller mistook for config keys, they are names this
tool used to accept and no longer does anywhere." This is a real edit the
edit bead owes, not a rewording exercise: it changes what the config-file
error message says, not just what this document says about it.

### 11.10 cli.md §5.1, the markdown exception

[../cli.md §5.1](../cli.md#51-one-format) currently reads:

> Everything a command has to say is one JSON object on **stdout**. There is
> no second renderer and no `--format` choice left to make: two
> implementations of the same result disagreed about what a run found three
> separate times, and one of them let an author-supplied comment id forge
> diagnostic lines the document never contained. A caller parses the object;
> nothing has to parse prose.
>
> `--format=json` is still accepted so callers already passing it keep
> working. `--format=text` is an error that says where the format went.
>
> The prose that remains is prose because it is written to be read, not
> rendered: `prime`, and the `summary` field of `describe`.

**Amended, narrowly, using the same table pattern
[../config.md §1.1](../config.md#11-what-this-changes-about-the-design-principles)
already uses:**

| Principle | Standing |
| --- | --- |
| No second renderer, no `--format` choice left to make | **Amended, narrowly.** `loam-refinery collect-reviews --format markdown` is the one exception, on the one command whose primary audience is a human reader rather than an agent in a loop — see [docs/features/combined-reviews.md §8.3](../features/combined-reviews.md#83-the-markdown-projection). It is not a second *renderer* in the sense this section warns against: it is a pure projection of the identical result value the JSON form serializes, built once, by one code path, with the same escaping and fencing discipline specified there to close the forgery half of this section's own argument. `submit-review`, `reviews`, `describe`, and `schema` are entirely unchanged — one command, one projection, one source of truth, not a second computation of any result. |

### 11.11 On "once" — checked, no amendment needed

An earlier revision of this document considered amending the row in
[../cli.md §1](../cli.md#1-overview)'s own amendment table reading "the
working tree is consulted **once**... to decide whether an anchor is
checkable at all," on the reasoning that
[§4.3](#43-the-head-check) adds a second caller of the same check and
"once" would then overstate it. That does not hold up:
[../cli.md §6.1](../cli.md#61-budgets) already glosses that same word
precisely — "once per anchored file when `ref` is `HEAD`" — a **per-call,
per-file** reading, not a claim that the tree is consulted once in the
tool's entire lifetime. No edit to `cli.md §1` is proposed here. Left in
rather than deleted, because finding a problem and then finding, on closer
reading, that it is not one, is worth being visible about.

### 11.12 The seven design principles, checked

| # | Principle (as stated in `cli.md §1`) | Standing |
| --- | --- | --- |
| 1 | Offline, read-only about the repository | **Extended, not newly amended.** `collect-reviews`'s head check reads the working tree the same way `submit-review`'s verification already does, for the identical purpose, at a second call site. See [§11.11](#1111-on-once--checked-no-amendment-needed). |
| 2 | The schema is the documentation | **Genuine gap, not a falsification.** Nothing explains the combined output's shape the way `describe --lens` explains every review-document field. A `topic:collect-reviews` entry through the existing topics provider ([../cli.md §2.2.3](../cli.md#223-the-entry-registry)) is the natural fix, recorded as follow-up work, not decided here. |
| 3 | Pay for detail on demand — "No command emits everything it knows" | **Amended.** `collect-reviews` emits every comment of every surviving submission in full, by default, with no index-only rung and no `--limit` ([§10](#10-budget) concedes and defends this). The assembled findings are the entire value this command exists to provide. |
| 4 | Errors route to their own explanation | **Consistent**, and simpler than before: [§3.3](#33-no-disable-no-warn-only) and [§3.4](#34-verification-is-required-to-submit) remove flags, not check names — every diagnostic a caller can still hit had an explanation before this revision and still does. |
| 5 | Everything is addressable — "Every comment carries an ID the reviewer chose" | **Amended.** Under [§6.1](#61-the-qualified-id), the `id` in the combined output is composed by the tool. What survives is the origin id, embedded unchanged as the substring after the first colon rather than carried as its own field, specifically so the reviewer's own chosen id stays recoverable by a fixed rule ([§6.2](#62-routing-feedback-back-to-a-reviewer)). |
| 6 | Advise, don't police | **Consistent.** Verification was never on the quality side of the boundary this principle draws, and this is not a close call once [../review-document.md §11](../review-document.md#11-validity)'s own taxonomy is the yardstick rather than an inference from this principle's two words: structural and verification checks are both "hard," both answering whether a document is usable at all, categorically separate from advisory's "is this a good review?" — a hallucinated anchor sits in the same tier as malformed JSON, not beside `body-thin` or `priority-flat`. [§3.4](#34-verification-is-required-to-submit) makes that tier's existing standard unconditional; it does not move verification into a tier it was never in. What changed is a caller's ability to accept a document nobody could confirm at all — an axis this principle was never about, since "good" never enters what verification checks — and [§5.4](#54-verdict-reported-never-reconciled)'s refusal to synthesize a verdict, the part of this principle that actually touches quality judgment, is untouched by any of this. |
| 7 | Cheap to call | **Amended**, for two independent reasons now. `store.enabled` decides whether a multi-reviewer workflow built on `collect-reviews` produces any output at all ([§9](#9-empty-and-failure-cases), [§11.7](#117-a-note-on-storeenabled-and-configmd-52)) — a real behavioral stake the flag did not carry before this feature existed, even though "configuration may not change whether a document is valid" remains word-for-word true. Separately, [§3.4](#34-verification-is-required-to-submit) means a caller cannot always get a document accepted just by running `submit-review` the way it always could before — a repository state now matters to whether the call succeeds, not only to how thoroughly it is checked. |

## 12. Worked example

Two examples exercising the merge semantics, one exercising the markdown
projection.

### 12.1 Two profiles, one ref

**Submission A**, stored under `profile: "backend"`:

```json
{
  "version": "1",
  "verdict": "request_changes",
  "profile": "backend",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "The retry loop is sound, but the context deadline is not propagated to the downstream call.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 9,
      "category": "correctness",
      "body": "The retry loop calls c.do(context.Background(), req) rather than passing the caller's ctx.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 88, "end_line": 94 }
      ],
      "suggestions": [
        {
          "summary": "Pass the caller's context straight through to c.do",
          "effort": "trivial",
          "scope": "line",
          "pros": ["Cancellation and deadlines propagate immediately"],
          "cons": ["A caller relying on retries outliving the request context sees a behavior change"]
        }
      ]
    }
  ]
}
```

**Submission B**, stored under `profile: "security"`, submitted about
eleven minutes later, same `ref`:

```json
{
  "version": "1",
  "verdict": "comment",
  "profile": "security",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "One low-severity logging concern; nothing blocking.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 3,
      "category": "security",
      "body": "The retry loop's debug log includes req.Header verbatim, which can carry an Authorization value on a retried request.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 82 }
      ],
      "suggestions": [
        {
          "summary": "Redact known-sensitive headers before logging the request",
          "effort": "small",
          "scope": "file",
          "pros": ["Removes the leak at the one place it can happen"],
          "cons": ["A future header added to the allowlist could reopen this silently"]
        }
      ]
    }
  ]
}
```

Both submissions reached the store only because they verified cleanly
against `ref` at submission time ([§3.4](#34-verification-is-required-to-submit)).
`internal/fetch/client.go` has not changed on disk since either ran, so the
head check at collection time finds nothing drifted.
`loam-refinery collect-reviews --ref=4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d`
returns:

```json
{
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "repo": { "name": "github.com/bobcob7/loam-refinery", "known": true },
  "store": { "enabled": true },
  "head_check": { "source": "repo", "is_head": true, "diverged": [] },
  "total": 2,
  "unreadable": 0,
  "submissions": [
    {
      "ordinal": 1,
      "profile": "backend",
      "verdict": "request_changes",
      "summary": "The retry loop is sound, but the context deadline is not propagated to the downstream call."
    },
    {
      "ordinal": 2,
      "profile": "security",
      "verdict": "comment",
      "summary": "One low-severity logging concern; nothing blocking."
    }
  ],
  "comments": [
    {
      "id": "backend:dropped-context-1",
      "profile": "backend",
      "priority": 9,
      "category": "correctness",
      "body": "The retry loop calls c.do(context.Background(), req) rather than passing the caller's ctx.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 88, "end_line": 94 }
      ],
      "suggestions": [
        {
          "summary": "Pass the caller's context straight through to c.do",
          "effort": "trivial",
          "scope": "line",
          "pros": ["Cancellation and deadlines propagate immediately"],
          "cons": ["A caller relying on retries outliving the request context sees a behavior change"]
        }
      ]
    },
    {
      "id": "security:dropped-context-1",
      "profile": "security",
      "priority": 3,
      "category": "security",
      "body": "The retry loop's debug log includes req.Header verbatim, which can carry an Authorization value on a retried request.",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 82 }
      ],
      "suggestions": [
        {
          "summary": "Redact known-sensitive headers before logging the request",
          "effort": "small",
          "scope": "file",
          "pros": ["Removes the leak at the one place it can happen"],
          "cons": ["A future header added to the allowlist could reopen this silently"]
        }
      ]
    }
  ]
}
```

Note what did **not** happen: `dropped-context-1` was never a group. Both
comments survive, distinctly, under `backend:dropped-context-1` and
`security:dropped-context-1` — both profile-qualified, because both
submissions are current for their profile, so `ordinal` never surfaces in
either comment's `id` here even though it is present on both entries in
`submissions[]`. Nothing in this response names a file on disk, a hash, or
a timestamp: an orchestrator could paste the whole object into a PR comment
without leaking anything about where or how it was stored. `head_check`
here means something sharper than it did before this revision: `diverged:
[]` says every anchor in this output not only holds against the tree right
now, it was **confirmed once already, at submission**, and nothing has
moved since — a stronger claim than "nothing currently contradicts it,"
now that [§3.4](#34-verification-is-required-to-submit) guarantees every
stored anchor was checked at least once.

### 12.2 One profile, revised, plus one unprofiled submission

The same ref, three stored digests, referred to here by an internal label
that never appears in the output: `backend` reviewed once, revised its own
finding and added a second, and an unprimed reviewer with no `profile`
also ran.

- `D1`, `profile: "backend"`, stored earliest — one comment,
  `stale-cache-1`, priority 6.
- `D2`, `profile: "backend"`, stored second — a revision: `stale-cache-1`
  again (id kept, body sharpened, priority raised to 8), plus a new
  comment, `missing-invalidation-1`.
- `D3`, no `profile`, stored latest — one comment, `todo-left-in-1`.

`D1` and `D2` share a claimed profile; `D2` is later, so `D1` carries
`superseded_by` and `D2` does not. Neither is dropped — `D1`'s
`stale-cache-1` still appears, under an ordinal-qualified id because its
submission is no longer current for `backend`; `D2`'s comments use the
plain `backend:` qualifier. `D3` never qualifies for supersession — no
`profile` means no identity axis to supersede on — so it survives
unconditionally, also ordinal-qualified, since it has no profile to be
qualified by in the first place. Ordinals are assigned by
[§8.1](#81-shape)'s ordering: `backend`'s two submissions cluster first,
oldest first, so `D1` gets `ordinal: 1` and `D2` gets `ordinal: 2`; the
unprofiled `D3` follows, `ordinal: 3`:

```json
{
  "ref": "9c1a2e4f6b8d0c3a5e7f9b1d3c5a7e9f1b3d5c7a",
  "repo": { "name": "github.com/bobcob7/loam-refinery", "known": true },
  "store": { "enabled": true },
  "head_check": { "source": "repo", "is_head": false },
  "total": 3,
  "unreadable": 0,
  "submissions": [
    {
      "ordinal": 1,
      "profile": "backend",
      "verdict": "request_changes",
      "summary": "Cache invalidation is missing on the write path.",
      "superseded_by": 2
    },
    {
      "ordinal": 2,
      "profile": "backend",
      "verdict": "request_changes",
      "summary": "Cache invalidation is missing on two write paths, not one."
    },
    {
      "ordinal": 3,
      "verdict": "comment",
      "summary": "One stray TODO; nothing blocking."
    }
  ],
  "comments": [
    {
      "id": "#1:stale-cache-1",
      "profile": "backend",
      "priority": 6,
      "category": "correctness",
      "body": "The write path updates the DB but never invalidates the cache entry.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 41 }],
      "suggestions": [ /* … */ ]
    },
    {
      "id": "backend:stale-cache-1",
      "profile": "backend",
      "priority": 8,
      "category": "correctness",
      "body": "The write path updates the DB but never invalidates the cache entry; on closer read, the batch-update path has the same gap.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 41 }],
      "suggestions": [ /* … */ ]
    },
    {
      "id": "backend:missing-invalidation-1",
      "profile": "backend",
      "priority": 8,
      "category": "correctness",
      "body": "Same gap as stale-cache-1, on the batch-update path.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 96 }],
      "suggestions": [ /* … */ ]
    },
    {
      "id": "#3:todo-left-in-1",
      "priority": 2,
      "category": "style",
      "body": "A TODO from the previous change is still here and looks resolved by this one.",
      "anchors": [{ "file": "internal/cache/store.go", "line": 12 }],
      "suggestions": [ /* … */ ]
    }
  ]
}
```

Note that `D1`'s comment carries `profile: "backend"` even though its
submission is superseded — `#1:stale-cache-1` is both ordinal-qualified
*and* attributed. The two facts are independent: `profile` says a
`backend` reviewer wrote this finding, full stop, and stays true regardless
of what later happens to that submission's currency. Whether it still
speaks for `backend` is answered elsewhere, without an orchestrator having
to join anything: the qualifier itself, `#1:` rather than `backend:`, and
`submissions[0].superseded_by`, which names `2` — `D2`'s `ordinal` — as the
entry that does. Compare `#3:todo-left-in-1`, which carries no `profile` at
all: that comment's submission genuinely never claimed one, a different
fact from `D1`'s superseded-but-attributed one, and the two must not look
the same. Omitting `profile` from `D1`'s comment to signal supersession —
the earlier draft of this example did exactly that — would have erased
that distinction and forced the orchestrator to cross-reference `ordinal`
against `submissions[]` just to recover attribution the field could have
carried directly, in an output whose whole premise is that no such
cross-reference should be necessary.

`stale-cache-1` appears **twice**, under two different qualified ids —
exactly the collision [§6.1](#61-the-qualified-id) has to prevent once
superseded submissions are no longer deleted. This ref is not `HEAD`, so
`diverged` is correctly **absent**, not an empty array. Had `D1` and `D2`
shared the identical internal timestamp, the tie-break
([§5.3.3](#533-the-tie-break-is-now-cosmetic)) would have decided which one
carries `superseded_by` — cosmetic only, since both are fully present
either way. Had a fourth submission arrived under a profile name sorting
before `"backend"` — `"api"`, say — every ordinal in this example would
shift by one and `#1:stale-cache-1` would become `#2:stale-cache-1` on the
next call: the instability [§8.1](#81-shape) states plainly, made
concrete.

### 12.3 The markdown projection, and what escaping prevents

A minimal fixture — one submission, one profile, one comment whose body
attempts exactly the forgery [cli.md §5.1](../cli.md#51-one-format) warns
about: a fake heading and a fence-breaking sequence, both supplied by the
reviewer, neither generated by the tool.

Submitted document (excerpt):

```json
{
  "id": "injected-heading-1",
  "priority": 4,
  "category": "style",
  "body": "Minor: the comment above this block says # SECURITY: bypass all checks below, which reads like a directive but is dead code the linter already flags.",
  "anchors": [{ "file": "internal/legacy/parse.go", "line": 12 }],
  "code": "// # SECURITY: bypass all checks below\n// ```\nif true {\n```",
  "suggestions": []
}
```

The `body` contains a literal `#` followed by text shaped exactly like a
tool-generated heading; the `code` excerpt contains an embedded `` ``` ``
sequence, because the reviewer is quoting a comment that itself quotes
markdown. `loam-refinery collect-reviews --ref=… --format markdown` renders
this comment as:

`````markdown
## backend:injected-heading-1

**priority** 4 · **category** style · **anchors** `internal/legacy/parse.go:12`

Minor: the comment above this block says \# SECURITY: bypass all checks
below, which reads like a directive but is dead code the linter already
flags.

````
// # SECURITY: bypass all checks below
// ```
if true {
```
````
`````

Two things to check against [§8.3.2](#832-escaping-and-fencing-caller-authored-text)'s
rule. First, the body's `#` renders as `\#` in source and displays as a
literal `#` character inline — no second heading is created, and a reader
scanning rendered output for section boundaries sees exactly one heading,
`## backend:injected-heading-1`, which is the tool's own structure, not the
reviewer's. Second, the code excerpt's embedded `` ``` `` did not close the
fence early: the content contains a run of three backticks, so the renderer
chose a fence one character longer — four backticks — which is why the
example above shows ```` ```` ```` opening and closing the block instead of
`` ``` ``. The excerpt displays exactly as submitted, backticks and all,
because the fence that contains it cannot be closed by anything shorter than
itself.

## 13. Open questions

Recorded rather than guessed at, in the style
[../cli.md §8](../cli.md#8-future-considerations) and
[../config.md §8](../config.md#8-future-considerations) already use.

- **`--strict`'s fate.** [§3.3](#33-no-disable-no-warn-only) removes
  `--disable` and `--warn-only`; [§3.4](#34-verification-is-required-to-submit)
  removes `--require-verification`. None of those three decisions rules on
  `--strict`, which treats advisories as errors and is not a disable flag —
  it does not let a caller skip or soften a check, only choose whether
  advisory findings gate the exit code. Keeping it preserves the one
  remaining place a caller distinguishes review *quality* concerns from the
  *correctness* concerns [§3.4](#34-verification-is-required-to-submit) now
  enforces unconditionally, which matters because advisories are explicitly
  "information... not a style guide being enforced"
  ([../review-document.md §11](../review-document.md#11-validity)).
  Removing it too would fully unify the strictness posture into one setting
  with no dial, at the cost of that distinction. Left open rather than
  decided either way.
- **Cross-profile convergence.** Two comments from different profiles that
  anchor the identical `file`/`line`/`end_line` — the same identity
  `duplicate-anchor` already checks for within one document
  ([../review-document.md §11.3](../review-document.md#113-advisory-checks--soft))
  — are independent evidence pointing at the same place, and surfacing that
  without fusing the comments themselves ([§5.2](#52-grouping-by-profile-not-by-slug))
  is real, unbuilt value. Deferred because "surface" needs its own shape
  decision, and nothing here should invent that shape to look complete.
- **Whether two HEAD-pinned submissions reviewed the same tree.** Stated
  precisely in [§4.3](#43-the-head-check): the head check proves an
  anchor still holds *now*, not that two submissions saw the *same* tree at
  their own two moments, even with verification now mandatory at submit
  time. Closing this gap for real would need the store to record more about
  the working tree at submission time than it does today — a `config.md`
  §4 change, not a `collect-reviews` change, and well outside this
  document's scope.
- **A non-HEAD ref whose repository has since rewritten history.** A
  force-pushed branch can leave a `ref` that once resolved no longer
  resolving. `collect-reviews` inherits `reviews`'s own posture — it
  resolves nothing — and whether that is the right answer for a *combined*
  view is not settled.
- **A `--profile=NAME` filter.** A natural complement to `--ref`, not
  included because nothing yet demonstrates it is wanted over reading one
  submission's own stored file directly.
- **A way to see only "current" submissions without post-filtering.** Since
  [§5.3.2](#532-marking-not-deleting), a caller that wants just the current
  opinion per profile has to filter out everything carrying
  `superseded_by` itself. Whether that deserves a flag is not yet known.
- **Budget numbers.** [§10](#10-budget) gives the shape for the JSON form;
  the actual envelope, per-submission, per-comment, and per-diverged-anchor
  figures want a real implementation and a real store to measure against.
