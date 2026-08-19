# refinery

Code review between agents only works if the feedback can be trusted, compared,
and acted on — and there is otherwise no way to tell a real review from a hollow
one. `refinery` exists to set that bar: it defines what a review has to deliver
to be worth acting on, and tells you whether a given review clears it. The goal
is a review process that gets both more reliable and cheaper — where agents hand
work to each other, and to humans, without anyone re-reading everything in
between.

## Why

When a subagent reviews code and reports back to an orchestrator, the feedback is
prose. Prose degrades in specific, recurring ways: verdicts drift from findings,
anchors go missing, a single vague suggestion arrives with no stated tradeoff,
"LGTM" comes back as a substantive review. There is no cheap way to spot any of
that, and no way to refer to one finding among twelve.

`refinery` makes the contract explicit. A review is a JSON document: a verdict, a
summary, and comments that each carry an ID, a numeric priority, a category,
anchors, and competing suggestions — each with its level of effort, its blast
radius, and its pros and cons. An embedded JSON Schema enforces the shape, and
the tool teaches that contract to a model a piece at a time, on demand, instead
of front-loading it.

## It is not a linter

Checks come in two tiers, and they are not peers:

- **Structural** checks are hard — malformed JSON, duplicate comment IDs, an
  anchor escaping the repository. These make a document unusable, so they fail.
- **Advisory** checks are soft — a verdict that overshoots its findings, a
  suggestion with no downside listed, a review where every comment is priority 9.
  These are reported and **do not fail the run**.

Review quality is a judgment the caller owns. `--strict` promotes advisories to
errors for callers who want them to gate, but that's opt-in.

## Install

```sh
go install github.com/bobcob7/refinery/cmd/refinery@latest
```

## Quickstart

An agent asked to review code runs `refinery prime` — about 250 tokens teaching
the loop, not the contract — then `refinery describe` for the document shape. It
writes its review and checks it:

```sh
refinery validate review.json
```

```
VALID  6 comments, 3 advisories

advisory  suggestion-no-cons        dropped-context-1
          suggestion 1 ("Pass the caller's context straight through") lists no
          cons; state the tradeoff or say the fix is free
advisory  priority-flat
          all 6 comments are priority 7; the scale is not being used
advisory  id-grouping               missing-context-3
          slug "missing-context" has suffixes 1, 3; renumber contiguously

refinery describe --lens=suggestion-no-cons,priority-flat,id-grouping
```

Exit 0 — the review is usable, and the agent knows where it was sloppy. It does
not have to remember what `broad-scope-alone` means, or re-read the contract to
find out. The last line is runnable, and it is the only recovery path the agent
needs.

## Comment IDs

Every comment carries a reviewer-authored ID: a short kebab-case slug naming the
*kind* of finding, plus a numeric suffix.

```
missing-context-2
unchecked-error-1
stale-doc-comment-3
```

The slug groups. Four findings of the same kind share a slug, so a consumer can
collapse them into one theme without reading four bodies. The suffix makes each
individually addressable — an orchestrator says "resolve `missing-context-2`" and
both sides know exactly which finding is meant. There's no controlled vocabulary;
naming the finding is part of the reviewer's job.

## Comment shape

| Field | What it carries |
| --- | --- |
| `id` | `slug-N`, reviewer-authored and groupable |
| `priority` | 1–10. 9–10 blocks merge, 7–8 should fix, 4–6 worth fixing, 1–3 optional |
| `category` | `correctness`, `security`, `performance`, `maintainability`, `testing`, `documentation`, `style` — the last four conventionally stay below the blocking band |
| `body` | The finding and its rationale |
| `code` | Optional excerpt of the problematic code, as it stands |
| `anchors` | Zero or more file/line spans, each meaningful at a git `ref` — a list, because one finding can occur in four call sites |
| `suggestions` | Zero or more competing fixes, each optionally quoting the code as it would leave it. See below |

Suggestions are a list because the useful case is offering a choice. "Patch this
call site" and "make the deadline impossible to drop at the type level" answer
the same finding at different cost and different risk; a reviewer that presents
both lets the orchestrator decide, and one that presents a single option has
quietly made that decision for it.

## Effort and blast radius

Each suggestion carries two independent ordinals, and keeping them apart is the
design's main bet.

- **`effort`** — `trivial`, `small`, `medium`, `large`. How much work.
- **`scope`** — `line`, `block`, `file`, `module`, `project`. How far applying it
  reaches: what else has to be re-read, re-tested, or re-approved.

They come apart constantly. Changing one shared constant is `trivial` effort with
`project` blast radius. Rewriting a gnarly parser internal is `large` effort with
`block` blast radius. An orchestrator triaging a review needs both:

| | narrow blast radius | wide blast radius |
| --- | --- | --- |
| **low effort** | apply it | cheap but far-reaching — check before applying |
| **high effort** | schedule it | escalate; this is its own change |

`scope` sits on the suggestion rather than the comment because one finding can be
answered at several radii — patch the call site, or change the type so the
mistake stops being expressible. Same finding, different blast radius, and the
choice belongs to the caller.

## Anchors are checkable claims

An anchor says *this path, at this commit, has this line*. That is a factual
claim, and a hallucinated line number is indistinguishable from a real one by
inspection — well-formed, plausibly numbered, wrong.

So the document carries a `ref`: a full commit SHA, never a branch or a tag. A
branch name names a moving target, so an anchor recorded against `main` means
whatever `main` points at when someone looks — stale without ever changing, and
reading as precision the whole time.

`validate` checks that claim against the repository it is run in, by default and
with no flag — the reviewed commit is a SHA, so it resolves by object lookup even
if it is not checked out:

```
VALID  5 comments, 0 advisories  [anchors verified: 5 of 5]
```

Verifying by default rather than on request is deliberate. An opt-in check makes
the plain invocation the weak one, and the plain invocation is what gets run. Run
somewhere without a repository, verification is **skipped and reported as
skipped** — `[anchors unverified: not a git repository]` — because a run that
checked nothing must not look like a run that checked everything.

`refinery` confirms an anchor points *somewhere*; it cannot tell you it points at
the *right* line, and it does not try — judging that means reading the code,
which is the reviewer's job. What the SHA buys is that anyone else can:

```sh
git show 4f2c1a9:internal/fetch/client.go | sed -n '88,94p'
git blame -L 88,94 4f2c1a9 -- internal/fetch/client.go
```

Every claim in a review stays resolvable by ordinary git tooling, indefinitely,
with no cooperation from this tool and no state it had to keep.

## Commands

| Command | Purpose | ~Tokens |
| --- | --- | --- |
| `refinery prime` | The workflow: how to use the tool and when to reach for `describe` | 250 |
| `refinery describe` | The contract in summary — enough to write a review | 600 |
| `refinery describe --lens=NAME` | One field or one failed check, in full | 250 each |
| `refinery describe --list` | Every name the binary can explain | 120 |
| `refinery validate [path]` | Check a review (`-` or omitted reads stdin) | 15 clean |
| `refinery schema` | Minimal JSON Schema, annotations stripped — for machines | 400 |

## Progressive disclosure

No command prints everything it knows, because the tool competes against a prose
paragraph that costs nothing to produce. A contract that takes 4,000 tokens to
learn has already lost.

So the material is tiered, and callers climb only as far as the problem requires.
`prime` teaches the loop. `describe` gives the shape. `--lens` opens exactly one
entry — and the lens namespace includes **every check name**, so a failed
validation routes straight to its own explanation rather than back to the top of
the contract. Priced against one review with two mistakes, that's ~1,400 tokens
where a monolithic contract costs ~8,000, and the gap widens with every retry.

`describe` renders entries from a registry rather than printing a document, so
what the tool can explain is free to grow — a planned knowledge base of review
guidance arrives as `kb:*` entries under the same command, the same flag, and the
same per-entry budget.

`refinery schema` deliberately does *not* serve this path: it emits the minimal
grammar for machines that need to validate or generate types. A model reading
JSON Schema to learn what `priority` means is paying for structure to reach a
sentence. `prime` says so.

Full accounting in [docs/cli.md §6](docs/cli.md#6-token-economy).

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Structurally valid. Advisories may be present |
| 1 | Structurally invalid, or advisories present under `--strict` |
| 2 | Usage or I/O error |

Exit 1 means *revise the review*; exit 2 means *fix the invocation*. An agent must
be able to tell those apart without parsing prose.

## Non-goals

- Fetching pull requests, diffs, or any repository state
- Posting comments to GitHub or any other forge
- Judging whether a review's *claims* are correct — only whether the review is
  well-formed and internally consistent
- Acting as an MCP server (see [docs/cli.md](docs/cli.md#8-future-considerations))

## Documentation

- [docs/review-document.md](docs/review-document.md) — the review format: every
  field, the priority/effort/scope ladders, and what makes a review valid
- [docs/cli.md](docs/cli.md) — the tool: commands, flags, exit codes, output
  formats, and implementation notes
