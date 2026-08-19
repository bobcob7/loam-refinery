# The refinery CLI

Tool specification. Version 0.1 (draft).

For the format this tool reads, see [review-document.md](review-document.md).

## 1. Overview

`refinery` accepts a review document and answers one question: **is this review
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
- A `prime` command that teaches the workflow in one small call
- A `describe` command that discloses the contract progressively, on demand
- Diagnostics addressed by comment ID, so feedback is directly actionable

Explicitly out of scope:

- Fetching anything over a network — PRs, diffs, remote refs
- Posting to GitHub or any forge
- Reading file *contents*. Verification asks a checkout whether a path and line
  number exist at a ref; it never looks at what is on the line, because judging
  that would make `refinery` a reviewer rather than a referee.
- Assessing the *truth* of a review's claims. `refinery` cannot know whether
  "this nil check is missing" is correct; it only knows the comment is anchored,
  prioritized, substantive, and honest about what its suggestions cost.
- Storing or aggregating reviews across runs

### Design principles

1. **Offline, and read-only about the repository.** No network, ever. `refinery`
   reads the repository it is run in to verify anchors
   ([§2.3.1](#231-verifying-anchors)), and reads only what it takes to answer
   "does this path exist at this ref, and does it have this many lines?" It never
   reads a diff, never forms an opinion about code, and never carries repository
   content into or out of a review.
2. **The schema is the documentation.** Field descriptions and examples live in
   the annotated schema, and `describe` renders from it, so explanation cannot
   drift from enforcement.
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
   output.

## 2. Commands

```
refinery prime
refinery describe [--lens=NAME[,NAME...]] [--format text|json]
refinery validate [path] [--strict] [--warn-only=…] [--disable=…]
                         [--format text|json]
refinery schema   [--annotated]
refinery version
```

Global: `--help` on any subcommand.

The four content commands form a ladder, cheapest first. A caller climbs only as
far as its current problem requires:

| Call | Answers |
| --- | --- |
| `prime` | How do I use this tool at all? |
| `describe` | What does a review have to contain? |
| `describe --lens=NAME` | What exactly does *this* field or *this* failed check want? |
| `schema` | Give me the machine-readable grammar. |

### 2.1 `prime`

Teaches the **workflow**, not the contract. Prints: what the tool is for, the
write → `validate` → `describe --lens` → revise loop, the exit codes, and the
instruction to reach for `describe` rather than guess.

Deliberately does **not** print the field list, the ladders, or the example —
that is `describe`, and a model that has not yet decided to write a review does
not need them. This is the one call that may be pinned into a system prompt for a
whole session, so it is the one that must stay small.

The workflow it teaches:

1. Write a review document.
2. Run `refinery validate`.
3. Exit 0 — done. Exit 2 — the invocation is wrong, not the review.
4. Exit 1 — each diagnostic names a check. Run
   `refinery describe --lens=<check-name>` for the ones you do not understand,
   fix, and validate again.
5. Unsure about a field before writing? `refinery describe --lens=<field>`. Never
   guess at an enum or a scale.

Budget: ~250 tokens. See [§6](#6-token-economy).

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
emit and what the pointer line prints. `--lens=code` does not: two fields end in
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
in pointer lines, cached in agent memory across sessions, and written into
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
   read the same values that `validate` runs on. A new advisory is documented the
   moment it is registered, and `--disable` accepts exactly the names `--lens`
   explains.
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

### 2.3 `validate`

Reads a review document from `path`, or from stdin when `path` is `-` or omitted.
Runs the structural checks, then verification if a source is supplied, then the
advisories; emits diagnostics and sets the exit code per [§4](#4-exit-codes).

**Every check runs.** A failure in one tier does not gate the next, and a failure
on one comment does not gate the others — see
[review-document.md §11.4](review-document.md#114-partial-documents). One
`validate` call reports everything wrong with a document that can be determined
from it, because the alternative is a caller paying a full write-validate cycle
per mistake. Checks that genuinely cannot run are listed as skipped rather than
passing silently.

Input must be a single JSON object. Multiple documents, JSON Lines, and arrays
fail `document-unparseable` and exit 1: the input is a document to repair, not
an invocation to fix.

#### 2.3.1 Verifying anchors

An anchor is a factual claim: *this path, at this ref, has this line*. Nothing in
the document can confirm it, and a hallucinated line number is indistinguishable
from a real one on inspection — it is well-formed, plausibly numbered, and wrong.
Verification is the only check tier that catches it.

It runs **by default**, against the git repository containing the working
directory, discovered the way git itself discovers one — walking up from the CWD
until a repository root is found. There is no flag. Anchors resolve by object
lookup, `git cat-file` against the object database. No ref resolution is
involved, because a ref is already a SHA; there is nothing to disambiguate and no
chance of resolving to a different commit than the reviewer saw. Commits that are
not checked out still work.

Verifying by default rather than on request is the deliberate part. An opt-in
check makes the plain invocation the weak one, and the plain invocation is what
gets run — a tool whose whole purpose is catching hallucinated anchors should not
require an argument to do it. Pointing `refinery` at a different repository is
`cd`, which every caller already has and no caller has to be taught.

When no repository can answer, verification is **skipped and reported as
skipped**, with the reason on the status line. It is never silently passed: a
run that verified nothing must not look like a run that verified everything, or
the tier is worse than absent — it would license confidence it never earned.

A caller who cannot accept an unchecked document passes
`--require-verification`, which turns "nobody confirmed these anchors" into a
`verification-required` error and exit 1. It fires whenever the anchor claims
went unchecked — no repository, an unreachable one, or a single file git could
not read — because those are one answer to the question the flag asks. It is
off by default: a document is not wrong for being checked somewhere that could
not check it, and only the caller knows whether it needed a repository to.
`--warn-only=verification-required` demotes it like any other verification
check; asking for both is contradictory, but visibly so.

Two things can go wrong there, and they are not the same thing. Running outside
a repository is ordinary, and reports `source: none`. A repository that exists
but could not be asked — git missing, a bare repository, a checkout git refuses
on ownership grounds — reports `source: unavailable` and carries git's own
words. Neither fails the run, because the document is not at fault for either;
but a caller that requires verified anchors can tell them apart, and a run that
checked nothing never claims otherwise.

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

Verification checks are errors by default because they are factual, not matters
of judgment. `--warn-only` demotes them for the legitimate case of a repository that lacks the
reviewed commit — a shallow clone, or a branch deleted and garbage-collected
since.

Verification is not the only audit path, and deliberately not the last word.
Because a ref is an immutable SHA, every anchor in a passing review remains
resolvable by ordinary git tooling long after `refinery` has exited — `git show`,
`git blame -L`, any reviewer or CI job that wants to check what the comment was
actually looking at. `refinery` confirms the anchor points somewhere; git lets
anyone confirm it points at the right thing. Keeping those separate is why the
tool needs no state and no opinion about code.

Diagnostics carry check names, not explanations — the explanation is one
`describe --lens` away, and printing it inline would charge every caller for
detail most of them already have. Output ends with a single pointer line
([§5.1](#51-text-format-default)).

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

## 3. Flags

Behavior is adjusted by flag, not config file. The tool must stay usable with no
setup.

```
--strict                  treat advisories as errors (exit 1)        validate
--warn-only=NAME[,NAME…]  demote the named verification checks       validate
--disable=NAME[,NAME...]  skip the named advisories                  validate
--lens=NAME[,NAME...]     open one entry in full                     describe
--list                    print the lens index, no bodies            describe
--annotated               emit the schema with descriptions intact   schema
--format text|json        output format (default: text)              describe validate
```

Structural checks cannot be disabled or demoted. Verification checks can be
demoted with `--warn-only` but not disabled: they run whenever a repository is
found, and when none is, the skip is reported rather than chosen. Naming an unknown check
in `--disable` or `--warn-only` is a usage error (exit 2) rather than a silent
no-op, so typos surface immediately.

`--strict` is how a caller opts into advisories gating a pipeline. It is not the
default posture: review quality is a judgment the caller owns.

## 4. Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Structurally valid, and anchors verified where a repository was available. Advisories may be present. |
| 1 | Structurally invalid, unparseable, a verification failure, unverified anchors under `--require-verification`, or advisories present under `--strict`. |
| 2 | Usage or I/O error: unreadable path, unknown advisory name, bad flag. |

Distinguishing 1 from 2 matters: exit 1 means *revise the review*, exit 2 means
*fix the invocation*. An agent must be able to tell those apart without parsing
prose.

Advisories alone never produce exit 1. Verification failures do, unless demoted
with `--warn-only`.

## 5. Output

### 5.1 Text format (default)

Diagnostics go to **stderr**; the status line goes to **stdout**. A caller can
capture pass/fail cheaply while still surfacing detail on failure.

Clean:

```
VALID  5 comments, 0 advisories  [anchors unverified]
```

Counts are `errors, advisories`, plus `skipped` when any check could not run:

```
INVALID  4 errors, 1 advisory, 2 skipped  [anchors verified: 5 of 7]
```

Skipped checks are named on one line after the diagnostics, with the reason
grouped rather than repeated:

```
skipped  priority-flat, comment-flood (2 comments have unusable priority)
```

The status line always states whether anchors were verified, and against what:
`[anchors verified: 9 of 9]` on the normal path, `[anchors unverified: <reason>]`
when it could not run. It costs four tokens and it is the difference between "this review
is clean" and "this review is clean as far as anyone bothered to check."

Valid with advisories — note that diagnostics are addressed by comment ID:

```
VALID  6 comments, 3 advisories  [anchors verified: 8 of 8]

advisory  suggestion-no-cons        dropped-context-1
          suggestion 1 ("Pass the caller's context straight through") lists no
          cons; state the tradeoff or say the fix is free
advisory  priority-flat
          all 6 comments are priority 7; the scale is not being used
advisory  id-grouping               missing-context-3
          slug "missing-context" has suffixes 1, 3; renumber contiguously

refinery describe --lens=suggestion-no-cons,priority-flat,id-grouping
```

Structurally invalid:

```
INVALID  2 errors, 1 advisory  [anchors verified: 6 of 7]

error     anchor-file-missing       dropped-context-1
          internal/fetch/client.go does not exist at 4f2c1a9
error     id-unique                 unchecked-error-1
          declared by comments[1] and comments[4]
advisory  summary-thin
          summary is 41 characters with 5 comments; expand it

refinery describe --lens=anchor-file-missing,id-unique,summary-thin
```

A verification failure names the ref it resolved against. "Line 88 is out of
range" invites the reviewer to argue; "line 88 is out of range in a 61-line file
at 4f2c1a9" ends it.

Renderer rules:

- No color codes when stdout is not a TTY
- Message bodies wrap at 80 columns, continuation lines aligned under the message
- Errors before advisories, then document order
- The third column is the comment ID when the diagnostic concerns one comment, a
  JSON Pointer when it comes from `schema`, and empty for document-level checks
- An unknown-property failure names the nearest valid field when one is within
  edit distance 2: `unknown field "end-line" — did you mean "end_line"?`. The
  format rejects unknown fields precisely to catch typos, so spending a few
  tokens naming the intended field turns a rejection into a fix

**The pointer line.** Output carrying diagnostics ends with a blank line and one
runnable command covering every check that fired, deduplicated, in the order the
diagnostics appeared. `schema` diagnostics contribute the *field* they failed on
rather than the name `schema`, which is why the second example asks for
`priority` — the useful lens for "12 is greater than the maximum of 10" is the
priority scale, not an explanation of JSON Schema.

The line exists so recovery is copy-and-run rather than recall. It costs about
15 tokens and it is emitted once per run, never per diagnostic. Suppressed when
there are no diagnostics, and absent from `--format json`, where the check names
are already machine-readable fields.

### 5.2 JSON format

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
the diagnostics — the structured form of the text renderer's pointer line, so a
programmatic caller can fetch explanations without parsing prose. Omitted when
there are no diagnostics.

`skipped` lists the checks that could not run, each with `name` and `reason`. It
is always present, empty when everything ran. As with `verification`, absence
must never be read as success: a consumer that ignores this field will treat a
check that never executed as a check that found nothing.

`verification` is always present. `source` is `"repo"` or `"none"`;
with `"none"`, `verified` is `0` and a consumer can tell that unverified anchors
were not checked rather than found sound. A caller that treats a missing
`verification` block as "verified" is reading an older version, which is why the
field is required rather than omitted when empty.

The whole object goes to stdout in this mode; nothing is written to stderr except
on exit code 2.

## 6. Token economy

Efficiency is a design constraint here, not a nice-to-have. Structured review
competes against a prose paragraph that costs nothing to produce, so the contract
has to pay for itself.

### 6.1 Budgets

Text output. These are ceilings, enforced by golden-file tests that fail when a
command grows past its budget — a limit nothing measures is a limit that erodes.

| Call | Budget | Frequency |
| --- | --- | --- |
| `prime` | 250 | Once per session, often pinned into a system prompt |
| `describe` | 625 | Once per session that writes a review |
| `describe --lens=NAME` | 250 each | Only on uncertainty or a failed check |
| `describe --list` | 200 | Rare; discovery and the unknown-lens error |
| `schema` | 1,000 | Rare; machine consumers only |
| `schema --annotated` | 5,000 | Rare; codegen only |
| `validate`, clean | 20 | Every attempt |
| `validate`, per diagnostic | 40 | Every failed attempt |
| pointer line | 15 | Once per failed attempt |

Verification adds wall-clock, not tokens: its output is the same status line
either way. Because it runs by default, that cost is paid on every loop — object
lookups against a local database, one per distinct file, since the whole document
shares one `ref`.

The two that matter most are `prime` and clean `validate`, because they are paid
on every single loop. Everything else is conditional.

The largest saving is not in any row of that table — it is in **not repeating
it**. A validator that reports one problem at a time turns a document with four
mistakes into four write-validate cycles, each paying the model's full cost of
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
| First validate | 15 | 15 |
| Two failed checks | re-read 4,000 | 500 (two lenses) + 15 pointer |
| Second validate | 15 | 15 |
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

This is also why `validate` output is terse and check names are stable. A stable
name is cacheable — across a session, and across the agent's own memory of what
`broad-scope-alone` meant last time. Prose diagnostics are not.

## 7. Implementation notes

### 7.1 Package layout

```
cmd/refinery/main.go              flag parsing, wiring, exit codes
internal/cli/                     subcommand implementations
internal/cli/interfaces.go        validator, renderer
internal/review/review.go         document types, enums, priority bands
internal/schema/schema.go         //go:embed of the schema, draft compilation
internal/schema/review.schema.json
internal/structural/              hard checks
internal/advisory/                advisory registry and implementations
internal/advisory/interfaces.go   advisory
internal/entry/                   entry registry, namespaces, alias resolution
internal/entry/interfaces.go      provider
internal/entry/schema.go          field:* provider, reads the annotated schema
internal/entry/checks.go          check:* provider, reads the check registries
internal/entry/topics/            topic:* entries, //go:embed of hand-written md
internal/render/                  text and json renderers
```

Interfaces live in each package's `interfaces.go`, defined at the consumer.
Constructors take dependencies explicitly; the graph is wired in `main`.

### 7.2 Advisory registry

An advisory is a small value with a name, a one-line description (reused verbatim
by `prime`), and a check function over the parsed document. The registry is a
slice built at construction, not package-level state — so tests can build a
registry holding one advisory.

Adding an advisory means adding one file and one registry entry. The `check:*`
entry provider and `--disable` validation both read from that registry, so a new
advisory becomes explainable via `describe --lens` and configurable via
`--disable` without touching either command.

### 7.3 Dependencies

Keep the tree shallow — this binary is invoked in tight loops.

- `github.com/santhosh-tekuri/jsonschema/v6` for draft 2020-12 validation
- Standard library `flag` with `flag.NewFlagSet` per subcommand. A CLI framework
  is not warranted for four subcommands, and `prime` already serves the
  agent-facing help role that a framework's generated help would cover.
- testify for tests

### 7.4 Testing

Per project standards: `t.Parallel()` on every test and subtest, `t.Context()`,
`slog.New(slog.NewJSONHandler(io.Discard, nil))` for loggers, moq-generated
mocks in `moq_test.go`.

Testdata carries a corpus of review documents under `internal/advisory/testdata/`
and `internal/structural/testdata/`, each paired with its expected diagnostic
set. Every check needs a passing case and a failing case. Golden files cover the
text and JSON renderers.

## 8. Future considerations

Deliberately deferred, recorded so the design leaves room for them.

- **Diff-aware verification.** Verification confirms an anchor points at something
  real; it does not confirm it points at something *changed*. A base ref
  at the document root, plus the changed-path set derived from it, would catch
  the reviewer that wandered off the diff into untouched code. Deferred because
  it needs a `base` field on the document and a decision about whether commenting
  on unchanged code is an error at all — sometimes the bug is in what the change
  failed to touch.
- **Content-aware verification.** Recording a hash of the anchored span alongside
  the ref would let a consumer detect that the anchored code changed after the
  review was written, rather than merely that the line still exists. Deferred
  because it puts repository content inside the review document, which every
  other decision here has avoided.
- **Review merging.** A `refinery merge` subcommand combining several subagent
  reviews into one. Comment IDs and slug grouping are what make this tractable:
  merge by slug, keep the highest priority per group, and union the suggestions.
  Suffixes are renumbered on merge, so IDs are stable within a document but not
  across them.
- **Suggestion selection.** A `refinery pick` subcommand that, given a review and
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
  pointer line, and `--list` grows to show it.

  Two things have to hold when it lands. Bare-name resolution must stay
  predictable, which is what the namespace precedence and the ambiguity error in
  [§2.2.1](#221-lens-names) are for. And per-entry budgets must survive contact
  with prose written by hand rather than generated from a schema — a knowledge
  base is exactly where 250-token entries quietly become 900-token ones, so the
  budget tests apply to `kb:*` from its first entry.

  A project-supplied knowledge base — conventions read from the repository rather
  than compiled into the binary — is the obvious follow-on and would be another
  provider. It is deliberately not specified here, because it breaks the
  stateless-and-offline principle in a way that deserves its own design.
- **MCP adapter.** A `refinery mcp` subcommand serving the same command layer over
  stdio, for hosts without a shell. The internal packages are structured to make
  this an adapter rather than a rewrite; it is not built now because MCP tool
  schemas cost context on every session whether used or not.
