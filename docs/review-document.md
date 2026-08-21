# The review document

Format specification. Version 0.2 (draft).

For the tool that reads and checks these documents, see [cli.md](cli.md).

## 1. What it is

A review document is a single JSON object holding one agent's complete review of
one change. It is self-contained in the sense that matters: it carries findings,
never the code they are about. It does name a commit — anchors are meaningless
without one ([§5.1](#51-refs)) — but a SHA is a pointer anyone can resolve, not
content the document is transporting, and there is no reference to a diff, a pull
request, or a forge anywhere in the format.

The format exists so that review feedback can survive the trip between agents. A
subagent asked to review a change can otherwise return a verdict that contradicts
its own findings, comments with no location, a single vague suggestion with no
stated tradeoff, or a paragraph that says nothing — and the orchestrator
consuming that has no way to tell, and no stable way to refer to an individual
finding. Fixing that is what every field below is for.

The authoritative schema is embedded at `internal/schema/review.schema.json`
(JSON Schema draft 2020-12). This document describes it normatively; the file is
the implementation.

**Agents should not read this page.** It is the complete reference, written for
people designing against the format. An agent writing a review gets the same
material in the size it needs from `loam-refinery describe`, and any single section here
from `loam-refinery describe --lens=<name>` — every field name and every check name below
is a valid lens. See [cli.md §2.2](cli.md#22-describe).

## 2. Concepts

**Review document** — a single JSON object representing one agent's complete
review of one change.

**Verdict** — the review's overall disposition. One of `approve`,
`request_changes`, `comment`.

**Comment** — one finding. Carries a reviewer-authored ID, a priority, a
category, a body, zero or more anchors, and zero or more suggestions.

**Comment ID** — a short kebab-case slug with a numeric suffix, e.g.
`missing-context-2`. See [§4](#comment-ids).

**Anchor** — where a comment applies: a repository-relative file path and
optionally a line or line range, read at the document's `ref`.

**Ref** — the commit SHA every line number in the document is meaningful against.
One per review.

**Suggestion** — one proposed way to address a comment, with its level of effort,
its blast radius, its upsides, and its downsides. A comment may carry several
competing suggestions.

**Advisory** — a soft diagnostic raised against a document. Named, reported,
non-fatal. See [§11.3](#113-advisory-checks--soft).

## 3. Root object

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `version` | string | yes | Format version. Always `"1"`. Rejected if anything else. |
| `verdict` | enum | yes | `approve`, `request_changes`, `comment` |
| `summary` | string | yes | 1–3 sentence overall assessment. 30–1500 chars. |
| `ref` | string | yes | Commit SHA the whole review was performed against, 40 lowercase hex. See [§5.1](#51-refs). |
| `comments` | array | yes | Comment objects. May be empty only when `verdict` is `approve`; otherwise at least one. Enforced by the schema — see below. |
| `profile` | string | no | The reviewer profile ([cli.md §2.1.1](cli.md#211-reviewer-profiles)) this review was written under, if any — the same `NAME` as `prime --profile=NAME`. Caller-authored and unverifiable, like `id`. Shape constrained by `profile-format` ([§11.1](#111-structural-checks--hard)); honesty is not. Read by `loam-refinery collect-reviews` ([features/combined-reviews.md](features/combined-reviews.md)) for attribution. |

**`profile`'s rollout constraint.** `additionalProperties: false` means an
old binary's copy of this schema rejects any document naming a property it
does not know. The schema change that adds `profile` therefore has to reach
every binary an orchestrator might run a reviewer against *before* any
`prime.txt`, profile file, or prompt teaches a reviewer to emit `profile` at
all — a new document field reaching an old binary fails the whole
submission, and the reviewer's contract-correct response is to delete the
field it does not recognize and resubmit, silently discarding attribution
for a review that was otherwise sound. The failed run still gets recorded
and shows up in `reviews --failed` as a false signal that this reviewer
produced a bad review, rather than as the rollout-ordering problem it
actually is. This is not unique to `profile` — every future field this
format adds carries the identical exposure, since weakening
`additionalProperties: false` would give up the unknown-field protection
this section exists for.

**`comments` may be empty only for `approve`.** The schema expresses this
conditionally rather than leaving it to prose:

```json
{
  "if":   { "properties": { "verdict": { "enum": ["request_changes", "comment"] } },
            "required": ["verdict"] },
  "then": { "properties": { "comments": { "minItems": 1 } } }
}
```

A `request_changes` with nothing to change, or a `comment` verdict with nothing
to say, is not a review with a quality problem — it is a review that failed to
say anything at all, and no consumer can act on it. That belongs in the schema,
where it is caught once and needs no separate check name.

**Unknown fields are rejected.** Every object in this format sets
`additionalProperties: false`, here and in every object below.

This is the single most valuable thing the schema does. The failure mode this
format exists to catch is the error that reads as correct, and a misspelled field
is exactly that: write `end-line` instead of `end_line` and a permissive schema
drops it silently, leaving a review that validates clean while claiming a
one-line span the reviewer never meant. Nothing downstream can detect that. A
loud rejection costs one retry; a silent drop costs a wrong anchor that survives
every other check in this document.

The cost is that a consumer cannot stash its own metadata on a review. That is
deliberate. A pipeline that needs to carry provenance, tracking IDs, or its own
state should **wrap** the document rather than decorate it:

```json
{ "review": { "version": "1", ... }, "pipeline": { "attempt": 2 } }
```

The review stays exactly what `loam-refinery` validates, and the wrapper stays exactly
what the pipeline owns. No extension namespace is reserved, because a reserved
namespace is a slower way of arriving at the same wrapper with more ambiguity on
the way.

`loam-refinery`'s own review store takes the third option and keeps its metadata
out of the document entirely, in a database beside the files
([config.md §4](config.md#4-the-store)). A stored review is therefore still a
review document, byte for byte, and still validates. Where a consumer controls
its own storage that is the better answer than either; the wrapper above is for
when one object has to carry everything.

## 4. Comment object

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `id` | string | yes | Reviewer-authored identifier. See below. |
| `priority` | integer | yes | 1–10. See [§8](#8-priority) |
| `category` | enum | yes | See [§9](#9-category) |
| `body` | string | yes | The finding and its rationale. 20–4000 chars. |
| `code` | string | no | An illustrative excerpt of the problematic code as it stands. Soft. See [§6.1](#61-code-before-and-after). |
| `anchors` | array | yes | Zero or more anchor objects. See [§5](#5-anchor-object). |
| `suggestions` | array | yes | Zero or more suggestion objects. See [§6](#6-suggestion-object). May be empty. |

### Comment IDs

An ID is a short kebab-case slug describing the *kind* of finding, followed by a
numeric suffix:

```
missing-context-2
unchecked-error-1
stale-doc-comment-3
```

Pattern: `^[a-z][a-z0-9]*(-[a-z0-9]+)*-[1-9][0-9]*$`. Slug portion ≤ 48
characters, at most 5 words.

The slug is the **grouping mechanism**. Findings of the same kind share a slug
and differ only in suffix, so a consumer can collapse `missing-context-1` through
`missing-context-4` into one theme without reading four bodies. The suffix makes
each one individually addressable — an orchestrator can say "resolve
`missing-context-2`" and both sides know exactly which finding is meant.

The reviewer invents the slugs. There is no controlled vocabulary; naming the
finding is part of the reviewer's job, and a well-chosen slug carries real
information. IDs must be unique within a document
([§11.1](#111-structural-checks--hard)), and suffixes within a slug should run
contiguously from 1 ([§11.3](#113-advisory-checks--soft)).

## 5. Anchor object

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `file` | string | yes | Repository-relative POSIX path. No leading `/`, no `..` segment. |
| `line` | integer | no | 1-indexed. Omit for file-level comments. |
| `end_line` | integer | no | 1-indexed, inclusive. Requires `line`. Must be ≥ `line`. |

`anchors` is a list because one finding can occur in several places — the same
unchecked error in four call sites is one comment, not four. Filing it once with
four anchors keeps the finding and its suggestions together. Architectural
findings that have no single location may carry no anchors at all.

Anchors describe where the *problem* is. Where a *fix* lands, and how far it
reaches, is the suggestion's `scope` ([§7](#7-scope--blast-radius)) — the two are
frequently different.

### 5.1 Refs

`file` and `line` do not identify anything on their own. `client.go:88` is a
claim about a moment in a repository's history, and a review that omits which
moment is unverifiable the instant anything moves — the line it meant is now
somewhere else, or gone, and nothing in the document records that.

`ref` supplies the moment, once, at the document root. Every anchor in the
document is read at that commit.

There is deliberately no per-anchor `ref`. A review is of one change at one
revision — that is what makes it a review rather than a collection of remarks —
and letting anchors disagree would buy a case that does not exist while
complicating every consumer: verification, deduplication, and merging would each
need to reason about which commit an anchor meant. A comment that genuinely needs
to talk about a second revision can say so in its `body`, where prose belongs.

**A `ref` is a full 40-character lowercase commit SHA. Nothing else.** Not a
branch, not a tag, not an abbreviation.

Branches and tags are rejected because they are names for a moving target. An
anchor recorded against `main` means whatever `main` happens to point at when
someone looks, which is not what was reviewed, and the document gives no way to
tell the difference. A ref that can go stale without changing is worse than a
missing one: it reads as precision.

Abbreviations are rejected for a smaller reason — they are unambiguous only
until the repository grows enough that they aren't. The saving is 33 characters
once per document, against a claim that is supposed to remain checkable
indefinitely.

Neither restriction costs a reviewer anything. `git rev-parse HEAD` produces the
correct value, and a reviewing agent is holding a checkout already.

**`ref` is required.** A review is of one change at one revision; that is what
makes it a review rather than a collection of remarks, and a document that does
not say which revision has not finished describing itself.

It was optional in earlier drafts, on the reasoning that not every review is of
a git repository. That bought very little and cost the center of the design.
Every other field here already assumes a repository — repository-relative POSIX
paths, 1-indexed lines read at a commit, an anchor defined as a claim about a
moment in a history. A document without a ref carries all of that machinery and
then withholds the one value that makes any of it checkable, which is the exact
shape of the failure this format exists to catch: well-formed, plausible, and
unverifiable by anyone, ever.

It is also what made the tool's central check optional. Verification is the only
tier that catches a hallucinated line number
([cli.md §2.3.1](cli.md#231-verifying-anchors)), it cannot run without a ref,
and a format that lets a document opt out of being checkable has made its most
important guarantee a matter of the author's choice.

The requirement is **unconditional** — not "required when an anchor carries a
line". A conditional rule would create a class of valid documents that cannot be
verified and cannot be stored, and the consumer of every such document has to
carry a branch for it. A review with no anchors at all is still a review *of*
something, and `git rev-parse HEAD` costs a reviewing agent nothing: it is
already holding the checkout.

Requiring it retires the `ref-missing` advisory, which existed only to warn
about the document class this now rejects outright. Per
[§11.5](#115-name-stability) it is removed rather than repurposed, and
`describe --lens=ref-missing` becomes an unknown-lens usage error.

This tightens format version `"1"` rather than introducing a `"2"`. The format
is a draft with no released consumers, and forking a version to carry a rule
that no document in the world was written against would spend the one
compatibility signal there is on nothing. A document that was valid before and
is not now is a document nobody has.

`loam-refinery` requires the `ref` to resolve before verifying anything. When it does
not, that is one `ref-unknown` error for the document, not one per anchor —
nothing downstream is learned by repeating it.

## 6. Suggestion object

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `summary` | string | yes | What to do, in one sentence. 10–500 chars. |
| `effort` | enum | yes | Level of effort — how much work. See below. |
| `scope` | enum | yes | Blast radius — how far it reaches. See [§7](#7-scope--blast-radius) |
| `pros` | array of string | yes | Why this option is worth taking. May be empty, but see [§11.3](#113-advisory-checks--soft). |
| `cons` | array of string | yes | What it costs or risks. May be empty, but see [§11.3](#113-advisory-checks--soft). |
| `code` | string | no | An illustrative excerpt of the code as this suggestion would leave it. Soft. No fences. See [§6.1](#61-code-before-and-after). |

A comment carries a *list* of suggestions because the useful case is offering a
consumer a choice. "Thread the context through" and "add a per-attempt timeout"
are both valid responses to the same finding with different effort and different
tradeoffs; a reviewer that presents both lets the orchestrator decide, and a
reviewer that presents one has quietly made that decision for it.

`pros` and `cons` exist for the same reason. A suggestion with no stated
downside is either trivially correct or under-examined, and the difference
matters. The schema permits empty arrays — a genuinely free fix has no cons —
but an advisory fires so the omission is deliberate rather than lazy.

### 6.1 `code`: before and after

`code` appears twice, and the two halves are a pair:

- On a **comment**, it excerpts the problematic code **as it stands**.
- On a **suggestion**, it excerpts that code **as that suggestion would leave
  it**.

Read together they show the change without anyone opening a file, and with
several suggestions the comment's excerpt is quoted once against several
possible afters — which is exactly the comparison an orchestrator choosing
between options wants to make.

Both are deliberately **soft**. Neither is a patch, a replacement, or a
location. They carry no line numbers, are matched against nothing, and are never
verified. Truncation is fine, an ellipsis in the middle is fine, and surrounding
context may be trimmed to whatever makes the point. Their only job is to make a
finding legible on its own.

The after-excerpt lives on the suggestion rather than the comment because
competing suggestions produce different code. A `line`-scope option rewrites the
call site; a `module`-scope option answering the same finding changes the
exported signature. One before, several afters.

Because they are soft, they carry none of an anchor's constraints. A comment with
four anchors may carry a single excerpt illustrating the shape of the problem at
all four. Nothing about `code` needs to be exact, so nothing about it can be
ambiguous — and nothing downstream may treat it as authoritative.

Both are optional, and often absent. A finding about a *missing* test has no
problematic code to quote; an architectural finding has nothing short enough to
be worth quoting. Where the change actually goes is described in `summary`, in
prose.

If a consumer ever needs a machine-appliable change, that is a different field
that does not exist yet. `code` must not be pressed into the role: a soft excerpt
that gets applied is a patch that silently corrupts a file.

### 6.2 Effort

An ordinal enum rather than a number. Effort estimates from a model are not
precise enough to justify a 1–10 scale; four anchored buckets are honest about
the resolution actually available.

| Value | Anchor |
| --- | --- |
| `trivial` | A line or two. No design decision, no new tests. |
| `small` | Contained in one function or file. Under an hour. |
| `medium` | Spans several files or needs a design choice. Under a day. |
| `large` | Structural change, migration, or new subsystem. |

## 7. Scope — blast radius

How far applying a suggestion reaches. A property of the **fix**, not of the
finding: it answers "if I take this option, what else has to be re-read,
re-tested, or re-approved?"

| Value | What changes | What else is affected |
| --- | --- | --- |
| `line` | A line or a few adjacent lines | Nothing beyond them |
| `block` | One function, method, or type body | Callers unaffected — signatures and behavior hold |
| `file` | Structure within one file | Package's exported surface unchanged |
| `module` | Several files, or a package's exported surface | Other packages must adapt |
| `project` | Architecture, conventions, tooling, or a public API | Repository-wide; likely needs its own change |

**Scope and effort are independent, and that is the point.** Changing one shared
constant is `trivial` effort with `project` blast radius. Rewriting a gnarly
parser internal is `large` effort with `block` blast radius. An orchestrator
deciding what to apply now, what to defer, and what to escalate needs both
numbers, and collapsing them into a single "size" throws away the distinction
that matters most:

| | narrow blast radius | wide blast radius |
| --- | --- | --- |
| **low effort** | apply it | cheap but far-reaching — check before applying |
| **high effort** | schedule it | escalate; this is its own change |

Scope lives on the suggestion rather than the comment because one finding can be
answered at several radii. "Unchecked error here" can be fixed with a `line`
change at this call site or a `module` change that makes the error unignorable at
the type level — same finding, different blast radius, and the orchestrator picks.

## 8. Priority

Integer, 1–10. Higher is more urgent. The bands below are the calibration
reference `loam-refinery prime` teaches; the value is stored as an integer so
consumers can sort, threshold, and merge across reviewers without mapping enums.

| Range | Meaning |
| --- | --- |
| 9–10 | Must fix before merge. The change is wrong or unsafe as it stands. |
| 7–8 | Should fix before merge. A real defect or design problem, not a preference. |
| 4–6 | Worth fixing. Does not block. |
| 1–3 | Optional. Preference or polish; the author may decline without justification. |

Reviewers are expected to use the range, not the endpoints. `priority-flat` fires
for a review that assigns every finding the same number, which is the signal that
the scale went unused. How high a reviewer sets its priorities is its own call.

### 8.1 Priority and category

Priority and category are independent fields. There is a convention connecting
them, and it runs in one direction only: some categories rarely justify blocking
a merge.

`correctness`, `security`, and `performance` can plausibly land anywhere on the
scale. A correctness bug can be trivial or fatal, and "the hot path is now
quadratic" and "this allocates unnecessarily" are both `performance`.

`testing`, `maintainability`, `documentation`, and `style` ordinarily sit below
the blocking band. A formatting complaint is not a reason to stop a change, no
matter how strongly the reviewer feels about it.

The format does **not** encode a ceiling per category. Any such table would be
inventing precision it does not have — there is no principled reason
`maintainability` would cap at 7 rather than 6, and a convention stated to the
digit reads as a rule that reviewers then argue with.

What the format notices is the one case that is hard to defend: a comment in the
second group filed at 9 or 10, claiming a change must not merge over a missing
godoc. That raises `priority-category-convention`
([§11.3](#113-advisory-checks--soft)) — a note that a convention was crossed, not
a finding that the reviewer was wrong. Undocumented behavior in a public API
genuinely can be worth blocking on, and a reviewer who means it should say so and
carry the advisory.

There is no floor. A `security` finding at priority 2 — theoretical, in dead code,
worth recording and not worth acting on — is fine. A tool that pushed security
findings upward would teach reviewers to file fewer of them.

## 9. Category

| Value | Meaning |
| --- | --- |
| `correctness` | Wrong behavior, logic errors, unhandled cases |
| `security` | Vulnerabilities, unsafe handling of untrusted input or secrets |
| `performance` | Avoidable cost in time, memory, or I/O |
| `maintainability` | Structure, naming, duplication, complexity |
| `testing` | Missing, weak, or misleading tests |
| `documentation` | Missing or incorrect comments, docs, or godoc |
| `style` | Formatting and convention adherence |

The first three can carry any priority; the rest conventionally stay below the
blocking band. See [§8.1](#81-priority-and-category).

## 10. Example

```json
{
  "version": "1",
  "verdict": "request_changes",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "The retry loop is sound, but the context deadline is not propagated to the downstream call, so a cancelled request keeps retrying until the attempt budget is exhausted.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 9,
      "category": "correctness",
      "body": "The retry loop calls c.do(context.Background(), req) rather than passing the caller's ctx. A cancelled or timed-out request will keep retrying for the full backoff schedule, holding the connection and ignoring the caller's deadline.",
      "code": "resp, err := c.do(context.Background(), req)",
      "anchors": [
        { "file": "internal/fetch/client.go", "line": 88, "end_line": 94 }
      ],
      "suggestions": [
        {
          "summary": "Pass the caller's context straight through to c.do",
          "effort": "trivial",
          "scope": "line",
          "code": "resp, err := c.do(ctx, req)",
          "pros": [
            "Cancellation and deadlines propagate immediately",
            "Nothing outside the loop body has to be re-read"
          ],
          "cons": [
            "Any caller relying on retries outliving the request context sees a behavior change"
          ]
        },
        {
          "summary": "Take a context on the exported Fetch method and thread it through every attempt",
          "effort": "small",
          "scope": "module",
          "code": "func (c *Client) Fetch(ctx context.Context, req *Request) (*Response, error)",
          "pros": [
            "Makes the deadline impossible to drop rather than merely fixing this one site",
            "Brings the package in line with the context-first convention used elsewhere"
          ],
          "cons": [
            "Breaks the exported signature; every caller of Fetch must be updated",
            "Larger blast radius than the defect strictly requires"
          ]
        }
      ]
    },
    {
      "id": "untested-cancellation-1",
      "priority": 7,
      "category": "testing",
      "body": "No test covers cancellation mid-retry, which is why the dropped context above went unnoticed. A regression here is silent — the code still returns a response, just later than the caller asked for.",
      "anchors": [{ "file": "internal/fetch/client_test.go" }],
      "suggestions": [
        {
          "summary": "Add a case that cancels the context after the first attempt and asserts the loop returns ctx.Err()",
          "effort": "small",
          "scope": "file",
          "pros": ["Pins the behavior that dropped-context-1 fixes"],
          "cons": ["Needs a controllable clock or an injected attempt hook to be deterministic"]
        }
      ]
    }
  ]
}
```

## 11. Validity

Every check name below is also a lens: `loam-refinery describe --lens=id-unique` explains
one check, in isolation, well enough to fix it. That is the intended path when a
review fails validation — the tables here are the index, not the explanation.

The format defines three tiers of check. They answer different questions and they
are not peers.

**Structural** checks ask *is this a document?* They catch objects that cannot be
consumed — malformed JSON, a duplicate comment ID, a path escaping the
repository. Always run, always hard.

**Verification** checks ask *is this document about anything real?* They catch
anchors that point at a file or a line that does not exist at the stated ref.
Answering requires the repository the review is about, so they run whenever one
is available and are skipped, visibly, when it is not. Hard when they run.

**Advisory** checks ask *is this a good review?* They surface quality signals — a
verdict that overshoots its findings, a suggestion with no stated downside, a
review where every comment is priority 9. Always run, never hard.

The middle tier exists because the other two could not absorb it honestly. A
comment anchored to a deleted file is not a malformed document, so it is not
structural; and it is not a matter of taste, so calling it advisory would be a
lie. It is a factual error about the world, and it is the one class of error a
reviewing agent produces most confidently — a hallucinated line number reads
exactly like a real one.

Advisories are information for the author or the orchestrator, not a style guide
being enforced. A reviewer that consciously files six high-priority findings is
not wrong; it should just know that's what it did. How a tool acts on each tier —
exit codes, `--strict`, which source of truth it accepts — is
[the CLI's concern](cli.md#4-exit-codes), not the format's.

### 11.1 Structural checks — hard

| Check | Rule |
| --- | --- |
| `document-unparseable` | The input is not a single JSON object: malformed JSON, a top-level value that is not an object, or a second value after the first. The one failure that stops the run — see [§11.4](#114-partial-documents). |
| `schema` | JSON Schema draft 2020-12 conformance against the embedded schema, including `additionalProperties: false` on every object. Reported with a JSON Pointer into the document. |
| `id-unique` | No two comments share an `id`. Duplicate IDs break addressing outright. |
| `anchor-range-ordered` | `end_line` requires `line` and must be ≥ it. |
| `anchor-path-safe` | `file` must be relative POSIX, with no leading `/`, no `..` segment, and no backslashes. |
| `ref-format` | `ref`, where present, matches `^[0-9a-f]{40}$` — a full lowercase commit SHA. Branches, tags, and abbreviated SHAs are rejected. Checkable without a repository. |
| `profile-format` | `profile`, where present, matches `^[a-z0-9]+(-[a-z0-9]+)*$` — the same grammar [cli.md §2.1.2](cli.md#212-the-profile-file) already defines for the filename a profile resolves to. Checkable without a repository, exactly like `ref-format`. Exists because `collect-reviews`'s qualified-id round trip ([docs/features/combined-reviews.md §6.1](features/combined-reviews.md#61-the-qualified-id)) depends on `profile` never containing a colon. |

See [§11.4](#114-partial-documents) — a structural failure does not stop the
other tiers from running. `document-unparseable` is the single exception, and
only because there is no document left for another tier to check.

### 11.2 Verification checks — conditional

Anchor claims checked against the repository the review is about. Skipped
entirely when none can answer — reported as `source: none` outside a repository
and `source: unavailable` when one could not be asked — and a skipped
verification is reported as skipped, never as a pass.

| Check | Rule |
| --- | --- |
| `ref-unknown` | The document `ref` does not resolve in the repository. Reported once, not once per anchor. |
| `anchor-file-missing` | `file` does not exist at the document `ref`. |
| `anchor-line-out-of-range` | `line` or `end_line` exceeds the file's line count at the document `ref`. |
| `anchor-worktree-diverged` | The anchored file's working-tree copy exists and differs from `ref`. Only checked when `ref` is `HEAD`. |

All four are errors, not advisories, though not in the same way: an anchor
that points nowhere makes its comment unactionable, and no amount of good
judgment elsewhere in the review repairs it. `anchor-worktree-diverged` is
still the one exception at the level of the **check** — it can only
withhold a verification, never wrongly deny one, so firing it is never
evidence the anchor is *wrong* ([cli.md §2.3.1](cli.md#231-verifying-anchors)
has the full argument) — but that no longer makes it non-fatal at the level
of the **document**. `loam-refinery submit-review` reads whether the
reviewed state is a commit at all off this tier's own result, and a
diverged anchor at `ref == HEAD` fails that precondition outright, at its
own exit code, discarding whatever else this tier found and running before
the structural and advisory tiers get a turn
([cli.md §2.3.1](cli.md#231-verifying-anchors)).
A working tree moving on does not change whether an anchor *resolves*: a
ref is an immutable SHA, so later commits cannot affect that. It changes
whether the anchor gets *checked* at all — which is exactly the state this
format calls unreviewable, not merely unconfirmed.

There is no flag to make any of the four non-fatal. A shallow clone, or a
commit never fetched because its branch was deleted, still fails a document
that carries anchors — the fix is a deeper fetch, not a command-line
option.

What they cannot check is whether the anchored line is the *right* line. A
comment about a nil check that anchors a correct line number in the correct file,
describing code that is not there, passes every check in this document.

`loam-refinery` does not close that gap, and should not try — deciding whether a
comment describes the code it points at means reading and judging the code, which
is the reviewer's job, not the referee's. What the format does is make the gap
**auditable by ordinary git tooling**. Because a ref is an immutable commit SHA
and an anchor is a path and a line span, every claim in a review resolves to an
exact object:

```sh
git show 4f2c1a9:internal/fetch/client.go | sed -n '88,94p'
git blame -L 88,94 4f2c1a9 -- internal/fetch/client.go
```

That is the whole payoff of the ref restriction. Anyone — a person, a later
agent, a CI job — can pull up precisely what the reviewer was looking at and
judge the comment against it, with no cooperation from `loam-refinery` and no state it
had to keep. A branch name would break this: the commands still run, they just
answer a different question than the one the reviewer was asked.

### 11.3 Advisory checks — soft

**Consistency**

Nothing here relates `verdict` to `priority`. A reviewer may approve a change
while filing a priority-10 finding, or request changes over a single 4 — the
verdict is a judgment about whether the change should land, the priorities are
judgments about individual findings, and reconciling them is the reviewer's job.
A tool that second-guessed it would be overriding the one call the reviewer was
actually asked to make.

| Advisory | Signal |
| --- | --- |
| `id-grouping` | Suffixes within a slug do not run contiguously from 1 (`foo-1`, `foo-3`), or a slug is used once with a suffix other than 1. |

**Substance**

| Advisory | Signal |
| --- | --- |
| `body-thin` | `body` under 60 characters after normalization. Meets the schema minimum but is unlikely to carry a rationale. |
| `vacuous-body` | `body` matches a low-signal phrase list (`lgtm`, `looks good`, `no issues`, `see above`, `consider refactoring` with nothing following) after normalization. |
| `suggestion-absent` | Comment at priority ≥ 7 with no suggestions. High-priority findings should propose a way out. |
| `suggestion-no-cons` | A suggestion lists no `cons`. Either the fix is free or the tradeoff went unexamined; saying which is the point. |
| `suggestion-no-pros` | A suggestion lists no `pros`. |
| `broad-scope-alone` | A comment's only suggestion has scope `module` or `project`. A wide blast radius offered without a narrower alternative gives the orchestrator no choice exactly where the choice costs most. |
| `broad-scope-no-cons` | A suggestion at scope `module` or `project` lists no `cons`. Reaching that far always costs something. |
| `summary-thin` | `summary` under 60 characters while the review has ≥ 3 comments. |

**Calibration**

| Advisory | Signal |
| --- | --- |
| `priority-category-convention` | A `testing`, `maintainability`, `documentation`, or `style` comment filed at priority 9–10 — claiming the change must not merge. Conventionally those categories stay below the blocking band ([§8.1](#81-priority-and-category)). |
| `priority-flat` | Four or more comments all at the same priority. Suggests the scale was not used. |
| `duplicate-anchor` | Two comments anchor the identical `file`, `line`, and `end_line`. Often one finding filed twice; if deliberate, consider one comment with two suggestions. |
| `duplicate-body` | Two comments have identical bodies after normalization. |
| `comment-flood` | More than 25 comments. Feedback at this volume is not actionable. |

### 11.4 Partial documents

**A failing check never suppresses another check.** Every check in every tier is
evaluated unless its own inputs are unusable, and skipping is per check and per
item — never document-wide.

The reason is arithmetic. A caller that gets one error, fixes it, and is handed a
second error has paid two full write-validate cycles for information that fit in
one response. A validator that reveals problems one at a time converts a single
request into as many requests as the document has mistakes, which is the precise
cost this format exists to avoid.

So a document that fails schema conformance is still checked as far as it can be:

| Situation | Behavior |
| --- | --- |
| `priority` is `12` | Out of range, but an integer. Priority-based checks still run on it. |
| `priority` is `"high"` | Wrong type. That one comment is skipped by priority-based checks; every other comment is still checked. |
| `comments` is not an array | No comment-level check can run at all. Document-level checks still do. |
| One anchor is malformed | That anchor is not verified. Every other anchor in the document is. |
| The JSON does not parse | Nothing can run. This is the only true stop. |

**Aggregate checks are the exception.** A check that reasons over a population —
`priority-flat` and `comment-flood` — runs only when that
whole population is well-typed. Evaluating "more than half of comments are
priority 8 or above" against the subset that happened to parse produces a
confident, specific, wrong claim, which is worse than no claim. Those checks are
skipped and the caller sees them again on the retry it was going to make anyway.

**A skipped check is reported as skipped**, never silently omitted and never
counted as a pass — the same rule verification follows ([§11.2](#112-verification-checks--conditional)).
A caller must be able to tell "this check found nothing" from "this check never
ran," because the two justify very different amounts of confidence.

### 11.5 Name stability

Check names are API. They appear in diagnostics, in the `lenses` an agent
reads and opens, and in whatever an agent remembers across sessions.
Renaming one breaks all of that silently, so names do not change; a check
that outlives its usefulness is removed, not repurposed.

There is no version negotiation beyond the `version` field, which exists so a
future consumer can recognize a format it does not understand and say so. It is
not a compatibility scheme, and nothing here promises one yet.
