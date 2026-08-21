# The loam-refinery CLI

Tool specification. Version 0.2 (draft).

For the format this tool reads, see [review-document.md](review-document.md).
For where settings and stored reviews live, see [config.md](config.md).

## 1. Overview

`loam-refinery` accepts a review document and answers one question: **is this review
well-formed, and where does it fall short of being worth acting on?** It defines
the contract, enforces the parts that must hold for a document to be consumable,
and advises on the parts that are matters of review quality.

Structured review is only worth adopting if it makes reviewing *cheaper* than
prose, not merely more reliable. Every command below is shaped by that: the tool
is invoked in a loop by agents that pay per token, and a contract that costs
4,000 tokens to learn has already lost to the unstructured paragraph it replaced.
See [§6, Token economy](#6-token-economy).

In scope:

- Structural validation: schema conformance and referential integrity
- Verification: anchors checked against a caller-supplied source of truth
- Advisory validation: consistency, substance, and calibration signals
- A `prime` command that teaches the workflow in one small call, optionally
  carrying an operator-authored reviewer profile
  ([§2.1.1](#211-reviewer-profiles))
- A `describe` command that discloses the contract progressively, on demand
- Diagnostics addressed by comment ID, so feedback is directly actionable
- Keeping a local copy of reviews that passed, and getting them back out
  ([config.md](config.md))

Explicitly out of scope:

- Fetching anything over a network — PRs, diffs, remote refs
- Posting to GitHub or any forge
- Reading file *contents* to judge them. Verification asks a checkout whether a
  path and line number exist at a ref, and reads the working-tree copy of an
  anchored file — but only to tell whether it is still the file `ref` names,
  never to decide whether the anchor itself is right
  ([§2.3.1](#231-verifying-anchors)). Judging what a line *does* would make
  `loam-refinery` a reviewer rather than a referee, and that boundary has not
  moved. Amended; see [Amendments](#amendments) below.
- Assessing the *truth* of a review's claims. `loam-refinery` cannot know whether
  "this nil check is missing" is correct; it only knows the comment is anchored,
  prioritized, substantive, and honest about what its suggestions cost.
- Aggregating reviews. Passing reviews can be kept
  ([config.md](config.md)), but they are kept side by side — never merged,
  ranked, reconciled, or summarized across runs. Amended; see
  [Amendments](#amendments) below.

### Design principles

1. **Offline, and read-only about the repository.** No network, ever. `loam-refinery`
   reads the repository it is run in to verify anchors
   ([§2.3.1](#231-verifying-anchors)): does this path exist at this ref, does it
   have this many lines, and — to know whether that question can even be asked
   — does the working-tree copy of the file still match the one at `ref`. It
   never reads a diff, never forms an opinion about code, and never carries
   repository content into or out of a review: the working tree is consulted
   to decide whether `ref` is still authoritative for a file, never treated as
   a second source of truth to verify against. It never writes inside the
   repository either: the store is a directory under the user's home
   ([config.md §2](config.md#2-locations)). Amended; see
   [Amendments](#amendments) below.
2. **The schema is the documentation for lenses.** Field descriptions and
   examples live in the annotated schema, and `describe --lens=NAME` renders
   each entry from it ([§2.2.3](#223-the-entry-registry)), so a lens
   explanation cannot drift from enforcement. `describe`'s own summary
   ([§2.2](#22-describe)) is different: hand-written prose, chosen to read
   well as one paragraph rather than assembled from entries. Nothing renders
   it from the schema, so nothing stops its prose from going stale — what
   stops it is narrower: a test pins the summary's field list against the
   schema's own property list, so a field can go undocumented there for no
   longer than it takes CI to notice. Amended; see
   [Amendments](#amendments) below.
3. **Pay for detail on demand.** No command emits everything it knows. `prime`
   teaches the loop, `describe` summarizes the contract, `describe --lens` opens one
   field or one failed check. A caller reads the paragraph it needs and nothing
   else.
4. **Errors route to their own explanation.** Every diagnostic names a check, and
   every check name is a valid lens. Recovering from a validation failure is a
   lookup, never a re-read of the whole contract.
5. **Everything is addressable.** Every comment carries an ID the reviewer chose.
   Diagnostics, downstream agents, and humans all refer to findings by that ID.
6. **Advise, don't police.** Default exit status reflects whether the document is
   *usable*, not whether it is *good*.
7. **Cheap to call.** Fast startup, no config file required, terse default
   output. A config file exists ([config.md §3](config.md#3-the-config-file))
   but is never necessary, and by construction cannot change whether a document
   is valid — only whether a copy of the answer is kept. The config
   *directory* holds one other thing a person may write — reviewer profiles
   ([§2.1.1](#211-reviewer-profiles)) — and the same rule binds them: they
   change what `prime` prints, never what `submit-review` decides.

### Amendments

Design principles are revised in the open, not contradicted quietly — the same
posture [config.md §1.1](config.md#11-what-this-changes-about-the-design-principles)
uses for its own promises. Consulting the working tree to decide whether an
anchor is checkable ([§2.3.1](#231-verifying-anchors)) touches four
statements made above:

| Principle | Standing |
| --- | --- |
| Verification reads only whether a path and line exist, never file content (design principle 1) | **Amended.** Verifying an anchor now also reads the working-tree copy of the anchored file, to compare it against the blob at `ref` — git's own comparison, filters applied, not a raw byte diff. What survives is the reason the principle existed: `loam-refinery` compares the file against `ref` to decide whether `ref` is still authoritative for it, and never reads to interpret what a line means — reading to decide checkability is not reading to review. |
| Reading file contents is out of scope (§1, explicitly out of scope) | **Amended, narrowly.** Reading to *compare* — is this, right now, the file `ref` names — is now in scope, for the one purpose the check exists for. Reading to *judge* — forming an opinion about what a line does — remains out of scope, and that narrower boundary is the one that actually matters: it is what keeps `loam-refinery` a referee rather than a reviewer. |
| Anchors resolve only against the object database, never a working tree ([§2.3.1](#231-verifying-anchors)) | **Amended, and now re-amended.** The working tree is consulted once, and only when `ref` is `HEAD`, to decide whether an anchor is checkable at all — whether the file at `ref` and the file on disk are still the same file. It never supplies an answer of its own: it can only withhold a verification, never grant one — that much is still exactly true, and is the reason the working tree gets consulted at all rather than being ignored in favor of the object database alone. What changed is what withholding *does*: a diverged file is no longer reported unverified and passed through; it fails the precondition outright, once for the document, at exit 3, before the object database's own answers for every other anchor are even reached. |
| Exit 0 means anchors verified where a repository was available ([§4](#4-exit-codes)) | **Amended, and now re-amended twice.** A repository being available no longer implied every anchor was verified merely because a run exited 0 — that gap was real for one release, while `anchor-worktree-diverged` ([§2.3.1](#231-verifying-anchors)) was a routine, non-fatal reason some anchors were not. It closed with [§2.3.1](#231-verifying-anchors): the check still only withholds a verification, never wrongly denies one, but a withheld verification stopped being tolerated. What changed a second time is *how* it stopped being tolerated: a diverged anchor is now caught by a precondition before the rest of the run even starts, exit 3, not folded into exit 1 with the other verification failures. Either way, exit 0 means every anchor was verified, full stop; a document carrying any anchor the working tree could not confirm does not reach exit 0 at all — it does not even reach the checks that would have counted it. |

Two things do not move. `loam-refinery` still forms no opinion about what an
anchored line *does* — a match says the file on disk is still the one `ref`
names, not that the comment about it is correct, and judging that remains the
reviewer's job, not the referee's
([review-document.md §11.2](review-document.md#112-verification-checks--conditional)).
And the tool still never writes inside the repository, and still opens no
socket: comparing a working-tree file against a blob touches neither
promise.

A second amendment lands in this list for an unrelated reason:
[docs/features/combined-reviews.md](features/combined-reviews.md) specifies
`collect-reviews`, a command that reads several stored reviews for one ref
back as one attributed output.

| Principle | Standing |
| --- | --- |
| Aggregating reviews is out of scope (§1, explicitly out of scope) | **Amended, narrowly.** `collect-reviews` combines the reviews stored for one ref into one attributed output — see [docs/features/combined-reviews.md](features/combined-reviews.md). Findings are combined, never fused across profiles, ranked, or reduced to one verdict; within one claimed profile, a later submission is marked current relative to an earlier one, without deleting the earlier one's findings. Aggregation across *refs* remains entirely out of scope: `collect-reviews` combines within one ref and refuses to run without one. |

A third amendment corrects the record rather than the tool: the schema
gained the `profile` field in the same branch that added `collect-reviews`,
and `describe`'s hand-written summary went a full amendment pass without
mentioning it — proof that "the schema is the documentation" was never true
of that summary the way it is of a lens.

| Principle | Standing |
| --- | --- |
| The schema is the documentation (design principle 2) | **Amended, narrowly.** True of `describe --lens=NAME`, which renders each entry from the annotated schema. Not true of `describe`'s own summary, which is hand-written prose and always was — nothing ever rendered it from the schema. What changed is that the gap is now mechanically pinned: a test compares the summary's field list against the schema's property list, field for field, so a field added to one without the other fails the build instead of surviving an amendment pass unnoticed. |

## 2. Commands

```
loam-refinery prime           [--profile=NAME] [--list]
loam-refinery describe        [--lens=NAME[,NAME...]]
loam-refinery submit-review   [path] [--strict]
loam-refinery reviews         [--repo=NAME] [--ref=SHA] [--limit=N] [--content]
                              [--failed] [--list]
loam-refinery collect-reviews --ref=SHA [--repo=NAME] [--format json|markdown]
loam-refinery schema          [--annotated]
loam-refinery version
```

Global: `--help` on any subcommand.

Four of them form a ladder over the contract, cheapest first. A caller climbs
only as far as its current problem requires:

| Call | Answers |
| --- | --- |
| `prime` | How do I use this tool at all? |
| `describe` | What does a review have to contain? |
| `describe --lens=NAME` | What exactly does *this* field or *this* failed check want? |
| `schema` | Give me the machine-readable grammar. |

`reviews` and `collect-reviews` are not rungs on that ladder. They answer a
different question — *what was concluded about this commit already*, alone or
combined — and a caller that never stores anything never needs either one.
`collect-reviews` reads the same store `reviews` does, and is specified in
full in [docs/features/combined-reviews.md](features/combined-reviews.md).

Neither is `prime --profile`. The ladder is climbed by a reviewer that has run
into something it does not know; a profile is handed to a reviewer before it
knows anything, by whoever launched it ([§2.1.1](#211-reviewer-profiles)).

### 2.1 `prime`

Teaches the **workflow**, not the contract. Prints: what the tool is for, the
write → `submit-review` → `describe --lens` → revise loop, the exit codes, and the
instruction to reach for `describe` rather than guess.

Deliberately does **not** print the field list, the ladders, or the example —
that is `describe`, and a model that has not yet decided to write a review does
not need them. This is the one call that may be pinned into a system prompt for a
whole session, so it is the one that must stay small.

The workflow it teaches:

1. Write a review document.
2. Run `loam-refinery submit-review`.
3. Exit 0 — done. Exit 2 — the invocation is wrong, not the review. Exit 101 —
   the tool failed; nothing about the review or the command will fix it.
4. Exit 1 — each diagnostic names a check. Run
   `loam-refinery describe --lens=<check-name>` for the ones you do not understand,
   fix, and submit-review again.
5. Unsure about a field before writing? `loam-refinery describe --lens=<field>`. Never
   guess at an enum or a scale.

Budget: ~250 tokens. See [§6](#6-token-economy).

#### 2.1.1 Reviewer profiles

`--profile=NAME` appends an operator-authored profile to the end of `prime`'s
output. The tool's own text is unchanged, byte for byte, whether a profile is
named or not.

A profile answers a question the contract cannot: *who is reviewing, and what
should they be looking hardest at.* A Go service and a Terraform module want the
same document shape and very different attention, and that difference belongs to
whoever runs the reviewers — not to a binary everyone installs.

It rides on `prime` rather than on `describe --lens` because the two commands
push in opposite directions. `describe` is **pull-based**: a reviewer fetches a
lens at the moment it becomes uncertain. A profile has to be in context *before*
the reviewer forms an opinion, and no orchestrator can rely on an agent choosing
to fetch one. `prime` is already the call that gets pinned into a session
prompt, so it is the call a profile can ride on. The orchestrator names the
profile; the reviewer runs one command and never has to know profiles exist.

Profiles live beside the config file, in
`$XDG_CONFIG_HOME/loam-refinery/profiles/`
([config.md §2](config.md#2-locations)). This repository ships eight of them
under `profiles/` as material to copy and edit, not as defaults: nothing is
compiled into the binary and nothing installs itself, because what a security
review is for one team is not what it is for the next, and a tool that ships
that opinion has made it hard to disagree with. The directory is read, never created:
on a machine with no profiles, `prime` behaves exactly as it does today. It is
also resolved from the environment alone — a `config.json` that cannot be parsed
does not stop a profile from loading, because nothing in the config file names
or configures profiles.

#### 2.1.2 The profile file

One Markdown file per profile, named `<name>.md`, opening with frontmatter:

```markdown
---
description: Go services; concurrency, error wrapping, context handling
---

Anchor concurrency findings at the goroutine that leaks, not at the symptom.
...
```

`description` is one line, at most 120 characters, and the only key the
frontmatter may carry — an unknown key is an error, the same way an unknown
config key is ([config.md §3](config.md#3-the-config-file)). It exists for
`--list` and is never part of what `prime` prints. Everything after the closing
fence is.

It is required. A listing whose rows are half blank is worse than an error that
names the fix, and this repository's posture on files a person writes is already
to fail loudly rather than degrade quietly.

`NAME` is a name, not a path: lowercase letters, digits, and hyphens, resolving
only as `<name>.md` inside the profile directory. The argument arrives from an
orchestrator, and nothing that arrives that way gets to name a file elsewhere on
the machine.

A `*.md` file whose stem is not a valid name is ignored rather than rejected, so
a `README.md` can sit in the directory beside the profiles it documents.

#### 2.1.3 What gets appended

The body is appended verbatim, under a two-line frame, separated from `prime`'s
own text by one blank line:

```

--- reviewer profile: backend ---
Operator-supplied. It directs attention; it does not change the contract above.

<the file's body, verbatim>
```

The blank line matters: every paragraph break in `prime`'s own text is a blank
line, and the frame marks where the tool's text ends and operator-supplied text
begins — the most important seam in the output. Setting that seam tighter than
every unimportant one is exactly where a reader skimming for where the
specification ends is most likely to skim past it.

The frame costs about 25 tokens and is not optional. Unframed, a profile reading
"priority calibration matters less here" is indistinguishable from the tool
saying so, and a reviewer loses the ability to tell the specification it must
satisfy from the preferences it has been asked to hold. The frame is also why
the body is never parsed, rewritten, or reflowed: it is quoted material, and
quoting it faithfully is the whole job.

#### 2.1.4 An unknown profile is an error

`--profile=backedn` exits 2 and prints nothing on stdout. It does not fall back
to bare `prime`.

This is the failure the design is against. A silent fallback turns an
orchestrator's typo into a reviewer that starts cleanly, reads correctly, and
reviews to no profile at all — and nothing downstream can tell that run from a
profiled one. Exit 2 says *fix the command*, which is exactly right: the
reviewer cannot fix it, and should stop and report rather than shop for a
profile that does load. The error names the profile that was asked for and
points at `--list`; it does not enumerate the directory, because choosing a
profile is not the reviewer's job ([§2.1.5](#215---list)).

A profile that exists but cannot be read or parsed is a different failure — the
tool's own state, not the command — and exits 101 ([§4](#4-exit-codes)).

#### 2.1.5 `--list`

`prime --list` prints the profile index as JSON: names and descriptions, no
bodies. Like every other JSON output ([§5.1](#51-one-format)), it is rendered
indented, not compact:

```json
{
  "profiles": [
    {
      "name": "backend",
      "description": "Go services; concurrency, error wrapping, context handling"
    }
  ]
}
```

It is an operator affordance, not a rung on the disclosure ladder. Nothing in
`prime`'s prose mentions it, and nothing spends tokens telling a reviewer that
profiles exist. The orchestrator knows which profile it wants before it launches
anything, and a reviewer browsing profiles is a reviewer choosing what standard
to hold itself to.

No profile directory is an empty list and exit 0, not an error — the same answer
`reviews` gives for a store that does not exist yet
([config.md §6.2](config.md#62-empty-answers)). `--list` combined with
`--profile` is a usage error.

**A file that fails to parse is left out of the index, not treated as a reason
to fail the whole call.** `--list` answers one question — *what profiles can I
use* — and a file that does not parse is not a usable profile, so omitting it
from the index is the correct answer rather than a concession. It is named on
stderr instead (`skipping unparseable profile <file>`), and the call still
exits 0 with the profiles it did find. The failure does not disappear:
`prime --profile=NAME` for that same file still exits 101 with the specific
parse error ([§2.1.4](#214-an-unknown-profile-is-an-error)), which is where an
operator can actually act on it. Only the directory itself failing to resolve
or read — as opposed to one file inside it failing to parse — is a tool error
for `--list` too, and still exits 101 with nothing on stdout
([§4](#4-exit-codes)).

#### 2.1.6 What a profile may not do

A profile shapes attention. It does not touch the contract.

Nothing in a profile is read by `submit-review`, and no part of one feeds
`--strict`. This is
[config.md §3.1](config.md#31-what-configuration-may-not-do) applied to a second
file in the same directory, for the same reason: the moment a file on one
machine can decide what counts as a valid review, `prime --profile=backend` here
and `prime --profile=backend` there produce reviewers held to different
standards, and `submit-review` agrees with both. A profile that wants a check
enforced is asking for a flag on the `submit-review` invocation, and that flag is the
orchestrator's to pass.

The line, concretely: a profile may say *look hard at context cancellation*. It
may not say *skip the priority advisory*.

### 2.2 `describe`

Explains the contract, disclosed progressively.

**Default** — a summary of the document shape: the root fields, the comment
fields, the enum values without their full explanations, and a minimal valid
example. Enough to write a review that will usually pass, and to know what to ask
about next. Ends with the lens index. Budget: ~600 tokens.

**`--lens=NAME`** — one entry in full. Budget: ~250 tokens per entry.

The name is `describe` rather than `docs` because the command does not serve a
document — it answers a question about one named thing. That framing is what lets
its subject matter grow past the schema without the command surface changing.

#### 2.2.1 Lens names

A lens name resolves to an **entry** in the entry registry ([§2.2.3](#223-the-entry-registry)).
Every name carries a namespace, which may be written explicitly and is otherwise
inferred:

| Namespace | Holds | Named by | Example |
| --- | --- | --- | --- |
| `field` | One document field | Its JSON path | `field:comments.suggestions.effort` |
| `check` | One structural, verification, or advisory check | Its check name | `check:id-unique` |
| `topic` | A cross-cutting concept | A short slug | `topic:ids` |

**Fields are named by path**, because field names are not unique on their own:
`summary` is on the root and on suggestions, `code` on comments and suggestions.
A flat name space would have to pick a winner for those, and picking silently is
how a caller ends up reading about the wrong field.

**A bare final segment resolves when exactly one field ends with it.** So
`--lens=priority` and `--lens=effort` work, and are what a model will normally
emit and what `lenses` names. `--lens=code` does not: two fields end in
`code`, so it exits 2 listing `comments.code` and `comments.suggestions.code`.
Short where short is unambiguous, explicit where it is not.

Check names are kebab-case and field paths are dotted, so those two can never
collide. Topics are slugs chosen not to collide with either.

**A name that could mean more than one entry is never guessed at.** It exits 2
and lists the candidates in full, so the caller picks rather than discovering
later that it read about the wrong thing. The namespace prefix is there for
callers that want to skip resolution entirely, and is what will keep a future
`kb:` namespace from disturbing any of this.

**Aliases.** An entry may carry alternate names, and an alias resolves silently
to its entry. This is the rename escape hatch: a lens name is API — it is printed
in the `lenses` field, cached in agent memory across sessions, and written into
prompts — so an entry that gets renamed keeps its old name as an alias rather
than breaking callers. Aliases are not dropped.

`--lens` accepts a comma-separated list and emits each entry once, in the order
given, deduplicating repeats. Batching matters: three lenses in one call pay the
process cost once and the caller's framing once, where three calls pay both three
times.

#### 2.2.2 Entry content

A **field** entry gives: what the field means, its type and constraints, the full
enum or scale with per-value meaning, the mistake models actually make with it,
and a fragment showing it used well.

A **check** entry gives: what triggers it, why the check exists, what to change,
and a before/after fragment.

Every entry must be **self-contained**. An agent arriving from a validation
failure should be able to fix the problem from that entry alone, without a second
call and without reading the summary. An entry that defers elsewhere has spent
tokens to save none, which is the failure mode that quietly destroys the whole
progressive-disclosure argument ([§6.2](#62-why-progressive-disclosure)).

Entries may end with a `related:` line naming other lens names — bare names only,
one line, no prose. It costs a handful of tokens and lets a caller navigate
deliberately instead of re-reading the summary to find out what else exists. It
is a navigation aid, never a substitute for the entry being complete.

#### 2.2.3 The entry registry

`describe` renders whatever the registry holds. It does not know where entries
come from, and adding a source does not touch the command, its flags, or its
output format.

Entries are contributed by **providers**:

| Provider | Contributes | Derived from |
| --- | --- | --- |
| schema | `field:*` | The annotated JSON Schema |
| checks | `check:*` | The structural and advisory check registries |
| topics | `topic:*` | Hand-written entries compiled into the binary |

Two properties make this worth stating as architecture rather than leaving
implicit:

1. **Explanation cannot drift from enforcement.** The schema and check providers
   read the same values that `submit-review` runs on. A new advisory is documented
   the moment it is registered, and every check name `submit-review` reports is a
   name `--lens` explains.
2. **New subject matter is a new provider, not a new command.** Anything the tool
   should be able to explain — review heuristics, per-language calibration
   guidance, worked examples by category, project conventions — arrives by
   registering entries. Callers keep using `describe --lens=<name>`, the pointer
   line keeps working, and the budget discipline in [§6](#6-token-economy)
   applies to the new entries unchanged.

The `kb` namespace is **reserved** for that purpose and is not implemented. See
[§8](#8-future-considerations).

An entry is a small value: name, namespace, aliases, title, body, related names,
and the provider that contributed it. The registry is assembled at construction,
not package-level state, so a test can build a registry holding one entry.

#### 2.2.4 Unknown, ambiguous, and empty lenses

| Case | Behavior |
| --- | --- |
| Unknown name | Exit 2, print the full lens index |
| Ambiguous bare name | Exit 2, print the qualified candidates only |
| Empty `--lens=` | Exit 2, usage error |

Lens names are short, so printing the whole index costs less than a round trip
spent guessing. Never silently fall back to the default summary: a caller that
asked for detail and got a summary cannot tell that it missed.

#### 2.2.5 `--list`

Prints the lens index alone — every entry name grouped by namespace, one line
each, no bodies. This is the cheap way to find out what the binary can explain,
and it is what the unknown-lens error prints. As the registry grows it stays
proportional to the number of entries rather than their content.

### 2.3 `submit-review`

Reads a review document from `path`, or from stdin when `path` is `-` or omitted.
Immediately after parsing it runs verification, which is no longer
conditional on a source being supplied: it always runs, and a document that
carries any anchors needs a working source of truth to pass. Verification's
own result answers one question before anything else does — is the
reviewed state actually a commit — and when it says no, the run stops
there and reports only that; otherwise the structural checks run, then the
advisories, emitting diagnostics and setting the exit code per
[§4](#4-exit-codes) — see [§2.3.1](#231-verifying-anchors).

**Every check runs, with one exception.** A failure in one tier does not gate
the next, and a failure on one comment does not gate the others — see
[review-document.md §11.4](review-document.md#114-partial-documents). One
`submit-review` call reports everything wrong with a document that can be
determined from it, because the alternative is a caller paying a full
write-submit-review cycle per mistake. Checks that genuinely cannot run are
listed as skipped rather than passing silently. The exception is the
precondition itself ([§2.3.1](#231-verifying-anchors)): when the reviewed
state is not a commit, nothing else runs at all, deliberately.

Input must be a single JSON object. Multiple documents, JSON Lines, and arrays
fail `document-unparseable` and exit 1: the input is a document to repair, not
an invocation to fix.

A run also records itself in the store, and keeps a copy of what it read when
there is one to keep — the document under `reviews/` on exit 0, the submitted
input under `rejected/` on exit 1, no file at all on exit 3, since the
precondition that produces it fires before a document is examined. Every run
still records a row, exit 3 included; [config.md §5](config.md#5-storing-a-review)
is the full rule for which run writes what and why, and this section does not
restate it. There is no flag for storing and no mention of it in the output: it
is on by default, turned off only for a whole machine in the config file. It
never changes the *verdict*, but a store that cannot be written fails the
command with exit 101 — see [config.md §5.1](config.md#51-when-storing-fails).

#### 2.3.1 Verifying anchors

An anchor is a factual claim: *this path, at this ref, has this line*. Nothing in
the document can confirm it, and a hallucinated line number is indistinguishable
from a real one on inspection — it is well-formed, plausibly numbered, and wrong.
Verification is the only check tier that catches it.

It runs **by default**, against the git repository containing the working
directory, discovered the way git itself discovers one — walking up from the CWD
until a repository root is found. There is no flag. Path and line existence
resolve against the object database — `git ls-tree` for the path, `git
cat-file` for the object behind it. No ref resolution is
involved, because a ref is already a SHA; there is nothing to disambiguate and no
chance of resolving to a different commit than the reviewer saw. Commits that are
not checked out still work, and that stays true without qualification: the
working tree is consulted only when `ref` **is** the checked-out commit, never
otherwise, and even then only to decide whether the object database still has
the last word on a given file — never as a second place to check an anchor
against; see below.

Verifying by default rather than on request is the deliberate part. An opt-in
check makes the plain invocation the weak one, and the plain invocation is what
gets run — a tool whose whole purpose is catching hallucinated anchors should not
require an argument to do it. Pointing `loam-refinery` at a different repository is
`cd`, which every caller already has and no caller has to be taught.

When no repository can answer, verification is **skipped and reported as
skipped**, with the reason in `verification`. It is never silently passed: a
run that verified nothing must not look like a run that verified everything, or
the tier is worse than absent — it would license confidence it never earned.

**The precondition, read off verification's own result.** `submit-review`
asks one question none of the rest of verification does: is the reviewed
state a commit at all. Verification finds the answer the same way it finds
everything else, by checking every anchor; when `ref` names the
repository's current `HEAD` and that pass turns up at least one anchored
file whose working-tree copy has since diverged from the blob at `ref` —
the ordinary state of a checkout somebody is actively editing — the run
stops there: before the structural checks, before the advisories, and
before the rest of verification's own findings are added to the result,
discarded rather than reported alongside a premise that was never sound.
One diagnostic reports it, in `diagnostics`, naming `anchor-worktree-diverged`
once for the document rather than once per diverged anchor — exit 3, not
exit 1; [§4](#4-exit-codes) argues why that is its own code. A reviewer
that has to read twelve anchor failures to learn its whole premise was
wrong is being told the same thing twelve times, badly, and a document
whose reviewed state was never committed is not one worth carrying into the
structural and advisory tiers once that much is known.

```json
{
  "valid": false,
  "diagnostics": [
    {
      "severity": "error",
      "name": "anchor-worktree-diverged",
      "message": "internal/fetch/client.go differs from 4f2c1a9 in the working tree; the reviewed state is not a commit. Commit what was reviewed, or run \"git stash create\" and resubmit against that SHA — do not retry against this ref."
    }
  ],
  "lenses": ["anchor-worktree-diverged"]
}
```

(The rest of this shape — `counts`, `skipped` — is not pinned here; a real
implementation decides it, per [§5.2](#52-the-result-object).)

The fix is the reviewed state, not the review, and
`describe --lens=anchor-worktree-diverged` says so directly: commit what was
reviewed — even to a throwaway branch — or run `git stash create`, which
builds a real commit object out of the working tree and the index without
touching either, giving the reviewer a genuine, resolvable SHA to anchor
against instead of `HEAD` plus a promise that nothing will move. Neither is
optional to state, because this is exactly the failure a retry loop makes
worse: resubmitting the identical document against the identical dirty tree
cannot succeed, and a reviewer that treats exit 3 like exit 1 — revise and
resubmit — loops until something outside the review changes.

Once that precondition passes, every remaining anchor claim must still be
confirmed. `verification-required` fires — exit 1 — whenever an anchor claim
went unchecked for a reason short of a diverged working tree: no repository,
an unreachable one, or a single file git could not read. There is no flag to
demote it and no way to accept the gap: a document is not wrong for being
checked somewhere that could not check it, but as of this release nothing
enters the store unverified either: `collect-reviews`
([docs/features/combined-reviews.md](features/combined-reviews.md)) depends
on `ref` as a join key across independently-submitted reviews, and a store
that could hold both a fully verified submission and one nobody had ever
checked would put a caller in the position of having to know to ask which was
which. Requiring verification unconditionally is what lets a combined view
skip that question entirely.
A document with no anchors is never failed by this: there is nothing for it to
ask about.

Two things can go wrong there, and they are not the same thing. Running outside
a repository is ordinary, and reports `source: none`. A repository that exists
but could not be asked — git missing, a bare repository, a checkout git refuses
on ownership grounds — reports `source: unavailable` and carries git's own
words. Neither is a defect in the document, and a caller can still tell them
apart in the output — but neither is free anymore either: a document with
**any** anchor fails when nothing could confirm it, regardless of which of the
two produced the gap. Only a document with no anchors at all passes either
way, because there is nothing for either failure to withhold.

Being run in the *wrong* repository is loud rather than silent, and needs no
special handling. The reviewed commit will not exist in an unrelated repository,
so the run fails with one `ref-unknown` naming a SHA the caller can recognize as
foreign. The failure mode announces itself, in one line.

A caller-supplied file list — the changed paths an orchestrator already has — was
considered as an alternative source and rejected. A list carries no identity, so
nothing can confirm it describes the commit the review names; it would verify
anchors against *some* revision and report success, which is the failure this
format spends its whole design avoiding. The same reasoning that rules out branch
names as refs rules out a manifest as a source of truth: something that resolves,
but not provably to what was reviewed, is worse than nothing because it reads as
confirmation.

The working tree answers a different question from the one path and line
existence already answer, and only when `ref` **is** the checked-out commit.
When `ref` names some other commit, the working tree has nothing to do with
what the reviewer read, and consulting it anyway would silence anchors the
object database can already answer perfectly — the opposite of what "commits
that are not checked out still work" promises earlier in this section.
Restricting the comparison to `ref == HEAD` is what keeps that promise true
rather than merely repeated.

The property that licenses consulting the working tree at all is
**monotonicity**: the working tree can only ever *withhold* a verification,
never *grant* one. Every verified result is still computed against `ref` from
the object database, exactly as before; the most the working tree can do is
turn a would-be answer into no answer. That is the opposite of the failure
the paragraph above rejects for a caller-supplied file list — a list "would
verify anchors against *some* revision and **report success**," and success
is precisely what the working tree is never allowed to manufacture.
Withholding is safe in a way granting is not, and that asymmetry is the whole
argument: nothing here is offered as a second source of truth, only as a
reason `ref`'s answer might not apply.

"Differs" means git's own answer to *has this file changed since `ref`* — not
a raw content hash, which would invent a second definition of "changed" that
disagrees with git's own: under `core.autocrlf=true`, a byte comparison sees
line-ending normalization that git's own comparison already accounts for, so
every tracked file would read as diverged and the check would be worse than
useless. `git diff <ref>` already answers this question, filters applied;
`git status` does not — it compares the working tree against `HEAD` and the
index, not against an arbitrary `ref`, which is one more reason the
comparison only runs when `ref` **is** `HEAD`.

Verifying an anchor is therefore three cases, not two, and they are disjoint:

| Case | Result | Why |
| --- | --- | --- |
| Path absent at `ref` | `anchor-file-missing`, error | Unchanged from today. The working tree is not consulted and cannot soften it — softening a definite answer from `ref` is exactly what monotonicity forbids. |
| `ref` is `HEAD`, path present at `ref`, a working-tree copy exists, and it differs from `ref` | `anchor-worktree-diverged` — the precondition above, not a per-anchor outcome reached from here | The file the reviewer may have read is not the file at `ref`. Checking the anchor's lines against either copy would mean checking it against a file the reviewer may never have seen. Requiring a working-tree copy to *exist* keeps this disjoint from the row above: git reports a deletion as a difference too, but a file missing from the working tree says nothing about what the reviewer read, so a deleted file falls through to the row below rather than landing here. |
| Everything else | Checked normally | Either `ref` is not `HEAD`, so the working tree is irrelevant, or it is `HEAD` and the file matches or is absent from the working tree, so nothing contradicts `ref`. Path and line existence resolve exactly as they always have, against the object database. |

`anchor-worktree-diverged` is a verification check, named the way
`anchor-file-missing` and `anchor-line-out-of-range` are, and it is a lens
like every other check name ([§2.2](#22-describe)) — the precondition above
did not replace it, it is the same check, monotonic exactly as before,
running once up front instead of once per anchor mid-flow. The check and the
policy wrapped around its result are not the same thing: the check is still
monotonic — it can only withhold a verification, never grant one, and firing
it never counts an anchor as *wrong*, only as *unconfirmed* — and that
property is untouched. What changed is the policy: a withheld verification,
which used to be tolerated, is now fatal.

An anchor `anchor-worktree-diverged` reports is never line-checked at all —
not against `ref`, not against the working tree. That is deliberate: a file
that has only grown since `ref` would otherwise fail
`anchor-line-out-of-range` against a length that was never the review's
problem, an error about the state of the checkout rather than about the
review.

Verification checks are errors, unconditionally: they are factual, not
matters of judgment, and nothing softens any of them anymore. The remaining
`verification-required` causes — no repository, an unreachable one, a file
git could not read — fail the submission at exit 1 once verification
actually runs, the same way `ref-unknown`, `anchor-file-missing`, and
`anchor-line-out-of-range` already do. A shallow clone that never fetched
the reviewed commit is the sharpest instance: a reviewer working from one
can no longer submit a document with any anchors in it, and there is no
flag to work around it. The only fix is a deeper fetch before submitting —
`git fetch --deepen` or an unshallow clone — not a command-line option.

Verification is not the only audit path, and deliberately not the last word.
Because a ref is an immutable SHA, every anchor in a passing review remains
resolvable by ordinary git tooling long after `loam-refinery` has exited — `git show`,
`git blame -L`, any reviewer or CI job that wants to check what the comment was
actually looking at. `loam-refinery` confirms the anchor points somewhere; git lets
anyone confirm it points at the right thing. Keeping those separate is why the
tool needs no state and no opinion about code.

Diagnostics carry check names, not explanations — the explanation is one
`describe --lens` away, and printing it inline would charge every caller for
detail most of them already have. The names worth opening are collected in
`lenses` ([§5.2](#52-the-result-object)).

### 2.4 `schema`

Writes a **minimal** JSON Schema to stdout: structure and constraints only, with
every `description`, `examples`, `title`, and `$comment` stripped. It validates
identically to the annotated source and is roughly a third the size.

This command is for machines that need the grammar — validating in another
runtime, generating types, constraining a sampler. It is not for teaching, and a
model reaching for it to understand a field is spending tokens on JSON structure
to learn something `describe --lens` states in a sentence. `prime` says so
explicitly.

`--annotated` emits the source schema with descriptions and examples intact, for
codegen that wants doc comments on generated types.

Both forms come from one embedded file, so there is no second copy to drift.

### 2.5 `version`

Version, commit, and schema version, one per line.

### 2.6 `reviews`

Queries the store an earlier `submit-review` wrote: which reviews passed for a
repository and commit, and — with `--failed` — which runs produced none. It
resolves nothing, fetches nothing, and writes nothing at all.

It is deliberately minimal: enough to confirm end to end that storing works and
to look at what was stored, and not a query API. Several obvious conveniences
are recorded as deferred rather than built.

By default it returns an **index** — when each review was stored, its ref, its
verdict, and its counts — not the documents themselves. That is the same
progressive-disclosure argument as `describe`: the cheap answer carries enough
to choose, and `--content` is the expensive one a caller asks for once it has.

The repository is inferred from the working directory the way verification finds
one, so `loam-refinery reviews` inside a checkout asks about that checkout.

Fully specified in [config.md §6](config.md#6-reading-the-store).

## 3. Flags

Behavior is adjusted by flag. The tool must stay usable with no setup, so every
flag has a default that works and nothing has to be configured before a first
run; the config file that exists ([config.md](config.md)) is optional and may
only set the store flags.

```
--profile=NAME            append one reviewer profile                prime
--list                    print the profile index, no bodies         prime
--strict                  treat advisories as errors (exit 1)        submit-review
--lens=NAME[,NAME...]     open one entry in full                     describe
--list                    print the lens index, no bodies            describe
--repo=NAME               which repository's reviews                 reviews
--ref=SHA                 which commit; the full 40-char SHA         reviews
--limit=N                 most recent N; 0 for all                   reviews
--content                 include each stored file, not just rows    reviews
--failed                  list runs that stored no review            reviews
--list                    print the repositories in the store        reviews
--ref=SHA                 which commit; required, no default         collect-reviews
--repo=NAME               which repository's reviews                 collect-reviews
--format json|markdown    output format; markdown is unique here     collect-reviews
--annotated               emit the schema with descriptions intact   schema
```

Structural checks cannot be disabled or demoted, and neither can verification
or advisory ones anymore: `submit-review` dropped `--disable`, `--warn-only`,
and `--require-verification` together, so every check that used to have a
caller-facing knob no longer has one — see [§2.3.1](#231-verifying-anchors).
Removing `--disable` does not make an advisory an error; it only removes the
ability to make the tool stop mentioning one a caller has decided it does not
want to hear about. Verification runs whenever a repository is found, and
when none is, or an anchor cannot be confirmed, the submission fails outright
rather than reporting a gap a caller could choose to accept.

`--strict` is how a caller opts into advisories gating a pipeline. It is not the
default posture: review quality is a judgment the caller owns.

`--profile` is the only flag whose value names a file on the machine rather than
something compiled in. It is constrained to a name in one directory
([§2.1.2](#212-the-profile-file)), and like an unknown check name, an unknown
profile is a usage error rather than a silent no-op.

The store flags are the only ones with a config-file counterpart, and the flag
always wins ([config.md §2.1](config.md#21-precedence)). No flag that affects
whether a document is valid may be set from configuration, which is why
`--strict` has no config-file counterpart
([config.md §3.1](config.md#31-what-configuration-may-not-do)). `--disable`,
`--warn-only`, and `--require-verification` are not flags with no counterpart
— they are not flags at all anymore.

## 4. Exit codes

| Code | Meaning | Whose problem |
| --- | --- | --- |
| 0 | Structurally valid, with every anchor verified — or with no anchors at all. Advisories may be present. | Nobody's |
| 1 | Structurally invalid, unparseable, a verification failure — an anchor the object database could not confirm, or a repository that could not confirm one at all — or advisories present under `--strict`. | The review |
| 2 | Usage error: unknown flag or command, an unknown or malformed profile name, an unreadable input path, a malformed `--repo` or `--ref`, `--list` combined with another `reviews` flag or with `--profile`. | The invocation |
| 3 | Precondition failure: the reviewed state is not a commit — `ref` names the repository's `HEAD` and at least one anchored file's working-tree copy has since diverged from it ([§2.3.1](#231-verifying-anchors)). The run still records a row, with no file — [config.md §5](config.md#5-storing-a-review). | Whoever launched the reviewer |
| 101 | Tool error: the tool's own state could not be read or written — an unparseable config file, a store that could not be created, a store directory that could not be read, a profile that exists but could not be read or parsed. | The machine |

Four questions, four answers, and an agent must be able to tell them apart
without parsing prose. Exit 1 means *revise the review*. Exit 2 means *fix the
command*. Exit 3 means *the review and the command are both fine; escalate* —
neither one is what needs to change. Exit 101 means *none of those will help*
— the review was fine, the command was fine, the state behind them was fine,
and the tool could not do its job.

That last answer — 101 — is the one worth having a code for on its own
terms. An agent that reads "2" and re-examines its command line will loop
forever against a read-only home directory; the fix is not in anything it
controls, and the only useful response is to stop and report. Folding
machine failure into the usage code is the same mistake as folding it into
the document code, one step further out.

Exit 3 deserves the identical argument, made once for the class it names.
**Not 1:** 1 means revise the review, and a document naming a real commit
with real anchors is not wrong — the reviewer read something real, it just
was not yet a commit. Telling an agent to revise a correct document is
exactly the loop this section exists to prevent. **Not 2:** 2 means fix the
invocation, which implies the caller *can* — here the fix is `git commit`,
or a different, resolvable `ref`, and a subagent reviewing on instruction
from an orchestrator typically has authority over neither. **Not 101–125:**
that band is the tool's own state failing, and this is neither the
machine's fault nor the reviewer's — the tool worked exactly as specified.
`submit-review` cannot revise a review the reviewer got right, and it
cannot commit on anyone's behalf; the only correct move is to say so and
stop.

**Codes 101–125 are the tool-error band**, and only 101 is assigned. The range
starts above 100 because the low codes are the crowded ones: a shell reports 2
for builtin misuse, 126 for a command that cannot execute, 127 for one that does
not exist, and 128+N for a signal. A code an orchestrator has to distinguish
from "the review needs work" should not share a number with everything else that
can go wrong between the process and the shell. 101–125 is the free window below
the reserved 126.

**Codes 3–9 are reserved the same way**, for the same class of problem one
layer earlier: not a tool failure, but a precondition failure — the
invocation and the review are both fine, and something about the reviewed
state itself is not. Only 3 is assigned. Nothing in the shell's own crowded
low range claims it either: 2 is builtin misuse, 126 is a command that
cannot execute, 127 is one that does not exist, and 128+N is a signal — none
of them touch 3, which is why it was free to take.

Advisories alone never produce exit 1. Verification failures always do —
there is no flag left to demote one. A working tree that has diverged from
`ref` produces exit 3 instead, once verification itself finds it — see
[§2.3.1](#231-verifying-anchors).

A failed store produces exit 101 — never 1, and no longer 2. A full disk is not
a defect in the review and not a mistake in the command line. Stdout carries
nothing in that case, as on any failing exit; the code is the signal and stderr
is the reason. See [config.md §5.1](config.md#51-when-storing-fails).

**This table is written in `submit-review`'s terms; `collect-reviews` uses a
narrower slice of it and nothing outside it.** It never exits 1 and never
exits 3 — there is no review to fail structurally or verify, and no working
tree whose divergence a read-only query could be blocked by. Every empty or
not-found case is 0; a malformed `--ref` or `--repo` is 2; a store that
exists but cannot be opened or read is 101. See
[docs/features/combined-reviews.md §9](features/combined-reviews.md#9-empty-and-failure-cases)
for the full table.

## 5. Output

### 5.1 One format

Everything a command has to say is one JSON object on **stdout**. There is no
second renderer and no `--format` choice left to make: two implementations of
the same result disagreed about what a run found three separate times, and one
of them let an author-supplied comment id forge diagnostic lines the document
never contained. A caller parses the object; nothing has to parse prose.

There is no `--format` flag here at all, and that follows from the same
argument rather than sitting beside it. `--format=json` used to be accepted
anyway, as a courtesy to callers who already passed it, and `--format=text`
used to be the error that told them where the old format went — a
backward-compatibility shim, kept for exactly one reason: not breaking
callers. That reason does not survive next to the rename: `validate` does
not survive becoming `submit-review` in any form — no permanent alias, no
deprecation window, no transitional stderr note telling a caller where the
command went, and a caller still typing `validate` on an upgraded binary
gets exit 2, "unknown command," the same as any other typo. `submit-review`
breaks every existing caller of `validate` outright, no alias, no grace
period, and a flag whose only job was softening a break has nothing left to
soften once the same change breaks louder anyway. On a
command with one legal output there is also nothing left for the flag to
choose — it would cost a row in [§3](#3-flags), a branch in the parser, and
a sentence of explanation, for a decision the caller never gets to make.
**`--format` exists where there is a format to choose.** One command has
two; the rest have exactly one, and carry no flag at all.

The prose that remains is prose because it is written to be read, not rendered:
`prime`, and the `summary` field of `describe`.

**Amended, narrowly, using the same table pattern
[config.md §1.1](config.md#11-what-this-changes-about-the-design-principles)
already uses:**

| Principle | Standing |
| --- | --- |
| No second renderer, no `--format` choice left to make | **Amended, narrowly.** `loam-refinery collect-reviews --format markdown` is the one exception, on the one command whose primary audience is a human reader rather than an agent in a loop — see [docs/features/combined-reviews.md §8.3](features/combined-reviews.md#83-the-markdown-projection). It is not a second *renderer* in the sense this section warns against: it is a pure projection of the identical result value the JSON form serializes, built once, by one code path, with the same escaping and fencing discipline specified there to close the forgery half of this section's own argument. `submit-review`, `reviews`, `describe`, and `schema` are unchanged in the sense this row is actually about — one command, one projection, one source of truth, not a second computation of any result. A later, unrelated decision removes their `--format` flag entirely rather than leaving it accepting one value: a flag chooses between formats, and none of the four has more than one to choose between, `collect-reviews` now being the sole command that does. |

### 5.2 The result object

```json
{
  "valid": true,
  "strict": false,
  "verification": { "source": "repo", "anchors": 9, "verified": 9 },
  "counts": { "comments": 6, "errors": 0, "advisories": 2, "skipped": 0 },
  "skipped": [],
  "diagnostics": [
    {
      "severity": "advisory",
      "name": "suggestion-no-cons",
      "comment": "dropped-context-1",
      "path": "/comments/0/suggestions/0/cons",
      "message": "suggestion 1 lists no cons; state the tradeoff or say the fix is free"
    },
    {
      "severity": "advisory",
      "name": "priority-flat",
      "message": "all 6 comments are priority 7; the scale is not being used"
    }
  ],
  "lenses": ["suggestion-no-cons", "priority-flat"]
}
```

`comment` carries the comment ID when the diagnostic concerns one comment;
omitted otherwise. `path` is a JSON Pointer into the input document; omitted for
document-level checks. `lenses` is the deduplicated set of lens names covering
the diagnostics, so a caller can fetch explanations without guessing at
names. Omitted when there are no diagnostics.

`skipped` groups the checks that could not run **by reason**, not one entry
per check: `{ "reason": "...", "checks": [...] }`. One cause commonly stops
several checks at once — no repository stops every verification check at
once — and a grouped entry says the reason once instead of repeating the same
sentence under every name it applies to. It is always present, `[]` when
everything ran. As with `verification`, absence must never be read as
success: a consumer that ignores this field will treat a check that never
executed as a check that found nothing.

**`counts.skipped` counts checks; `skipped` counts reasons, and the two
numbers routinely disagree.** `counts.skipped` is how many checks did not
run. `skipped` is an array of reasons, each carrying every check it stopped,
so several checks stopped by one cause collapse into a single element. A
document validated outside a repository shows the gap plainly:

```json
{
  "valid": true,
  "strict": false,
  "verification": {
    "source": "none",
    "reason": "not a git repository",
    "anchors": 1,
    "verified": 0
  },
  "counts": { "comments": 1, "errors": 0, "advisories": 0, "skipped": 3 },
  "skipped": [
    {
      "reason": "not a git repository",
      "checks": [
        "ref-unknown",
        "anchor-file-missing",
        "anchor-line-out-of-range"
      ]
    }
  ],
  "diagnostics": []
}
```

`counts.skipped` is `3` — three checks stopped, named in the group above —
and `skipped` has a single element, because all three stopped for the same
reason. Read `counts.skipped` for how many checks did not run and `skipped`
for why; `len(skipped)` is neither, and a consumer that treats it as a check
count will read this ordinary case as a mismatch.

`verification` is always present. `source` is `"repo"`, `"none"` when the run
was not inside a repository, or `"unavailable"` when one could not be asked;
with either of the last two, `verified` is `0` and a consumer can tell that
unverified anchors were not checked rather than found sound. A caller that treats a missing
`verification` block as "verified" is reading an older version, which is why the
field is required rather than omitted when empty.

Inside a repository, `verified` can still be less than `anchors`: a hard
verification error — `anchor-file-missing`, `anchor-line-out-of-range` —
counts an anchor without verifying it, exactly as always.

**There is no `verification.unverified` field on this command's output.**
An earlier revision of this design gave `anchor-worktree-diverged` a soft,
per-anchor home here — one entry per anchor a dirty working tree kept from
being checked, and the run still passed. [§2.3.1](#231-verifying-anchors)'s
precondition replaced that: a diverged anchor now stops the run before
verification even reaches per-anchor checking, reported once, in
`diagnostics`, at exit 3 — never as a soft entry in a passing `verification`
block, because a passing run with a diverged anchor in it can no longer
happen. The check did not go anywhere: `anchor-worktree-diverged` is exactly
as real as it was, still monotonic, still a lens ([§2.2](#22-describe)) — it
changed which part of the output it can appear in, not whether it exists.
The shape this field used to have is not gone from the tool, either:
[docs/features/combined-reviews.md
§4.3.1](features/combined-reviews.md#431-the-head_check-shape) is where the
identical per-anchor list now lives, on `collect-reviews`, asking a
different question — has an *already-verified* anchor drifted since — that a
precondition on `submit-review` cannot ask and does not need to.

The whole object goes to stdout; nothing is written to stderr except on a
failing exit, where stdout carries nothing.

**Nothing here describes the store.** A run keeps a copy of a passing review
([config.md §5](config.md#5-storing-a-review)) and says nothing about it: where
a copy landed is not a fact about the review, and `loam-refinery reviews` is the
command for questions about the store. The exit code is how a caller learns that
storing failed, because that is a failure of the run rather than a property of
the document.

## 6. Token economy

Efficiency is a design constraint here, not a nice-to-have. Structured review
competes against a prose paragraph that costs nothing to produce, so the contract
has to pay for itself.

### 6.1 Budgets

JSON output, except `prime`, which is prose. These are ceilings, enforced by
golden-file tests that fail when a command grows past its budget — a limit
nothing measures is a limit that erodes.

| Call | Budget | Frequency |
| --- | --- | --- |
| `prime` | 250 | Once per session, often pinned into a system prompt |
| `prime --profile=NAME` | 250 + 25 + the profile | Once per session, when an orchestrator names one |
| `prime --list` | 10 + 40 per profile | Rare; operator discovery, never a reviewer's call |
| `describe` | 850 | Once per session that writes a review |
| `describe --lens=NAME` | 350 each | Only on uncertainty or a failed check |
| `describe --list` | 380 | Rare; discovery and the unknown-lens error |
| `schema` | 1,000 | Rare; machine consumers only |
| `schema --annotated` | 5,000 | Rare; codegen only |
| `submit-review`, clean | 80 | Every attempt |
| `submit-review`, clean, no repository | 140 | Common for a document with no anchors to verify; an anchored document run outside a repository now fails instead of landing here — see [§2.3.1](#231-verifying-anchors) |
| `submit-review`, uncommitted work (exit 3) | 153, flat | Fails immediately with exactly one diagnostic, flat regardless of how many anchors diverged — measured equal at one diverged anchor and at ten, replacing the old per-anchor row this ceiling used to carry |
| `submit-review`, per diagnostic | 60 | Every failed attempt |
| `reviews` | 60 + 150 per row | Rare; only where a store is used |
| `reviews --failed` | 60 + 120 per row | Rare; diagnosing a reviewing agent |
| `reviews --list` | 60 + 25 per repository | Rare; discovery |
| `reviews --content` | none | Returns caller-authored documents |
| `collect-reviews --format json` | 80 + 40 per submission + 60 per comment + 60 per diverged anchor | Rare; only when an orchestrator has run more than one reviewer against one ref |
| `collect-reviews --format markdown` | unbudgeted | Rare; human reading, or embedding somewhere a human reads it |

These are higher than the text format they replaced. Measured over a
realistic write-submit-review-fix cycle it is about **2.1x**; a clean `submit-review`
alone is the worst case at roughly **4x**, 14 tokens to 63, because there is a
floor to how small a JSON object can be and almost nothing to say. That is the
price of having one renderer rather than two. Two implementations of the
same result disagreed about what a run found three separate times, each
disagreement invisible to a green suite because no fixture pinned the shape
that differed; one of them let an author-supplied comment id forge diagnostic
lines that were never in the document. A format a caller must parse cannot do
that, and no second implementation can drift from it. `prime` is unchanged,
because it was never rendered — it is prose written to be pinned into a prompt,
and so is the `summary` field of `describe`.

Verification adds wall-clock, not tokens, when a repository can answer and
every anchored file matches `ref`: the `verification` block is the same size
whether zero anchors needed checking or several. Because it runs by default,
that cost is paid on every loop — object lookups against a local database,
plus, once per anchored file when `ref` is `HEAD`, a working-tree read and
comparison to decide whether `ref` still applies
([§2.3.1](#231-verifying-anchors)). Outside a repository the shape changes and
the cost stops being free: `SkipAll` stops three checks for one reason, and
`skipped` prints that reason once with all three check names beside it —
real content the clean in-repository case never has to print. That is the
no-repository row above, and it is measured, not guessed, at 115 —
trimming what `SkipAll` reports would shrink the number,
but a run that verified nothing would then look like a run that verified
everything, which [§2.3.1](#231-verifying-anchors) rules out on purpose. A
dirty checkout no longer costs content that scales with the number of
diverged anchors: [§2.3.1](#231-verifying-anchors)'s precondition reports
the whole thing once, in one diagnostic, the moment verification's own pass
finds the reviewed state is not a commit — discarding the rest of what that
pass found, and running before structural checks and before advisories. That
is the uncommitted-work row above, measured at 153 — flat whether one
anchor diverged or ten, which is
the whole point: the precondition reports once for the document rather than
once per anchor, so the cost cannot grow with how many anchors turned out to
be wrong about the same thing. It does not carry forward the old per-anchor
figure: the old row measured `verification.unverified` entries accumulating
inside an otherwise-passing run, a shape that can no longer occur, and
reusing its number for a structurally different response would have been
exactly the kind of number this section warns against inventing.

A profile's body is unbudgeted, like `reviews --content` and for the same
reason: it is content the caller wrote, and a ceiling on it would be this tool
rationing an operator's own words back to them. The seed profiles in
`profiles/` measure 210–290 tokens each, which is the norm rather than the
rule: a primed reviewer costs about 525 tokens instead of 250, paid once, in
exchange for prompt text the orchestrator would otherwise have written by hand.
A profile costing more than `prime` itself is worth re-reading before it is
worth installing. What is budgeted is everything
around it — `prime` holds at 250 whether a profile is named or not, the frame is
fixed at about 25, and `description` is capped at 120 characters so `--list`
cannot quietly become a second copy of every profile
([§2.1.2](#212-the-profile-file)). `prime`'s own byte-identity regardless of
whether a profile source holds anything, works, or is never called at all is
pinned in `internal/cli/prime_test.go` against a `profileSource` that panics
if touched — a stronger guarantee than a golden file over one fixed directory
state would give, since it fails at the call site rather than only on a
byte that happened to differ. The golden-file test pins `prime`'s own text
against the embedded `prime.txt`.

`prime --list`'s row is indented JSON ([§5.1](#51-one-format)), not compact:
measured against the real renderer with the eight profiles shipped in
`profiles/`, the envelope is about 5 tokens and each row averages about 35,
for roughly 285 total. The ceiling rounds both up with headroom, to 10 and
40 per profile.

The two that matter most are `prime` and clean `submit-review`, because they are paid
on every single loop.

Clean `submit-review` holds at 80 even though every clean run now writes to a store,
because the result object says nothing about the store. A draft that reported
where each review landed measured about 140: an absolute path is a store root, a
repository name, a 40-character SHA, and a filename, and it arrived on the one
path every loop pays. Moving that question to `loam-refinery reviews`, where it
is asked deliberately and rarely, gave the 60 tokens back.

That is the shape of this whole table. A cost belongs on the call that wants it,
not on the call that happens to be running.

Each `reviews` row pays for identity twice. A ref is 40 characters and a
digest is 64; both are fields on the row, and both appear again inside the
mandatory absolute `path` it also carries — store root, repository name,
ref, and filename. Measured against a real store that shape costs about 138
tokens a row; `--failed` drops the digest, and drops the ref segment of the
path too when the input had none to resolve, which measures about 105 with
a ref present and about 85 without one. The ceilings above round those up
with headroom, to 150 and 120. What keeps a cost this size acceptable is
that `reviews` is a rare, deliberate call rather than one paid on the
write-submit-review-fix loop `submit-review` sits on; `--limit`, defaulting to 10, is
the only brake on it.

`reviews --content` is the one call with **no ceiling**, and that is stated
rather than invented: it returns review documents the caller wrote, at whatever
size the caller wrote them. `--limit` is the control that exists instead, and
the default index form is what keeps the common query cheap.

`collect-reviews --format json`'s row is measured, not guessed, against the
real binary and a real store: an empty envelope costs about 73 tokens, each
additional submission's shape — `profile`, `ordinal`, `verdict`, an optional
`superseded_by` — costs about 27 to 34, and each additional comment's shape —
`id` and `profile` — costs about 46 to 52, both rounded up with headroom.
Neither figure prices the free-text content riding along with it: a
submission's `summary` and a comment's `body`, `suggestions`, and `anchors`
are unbudgeted, the same exemption `reviews --content` gets above, for the
same reason — it is content the caller wrote, and charging a ceiling against
it would be rationing the caller's own words back to it. Per-diverged-anchor
reuses the standing 60-token ceiling for one diagnostic-shaped entry — a
check name, a comment, and a reason — rather than a fresh measurement,
because `head_check.diverged`
([docs/features/combined-reviews.md §4.3.1](features/combined-reviews.md#431-the-head_check-shape))
is the identical shape that ceiling was already measured against.
`--format markdown` stays unbudgeted outright: it is read by a human once, or
piped into a comment body a human reads once, never paid on a loop the way
every other row here is.

The largest saving is not in any row of that table — it is in **not repeating
it**. A validator that reports one problem at a time turns a document with four
mistakes into four write-submit-review cycles, each paying the model's full cost of
re-reading its own draft and re-emitting it. Reporting everything findable in one
response ([review-document.md §11.4](review-document.md#114-partial-documents))
is worth more than every per-command budget here combined, which is why no check
is allowed to gate another.

The two `schema` figures are large on purpose and are ceilings rather than
targets. `schema --annotated` carries the descriptions every field lens renders
from, so it is necessarily at least the sum of those entries — a small annotated
schema would mean thin lenses, which is the opposite of what it is for. Both are
rare, machine-facing calls that no loop pays.

Budgets are **per entry**, not per command, for `describe --lens`. That is what
keeps the ceiling meaningful as the registry grows: a registry of 200 entries
costs a caller no more than a registry of 20, because a caller reads one entry.
Only `describe` default and `--list` scale with registry size, which is why the
default summary is specified as a fixed shape rather than a walk of everything
registered.

### 6.2 Why progressive disclosure

The obvious design is one command that prints the whole contract. Priced out
against a session that writes one review and gets two things wrong:

| | Monolithic | Progressive |
| --- | --- | --- |
| Learn the tool | 4,000 | 250 (`prime`) |
| Learn the contract | — | 600 (`describe`) |
| First submit-review | 15 | 15 |
| Two failed checks | re-read 4,000 | 500 (two lenses) + 15 pointer |
| Second submit-review | 15 | 15 |
| **Total** | **~8,030** | **~1,395** |

The gap widens with every additional attempt, because the monolithic path has
nothing smaller to re-read than everything. Progressive disclosure only works if
the small calls are genuinely sufficient, which is why a check lens is specified
as self-contained ([§2.2.2](#222-entry-content)) — a lens that sends the reader
onward to the full contract has spent tokens to save none.

### 6.3 Time, not just tokens

The same structure cuts wall-clock. A monolithic contract must be read before the
first attempt, serializing learning ahead of work. Under progressive disclosure
the model writes immediately after `prime` and `describe`, and pays for detail only
on the branch that actually needs it. Failed attempts get cheaper too: recovery
is a lookup keyed by a name the tool already printed, not a re-read and a
re-derivation.

This is also why `submit-review` output is terse and check names are stable. A stable
name is cacheable — across a session, and across the agent's own memory of what
`broad-scope-alone` meant last time. Prose diagnostics are not.

## 7. Implementation notes

### 7.1 Package layout

```
cmd/loam-refinery/main.go              flag parsing, wiring, exit codes
internal/cli/                     subcommand implementations
internal/cli/interfaces.go        validator, renderer
internal/cli/testdata/            golden files for describe, --list, and prime output
internal/review/review.go         document types, enums, priority bands
internal/schema/schema.go         //go:embed of the schema, draft compilation
internal/schema/review.schema.json
internal/validate/                assembles the three check tiers into one result
internal/structural/              hard checks
internal/verify/                  anchor claims checked against the repository
internal/advisory/                advisory registry and implementations
internal/advisory/interfaces.go   advisory
internal/entry/                   entry registry, namespaces, alias resolution
internal/entry/interfaces.go      provider
internal/entry/schema.go          field:* provider, reads the annotated schema
internal/entry/checks.go          check:* provider, reads the check registries
internal/entry/topics/            topic:* entries, //go:embed of hand-written md
internal/collect/                 collect-reviews's merge semantics, qualified ids
internal/render/                  the json and markdown renderers
internal/config/                  locations, the config file, precedence
internal/profile/                 the profile directory, frontmatter, bodies
internal/store/                   stored files, the run database, queries
internal/store/sql/schema.sql     //go:embed of the SQLite schema and migrations
internal/store/sql/query.sql      the queries, compiled by sqlc
internal/store/sqlc/              generated by sqlc; committed, never edited
internal/store/interfaces.go      clock, repository namer
```

Interfaces live in each package's `interfaces.go`, defined at the consumer.
Constructors take dependencies explicitly; the graph is wired in `main`.

`internal/render`'s two formatters share one constraint that makes carrying
two of them safe: exactly one function decides what `collect-reviews` found,
and both renderers are downstream of its return value, never of the store —
neither computes anything the other does not already have
([docs/features/combined-reviews.md
§8.3.1](features/combined-reviews.md#831-one-structure-two-renderers)).

### 7.2 Advisory registry

An advisory is a small value: a name, a one-line summary, a title, a body, and a
check function over the parsed document. The summary is not reused by `prime` —
`prime` stays fixed and deliberately small ([§2.1](#21-prime)) and cannot absorb
a per-check line — nor by `describe --list`, which prints names alone, no bodies
([§2.2.5](#225---list)). Nothing renders it today; it is bookkeeping on the
value, not a rung on the disclosure ladder. `describe --lens` explains a check
from its title and body alone ([§2.2.2](#222-entry-content)). The registry is a
slice built at construction, not package-level state — so tests can build a
registry holding one advisory.

Adding an advisory means adding one file and one registry entry. The `check:*`
entry provider reads from that registry, so a new advisory becomes explainable
via `describe --lens` the moment it is registered, with nothing else to wire
up. There is no longer a flag to make one configurable instead — advisories
always run and are always reported
([§2.3.1](#231-verifying-anchors)).

### 7.3 Dependencies

Keep the tree shallow — this binary is invoked in tight loops.

- `github.com/santhosh-tekuri/jsonschema/v6` for draft 2020-12 validation
- `modernc.org/sqlite` for the run database — pure Go, no cgo, so cross
  compilation and `go install` keep working. It is by a wide margin the heaviest
  thing here, and the reasoning for accepting it is in
  [config.md §4.5.3](config.md#453-why-sqlite): it buys queries that stay fast
  as a store grows, and it costs binary size rather than startup, which is the
  budget that matters for a binary invoked in loops.
- Standard library `flag` with `flag.NewFlagSet` per subcommand. A CLI framework
  is not warranted for four subcommands, and `prime` already serves the
  agent-facing help role that a framework's generated help would cover.
- testify for tests

Build-time tools are not dependencies of the binary and live in
`internal/tools/tools.go` behind the `tools` build tag, installed by
`make tools` and run by `make generate`: `moq`, `gofumpt`, and now `sqlc`, which
compiles `internal/store/sql/*.sql` into typed Go. Its output is committed, so a
build never needs it ([config.md §4.5.4](config.md#454-sqlc)).

### 7.4 Testing

Per project standards: `t.Parallel()` on every test and subtest, `t.Context()`,
`slog.New(slog.NewJSONHandler(io.Discard, nil))` for loggers, moq-generated
mocks in `moq_test.go`.

Testdata carries a corpus of review documents under `internal/advisory/testdata/`
and `internal/structural/testdata/`, each paired with its expected diagnostic
set. Every check needs a passing case and a failing case. Golden files cover the
renderer.

`collect-reviews`'s two renderers are pinned by three further tests, run
against the same fixture data the JSON renderer's own golden tests already
use: **parity**, that the qualified-id headings the Markdown renderer emits
are the identical set `comments[].id` names in the JSON form; **fidelity**,
that unescaping a Markdown body recovers the JSON `body` byte for byte; and
**forgery**, that a body or `code` excerpt engineered to look like tool
structure — a fake heading, a fence-breaking run of backticks — renders as
inert prose rather than as structure
([docs/features/combined-reviews.md
§8.3.3](features/combined-reviews.md#833-the-test-that-pins-it)). Parity is
the test that would have caught this section's own war story: two renderers
disagreeing about what a run found becomes a failing assertion instead of an
invisible drift.

## 8. Future considerations

Deliberately deferred, recorded so the design leaves room for them.

- **Diff-aware verification.** Verification confirms an anchor points at something
  real; it does not confirm it points at something *changed*. A base ref
  at the document root, plus the changed-path set derived from it, would catch
  the reviewer that wandered off the diff into untouched code. Deferred because
  it needs a `base` field on the document and a decision about whether commenting
  on unchanged code is an error at all — sometimes the bug is in what the change
  failed to touch.
- **Content-aware verification.** Superseded by a narrower, separate proposal:
  a **provenance hash**, computed by `loam-refinery` itself at ingest time —
  never supplied by the reviewer, never a document field, and not the
  `digest` column [config.md §4.5.1](config.md#451-what-it-holds) already
  uses for the submitted document's filename; same word would name two
  unrelated things, so this one is named differently on purpose — recorded
  alongside a passing review in the store
  ([config.md §4](config.md#4-the-store)), so a later consumer can tell
  whether the anchored code has drifted since the review passed, rather than
  merely that the line still exists today. This answers a different question
  from verification and must never be collapsed with it: verification asks
  whether the review is internally consistent with the ref it names, at the
  moment `submit-review` runs ([§2.3.1](#231-verifying-anchors)); a provenance
  hash asks, after the fact and from the store, whether the world has moved
  on since. Tracked as its own proposal for that reason, and touches the store,
  not the review-document schema. The same mechanism is also the only way to
  close a narrower gap `collect-reviews` leaves open: two submissions pinned
  to the same `HEAD` SHA can still have read different working trees at
  their own two moments, and a provenance hash recorded at ingest is what
  would let a later consumer tell whether they in fact saw the same one.
- **Combined reviews.** Specified in
  [docs/features/combined-reviews.md](features/combined-reviews.md):
  `collect-reviews`, not `merge`; grouped by profile, not by slug.
- **Cross-profile convergence on `collect-reviews`.** Two comments from
  different profiles that anchor the identical `file`/`line`/`end_line` — the
  same identity `duplicate-anchor` already checks for within one document
  ([review-document.md §11.3](review-document.md#113-advisory-checks--soft))
  — are independent evidence pointing at the same place.
  `collect-reviews` never fuses comments across profiles
  ([docs/features/combined-reviews.md
  §5.2](features/combined-reviews.md#52-grouping-by-profile-not-by-slug)), so
  surfacing that convergence as its own signal, without fusing the comments
  it comes from, is real, unbuilt value — deferred because "surface" needs
  its own shape decision, not because it lacks one.
- **A non-`HEAD` `collect-reviews` ref whose repository has since rewritten
  history.** A force-pushed branch can leave a `ref` that once resolved no
  longer resolving. `collect-reviews` inherits `reviews`'s own posture — it
  resolves nothing — and whether that is the right answer for a *combined*
  view is not settled.
- **A `--profile=NAME` filter on `collect-reviews`.** A natural complement to
  `--ref`, not included because nothing yet demonstrates it is wanted over
  reading one submission's own stored file directly.
- **A way to see only "current" `collect-reviews` submissions without
  post-filtering.** A caller that wants just the current opinion per profile
  has to filter out everything carrying `superseded_by` itself
  ([docs/features/combined-reviews.md
  §5.3.2](features/combined-reviews.md#532-marking-not-deleting)). Whether
  that deserves a flag is not yet known.
- **Suggestion selection.** A `loam-refinery pick` subcommand that, given a review and
  a ceiling on effort and blast radius, emits the subset of suggestions worth
  taking now — e.g. `--max-effort=small --max-scope=file` to assemble a
  low-risk cleanup pass, spilling everything wider into a deferred list. This is
  the payoff for keeping `effort` and `scope` as separate ordinal fields; a
  single collapsed "size" could not express the query.
- **Knowledge base.** A `kb:*` namespace of embedded guidance beyond the schema:
  how to write a body that carries a rationale, how to calibrate priority for a
  given language or change type, what actually counts as a `security` finding,
  worked examples per category. This is the reason `describe` is named for the
  act rather than for a document, and the reason entries come from providers
  ([§2.2.3](#223-the-entry-registry)) — a knowledge base is a fourth provider
  registering `kb:*` entries. No new subcommand, no new flag, no change to the
  `lenses` field, and `--list` grows to show it.

  Three things have to hold when it lands. Bare-name resolution must stay
  predictable, which is what the namespace precedence and the ambiguity error in
  [§2.2.1](#221-lens-names) are for. Per-entry budgets must survive contact
  with prose written by hand rather than generated from a schema — a knowledge
  base is exactly where 350-token entries quietly become 900-token ones, so the
  budget tests apply to `kb:*` from its first entry. And `describe --list`'s own
  ceiling ([§6.1](#61-budgets)) has little room left to grow into: measured
  after the `profile` root field
  ([review-document.md §3](review-document.md#3-root-object)) landed alongside
  everything else this branch added to the index, `describe --list` is 372
  tokens against its 380-token ceiling — 8 left, room for one more name before
  it needs raising, not three. A `kb:*` rollout that registers more than a
  single entry at once should expect to renegotiate that ceiling rather than
  assume the existing one still fits.

  A project-supplied knowledge base — conventions read from the repository rather
  than compiled into the binary — is the obvious follow-on and would be another
  provider. It is deliberately not specified here, because it breaks the
  stateless-and-offline principle in a way that deserves its own design.

  Reviewer profiles ([§2.1.1](#211-reviewer-profiles)) are the other half of
  this and are specified, so the split is worth stating before `kb:*` exists.
  A profile is *who is reviewing*: operator-authored, machine-local, pushed
  into context at session start, unbudgeted. `kb:*` is *what good looks like*:
  compiled into the binary, identical on every machine, pulled on demand,
  budgeted at 350. They never merge. A profile that starts explaining what a
  `security` finding is has drifted into `kb:*`'s job and should be a lens
  everyone gets; a `kb:*` entry that starts naming one team's conventions has
  drifted into a profile's and should not ship in the binary.

- **Composing profiles.** `--profile` takes exactly one name. Comma-separated
  composition — a `go` profile plus a `security` profile — is deferred, not
  refused: the mechanism is nearly free, since `splitNames` already exists, but
  the semantics are not. Two profiles that disagree need a rule for who wins,
  and two profiles that agree need one for not saying it twice, and neither
  question has an answer worth guessing at before anybody has written two
  profiles that want composing.

- **Project-supplied profiles.** A profile read from the repository under review
  rather than from the operator's home directory. Deferred for the same reason
  as a project-supplied knowledge base, and one more: a profile in a repository
  is a profile the code under review can edit, and a reviewer whose instructions
  arrive in the diff it is reviewing is not a reviewer.
- **MCP adapter.** A `loam-refinery mcp` subcommand serving the same command layer over
  stdio, for hosts without a shell. The internal packages are structured to make
  this an adapter rather than a rewrite; it is not built now because MCP tool
  schemas cost context on every session whether used or not.
