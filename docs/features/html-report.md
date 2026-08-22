# The HTML report

Feature design. Draft, pending implementation and pending the `cli.md` edit
it calls for but does not make.

Companion to [../cli.md §5.1](../cli.md#51-one-format), which this feature
asks to extend a second time, and to
[combined-reviews.md §8.3](combined-reviews.md#83-the-markdown-projection),
which it is the sibling of: same command, same source value, same two
failure modes to close, a different target grammar.

## 1. What this adds

`collect-reviews --format html` is a third projection of the one result
value [combined-reviews.md §8.1](combined-reviews.md#81-shape) already
defines, alongside `--format json` and `--format markdown`: one
self-contained `.html` file, written to stdout like the other two, meant to
be opened in a browser rather than parsed by a machine or pasted into a PR
comment. Nothing about what `collect-reviews` computes changes. What
changes is the third way of looking at it.

The three formats now serve three audiences, and naming that plainly is
worth doing once, here, rather than leaving it to be inferred from which
flag happens to be documented where:

| Format | Audience | Read how |
| --- | --- | --- |
| `--format json` | A machine | Parsed by an orchestrator, fed into another tool, asserted on in a test |
| `--format markdown` | A human, via pass-through | Embedded in a PR comment or chat message a person reads once ([combined-reviews.md §8.3.3](combined-reviews.md#833-the-test-that-pins-it)) |
| `--format html` | A human, directly | Opened in a browser — the report *is* the artifact, not a fragment embedded in a larger one |

The user asked for one thing specifically: a clean, self-contained report
with collapsible sections, page anchors, effective use of horizontal space,
and colorized code. Every decision below either serves that request or
exists because serving it safely required a decision the request itself
didn't make.

## 2. The amendment this falls under

[cli.md §5.1](../cli.md#51-one-format) permits exactly one exception today,
worded narrowly around `--format markdown`. This feature asks for a second
carve-out on the same command, under the same reasoning, not a new one.
Read literally, the existing table row's argument is: a second output is
acceptable when it is "a pure projection of the identical result value the
JSON form serializes, built once, by one code path, with the same escaping
and fencing discipline specified there to close the forgery half of this
section's own argument." HTML has to satisfy that sentence on its own
terms, not borrow markdown's compliance with it.

**It is a pure projection.** `internal/render.HTML` (name chosen to sit
beside `internal/render.JSON` and `internal/render.Markdown`) takes the
identical `collect.Result` value the other two already take
([combined-reviews.md §8.3.1](combined-reviews.md#831-one-structure-two-renderers)).
It re-sorts nothing, re-filters nothing, re-derives no count, and asks the
store no question the assembly step didn't already ask. Every fact on the
page — every submission, every comment, every severity number — is read off
a field `collect.Assemble` already populated. The one thing this renderer
computes that the other two don't is presentational, not factual: whether a
given comment's `<details>` starts open or closed. [§7.3](#73-findings-and-what-collapses-by-default)
argues that this is a rendering decision, not a second computation of any
result, in the same sense that Markdown choosing bold text over a table
cell is a rendering decision — it is a pure function of a field
(`Comment.Priority`) the JSON form already serializes, computed fresh on
every render from the same input, never stored, never fed back into
`Assemble`.

**It is one code path.** `internal/render` keeps `HTML` beside `JSON` and
`Markdown`, all three constructed the same way, all three taking the same
value in `internal/cli`'s output switch. There is no second implementation
of `collect-reviews` wearing an HTML skin.

**The escaping discipline is the same discipline, applied to a different
grammar, and that has to be said honestly rather than glossed — and, since
[§5](#5-the-script-what-it-may-touch-and-what-it-must-never-touch) accepted
a script this document originally argued it would never need, the
discipline now has to cover a second grammar besides HTML's own.**
Markdown's exception earns its safety from a hand-derived, CommonMark-aware
escaper, because markdown has no contextual-escaping equivalent in the
standard library. HTML's markup safety comes from `html/template`'s
built-in contextual autoescaping instead — a different mechanism, not a
rewrite of the same one. What is identical is the property both mechanisms
exist to guarantee: **caller-authored content can never be interpreted as
tool-generated structure**. That property, not the specific escaper that
enforces it, is what `§5.1`'s exception is actually conditioned on, and
[§4](#4-escaping-html-template-never-hand-rolled) and
[§6.1](#61-why-not-chromas-html-formatter) are where this document defends
that HTML's mechanism enforces it at least as strictly as markdown's does —
in one respect, more strictly, because `html/template` has no
position-dependent rule for an escaper to get subtly wrong the way
CommonMark's line-start grammar did ([§4](#4-escaping-html-template-never-hand-rolled)).
The script is not exempt from that same property; it is answerable to it by
a different means again — not an escaper, because nothing is ever
interpolated into it for one to escape.
[§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes) is where
this document defends that the script's zero-interpolated-data guarantee
closes the identical forgery question one grammar further out: a reviewer's
`body` cannot forge HTML structure, per `html/template`, and it cannot
reach JavaScript execution either, because there is no path from a
template value into the one `<script>` element this page ships.

### 2.1 What has to change in `cli.md §5.1`

The current table row is written around exactly one exception and needs to
generalize to two, on the same command, without weakening its own argument:

- The command syntax line and the flags table
  ([cli.md §2](../cli.md#2-commands), [cli.md §3](../cli.md#3-flags)) both
  need `--format json|markdown` widened to `--format json|markdown|html`.
- The amendment row's opening clause, "is the one exception," has to become
  "are two narrow exceptions" or equivalent — the row still describes one
  command carrying more than one non-JSON projection, but "one" stops being
  literally true the moment a second format joins markdown.
- "With the same escaping and fencing discipline specified there" has to
  stop implying a single shared mechanism. The honest replacement is close
  to: "with an escaping discipline suited to each target grammar — CommonMark
  fencing and position-aware backslash-escaping for markdown,
  `html/template`'s contextual autoescaping for HTML's markup, and a script
  that never receives caller-authored content in the first place for HTML's
  one script context — each closing the forgery half of this section's own
  argument in its own grammar's terms."
- The closing clause, "`collect-reviews` now being the sole command that
  does" (have more than one format to choose from), needs no wording
  change — it was already phrased as "more than one," not "exactly two,"
  and stays true with three.
- [cli.md §6.1](../cli.md#61-budgets)'s budget table gains a row:
  `collect-reviews --format html` — see [§11](#11-budgets-and-audience).

None of this reopens `§5.1`'s core argument. `submit-review`, `reviews`,
`describe`, and `schema` are as unaffected by this feature as they already
are by markdown's exception — they gained no flag, no second output, no new
failure mode. The exception widens on the one command that already carries
it; nothing about the rule the other four commands live under moves.

### 2.2 The tests that make the claim true, not merely stated

[combined-reviews.md §8.3.3](combined-reviews.md#833-the-test-that-pins-it)
pins the markdown projection with three tests — parity, fidelity, forgery —
run against the same fixtures the JSON renderer's golden tests already use.
HTML needs the same three, adapted to its grammar, against the identical
fixtures, or this section's claim to fit under `§5.1`'s amendment is asserted
rather than earned:

1. **Parity.** Parse every `<details class="finding">` element out of the
   rendered HTML — a structural query, not a text scan; [§2.2.1](#221-how-html-output-is-tested)
   is specific about using a real parser rather than a regular expression
   for this — and assert that the set of their `id` attributes equals the
   set of `comments[].id` the JSON form carries, exactly, byte for byte, no
   decoding step in between. This is simpler than the Markdown parity test
   it mirrors, and deliberately so: [§8](#8-anchors-ids-and-what-a-copied-link-contains)'s
   decision is that the `id` attribute *is* the qualified id, unmodified,
   including a leading `#` on the ordinal form — there is no substitution
   for this test to undo, which is itself a consequence worth stating, not
   just a simplification. Same members, same count. This is the test that
   would catch an HTML renderer that silently dropped a comment, or renamed
   one, the same way `§8.3.3`'s parity test catches the identical drift in
   Markdown.
2. **Fidelity.** For each comment, assert that `html/template`'s output,
   run back through Go's own `html.UnescapeString` on every text node, and
   with chroma's per-token spans stripped and their `Value` fields
   concatenated, reproduces the JSON form's `body` and `code` strings byte
   for byte. This is `html/template`'s own contract — the standard library
   guarantees round-trip fidelity through its escaper — so this test is
   less about catching `html/template` misbehaving and more about catching
   a template that stopped passing a field through it: `{{.Body}}` versus
   an accidental `{{.Body | someHelperThatBypassesEscaping}}` is exactly the
   class of regression this test exists to catch before it ships, not
   after.
3. **Forgery.** [combined-reviews.md §12.3](combined-reviews.md#123-the-markdown-projection-and-what-escaping-prevents)'s
   fixture — a `body` containing `<script>alert(1)</script>`, a fake
   `</details><details open><summary>FORGED` sequence, and a `code` excerpt
   containing a literal `</code></pre>` — rendered to HTML, and asserted:
   the page contains **exactly one** `<script>` element, its byte content
   equal to a fixed constant the test itself carries (see
   [§2.2.1](#221-how-html-output-is-tested) — this is the assertion that
   would catch a future change that starts interpolating something into it,
   the one regression [§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes)'s
   whole contract exists to prevent); the injected `</details>` sequence and
   the literal `<script>alert(1)</script>` both appear as escaped, inert
   text inside the real comment's body paragraph, never as a structural
   boundary or a second script element; and the page still parses as
   well-formed HTML with the same element count a clean fixture of the same
   shape produces — an injected closing tag that actually closed something,
   or an injected `<script>` that actually became one, would change that
   count, and the test fails the moment either does. This is the test in
   [§2.2.1](#221-how-html-output-is-tested)'s terms below, and it is the one
   that actually matters: parity and fidelity catch a renderer that drifted
   from the truth; forgery catches a renderer that can be made to lie.

All three run against the identical fixtures `§8.3.3` already uses, which is
what keeps "one structure, three renderers" a fact a test checks rather
than a sentence in this document.

#### 2.2.1 How HTML output is tested

A single golden file over the whole rendered page — the pattern
[cli.md §7.4](../cli.md#74-testing) uses elsewhere in `internal/render` —
does not scale here: a 145KB page ([§11.1](#111-measured-size)) changes on
almost any content edit, so a byte golden is either constantly re-recorded
with `-update` or, per
[render_test.go's own warning](../../internal/render/render_test.go) on
`TestIndexAndSummaryCannotForgeOutput`, rarely touched enough that
corruption gets baked into it unnoticed. This renderer keeps exactly one
golden file — small, stable, exercising every layout region once, reviewed
by eye — and puts everything else behind assertions that check a specific
property, not a whole file's worth of bytes:

- **Structural assertions**, parsing the output with Go's own HTML parser
  (`golang.org/x/net/html` — not part of `html/template`'s own dependency
  graph, which is stdlib and imports nothing from `x/net`, but already
  present in this module's graph as an indirect requirement pulled in by
  something else in the tree; [§6.4](#64-the-dependency-this-costs)
  accounts for what a test importing it directly actually costs) rather
  than a regular expression, exactly as
  [§2.2](#22-the-tests-that-make-the-claim-true-not-merely-stated)'s parity
  test above does. These check element counts, attribute presence, `id`
  uniqueness across the page — `id-unique` only checks the *input*
  document ([review-document.md §11.1](../review-document.md#111-structural-checks--hard)),
  so this renderer's own test checks the *output* directly — and the
  open/closed state of every `<details>` against
  [§7.3](#73-findings-and-what-collapses-by-default)'s priority boundary.
- **The forgery test**, in the shape of `internal/render/render_test.go`'s
  own `TestAnAuthoredValueCannotForgeOutput` — a single hostile fixture,
  asserted against by property rather than full-file comparison.
  [§2.2](#22-the-tests-that-make-the-claim-true-not-merely-stated)'s point 3
  above details this renderer's version, and it is the one that matters
  most for this format: HTML is the one grammar here where forged
  structure means a browser executing something, not a person misreading a
  table row.
- **The escaping and script-purity pins**: one test walks every
  `text/template.Template.Tree` this renderer parses and asserts
  `template.JS`, `template.JSStr`, and `template.HTML` appear nowhere
  except the one named exception
  ([§8.3](#83-templateurl-the-one-deliberate-bypass-and-why-its-safe-here)'s
  `template.URL` wrapper) — a static check over the template's action
  list, not any one fixture's output, so it cannot be defeated by a
  fixture that happens not to exercise the bypassed path. This is what
  turns [§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes)'s
  "zero interpolated bytes" claim into one a future edit cannot silently
  break.

Golden files still cover the one thing property-based assertions are the
wrong tool for: what the page actually looks like, which is why the one
retained golden stays — reviewed by eye, not asserted on structurally, and
kept deliberately small so a reviewer can read the whole diff.

### 2.3 Wiring: a third formatter beside JSON and Markdown, not a third interface method

`internal/cli/interfaces.go`'s `renderer` interface carries a comment
worth answering directly: "There is one implementation and one format: two
of either is how the same run came to be described two different ways,
which is the defect this interface no longer permits." Markdown already
answered this question once, and HTML should reuse that answer rather than
reopen it: `internal/cli/collect_reviews.go`'s `renderCollectReviews` does
not add a `Markdown` method to `renderer` at all. It constructs
`render.NewMarkdown()` directly and calls `.CollectReviews` on that
concrete value, bypassing the interface entirely — the dispatch function's
own comment explains why: "the interface's own doc comment is explicit
that two renderers behind one interface is exactly the shape it exists to
rule out everywhere else." Adding an `HTML` method to `renderer`, or
widening `CollectReviews` with a format parameter, would both reopen that
shape, for a command that has already demonstrated it does not need to.

**Decision: `render.HTML`, a third concrete formatter beside `render.JSON`
and `render.Markdown`, following the identical precedent.**
`renderCollectReviews`'s two-way branch becomes a three-way one:

```go
func (a *App) renderCollectReviews(format string, envelope render.CollectReviewsEnvelope) error {
	switch format {
	case "markdown":
		return render.NewMarkdown().CollectReviews(a.stdout, envelope)
	case "html":
		return render.NewHTML().CollectReviews(a.stdout, envelope)
	default:
		return a.renderer.CollectReviews(a.stdout, envelope)
	}
}
```

`renderer` itself gains nothing — no new method, no reason to revisit its
own doc comment, since that comment was never "collect-reviews has one
output," it was "one interface, one implementation of each thing that goes
through it." `checkCollectReviewsFormat`
([internal/cli/collect_reviews.go](../../internal/cli/collect_reviews.go))
widens from `"json", "markdown"` to `"json", "markdown", "html"`, and
nothing else in `internal/cli` changes shape.

## 3. The command

```
loam-refinery collect-reviews --ref=SHA [--repo=NAME] [--format json|markdown|html]
```

`--format html` behaves exactly like `--format markdown` in every respect
this document doesn't call out separately: it requires `--ref`, it reads
the same store, it uses the same empty-and-failure-case table
([combined-reviews.md §9](combined-reviews.md#9-empty-and-failure-cases)) —
`known: false` and zero submissions still render a well-formed page with an
empty state, not an error, and the process's exit code is unaffected by
which format was asked for. Output goes to stdout, one file's worth of
bytes, exactly like the other two; `collect-reviews --format html > report.html`
is how a caller gets a file, the same way `--format markdown > review.md`
already works today. Nothing about this feature gives `loam-refinery` a
second way to write a file — it still only writes inside the store
([config.md §2](../config.md#2-locations)), never elsewhere, and stdout
redirection is the caller's decision, not the tool's.

## 4. Escaping: `html/template`, never hand-rolled

The markdown renderer hand-rolls its escaper because markdown has no
stdlib equivalent to `html/template` — nothing in Go's standard library
understands CommonMark's grammar well enough to escape text for it
automatically, so `internal/render/markdown.go` derives its own rule from
the specification by hand
([combined-reviews.md §8.3.2](combined-reviews.md#832-escaping-and-fencing-caller-authored-text)).
That hand-derivation is exactly what produced this project's own open P0:
`refinery-xlp.22`, still open, found and reproduced end to end by a security
review run on this project's own branch. `escapeMarkdown` split free text on
`\n` alone; CommonMark counts a bare carriage return as a line ending too,
so any reviewer-authored `body`, `summary`, `pros`, or `cons` field
containing a bare `\r` could plant an unescaped line-start marker at what
CommonMark treats as column zero. Demonstrated concretely: a `body`
containing a carriage return followed by `## #1:forged-id-1` renders a real
`<h2>` carrying a forged qualified id — the one structural marker this
format owns — and a `body` ending in a carriage return followed by a tilde
fence opens an unterminated fence that swallows a subsequent reviewer's
entire priority-9 section, headings included. The bug is not that hand-rolled
escaping is impossible to get right; the previous section of this document
did get it right for the character set, the position rule, and the fence
sizing. It is that hand-rolled escaping has a surface — "what counts as a
line" — that a careful, reviewed implementation still missed, because
CommonMark's grammar has more than one way to answer that question and only
one of them was checked against.

In HTML the same class of mistake pays out categorically worse. A forged
markdown heading misleads a reader about document structure. A
carriage-return-shaped gap in an *HTML* escaper is the difference between
displaying `<script>` as four characters of visible text and executing it —
an `<h2>` a reviewer didn't write and a `<script>` a reviewer didn't write
are not the same order of failure, and this project should not accept for
its HTML output the exact risk profile that just produced a P0 on its
Markdown one, by hand-deriving a second position-dependent escaper for a
grammar that already has a battle-tested, contextual one in the standard
library.

`html/template` is that library. It parses the template, tracks which HTML
context each `{{ }}` action sits in — element text, attribute value,
`<script>` body, URL — and escapes every value for that specific context
automatically, at execution time, from the template structure itself rather
than from a hand-maintained character table. There is no equivalent of
`escapeMarkdown`'s "split on `\n`, walk runes, check a lookup table" for
this renderer to get subtly wrong, because there is no hand-written escaper
in this renderer at all: every field placed into the template — `Body`,
`Summary`, `Pros`, `Cons`, `AnchorFile`, chroma token `Value`s
([§6](#6-syntax-highlighting-chroma-token-api-only)) — goes through
`html/template`'s own contextual escaper, and only that. This closes the
carriage-return class of bug structurally rather than by finding and
patching the specific position rule that missed it: `html/template` has no
"first character of a line" concept for an attacker-controlled `\r` to
exploit, because HTML's own grammar has no line-start-sensitive block
constructs the way CommonMark does. A `<pre>` block's content is inert text
regardless of how many embedded line breaks — `\n`, `\r`, `\r\n` — it
contains; there is no position inside it that means anything other than
"more text." [§6.1](#61-why-not-chromas-html-formatter) is where this
matters concretely, for the one part of this renderer that touches
generated markup rather than only escaped text.

## 5. The script: what it may touch, and what it must never touch

An earlier draft of this section argued that collapsible sections and
working page anchors could both be had for zero lines of script, on the
strength of one specific claim: that browsers force-open a `<details>`
ancestor of any element a URL fragment targets. That claim was tested
directly rather than trusted from a reading of the living standard's prose,
and it is false. Tested in Chromium 144, over both `file://` and `http://`,
by direct navigation to a URL carrying the fragment and by clicking a real
in-page link to it, with the target `id` placed on the `<details>` element
itself and, separately, on its `<summary>` child — every one of those four
combinations produced the same result: the browser scrolls to the target
correctly and **never flips its `open` attribute**. A `<details>` collapsed
by [§7.3](#73-findings-and-what-collapses-by-default)'s default and
targeted by a fragment link stays collapsed, with the browser dutifully
scrolled to a spot the reader cannot see anything at. "Collapsible sections
that also support working page anchors" is not achievable with zero script;
the two halves of the user's original request are in genuine tension, and
resolving it costs the one thing this section used to argue against paying.

The user has accepted inline JavaScript to close that gap. What follows
replaces "there is no script" — which was the load-bearing property, not
the byte count, and is no longer true — with a narrower property doing the
identical job: not *no script*, but **no script a hostile reviewer can
reach.**

### 5.1 The contract: one static script, zero interpolated bytes

Exactly one `<script>` element ships with the page, and it is **static**:
every byte of it is fixed at compile time, identical across every report
this tool ever renders, never derived from a template action, and never
containing anything read from the review data it sits beside. It never
appears inside a `{{ }}` action anywhere in the template; `template.JS` and
`template.JSStr` — `html/template`'s two escaper-bypass types for a script
context, the mirror image of `template.URL`
([§8.3](#83-templateurl-the-one-deliberate-bypass-and-why-its-safe-here))
for a URL context — are used nowhere in this renderer, on any field. The
script's only channel into the page's data is reading `data-*` attributes
and element content the template already wrote, at runtime, in the
reader's own browser, after `html/template`'s contextual escaping has
already run and finished.

Every attribute the script reads or compares against is one of two shapes:
a grammar-constrained enum member — `data-priority-css="pri-mustfix"`,
`data-category-css="cat-security"`
([review-document.md §9](../review-document.md#9-category)),
`data-lens="backend"` (itself constrained to `profile-format` or an
ordinal tag, [§8](#8-anchors-ids-and-what-a-copied-link-contains)) — or an
integer. Reviewer prose — `Body`, `Summary`, `Pros`, `Cons`, a suggestion's
own text, a `code` excerpt — never reaches a `data-*` attribute, never
reaches an inline event-handler attribute (`onclick` and its relatives
appear nowhere in this template; every handler is wired by the script
itself, via `addEventListener`, so there is no attribute value
`html/template` would ever need to escape for a script context in the
first place), and never enters a JavaScript string, expression, or
comparison the script makes anywhere.

This is the property that **replaces** "there is no script context for
caller-authored content to escape into," now that a script context
genuinely exists: **no caller-authored byte ever reaches it.** That is a
stronger, mechanically checkable claim than the one this section used to
rest on, and it is what keeps
[§4](#4-escaping-html-template-never-hand-rolled)'s argument intact for
script the way `html/template`'s own autoescaper keeps it intact for
markup — not by escaping anything handed to the script, but by
guaranteeing nothing reviewer-authored is ever handed to it in the first
place.

### 5.2 Progressive enhancement is mandatory, not a nicety

A report opened with JavaScript disabled — a security-conscious reader's
browser setting, a `file://` open in a hardened profile, an archival or
linting tool that fetches the bytes and never executes anything — must be
**fully readable**: every finding present in the DOM, none of them hidden
by default, [§7.3](#73-findings-and-what-collapses-by-default)'s
open/closed split applied server-side exactly as it already is, and the
enhancement controls (expand-all, collapse-all, the filter toolbar) either
absent or visibly inert rather than gating any content behind them. This
was verified directly against the prototype, not assumed: with the script
disabled, the fixture's full 36 findings are present in the document and 5
of them start open — the identical set [§7.3](#73-findings-and-what-collapses-by-default)'s
priority-≥-7 rule opens when the script is running, because that split is
computed once, server-side, into the `open` attribute on each `<details>`
element, and the script never touches it except in direct response to a
reader's own click or a fragment it is routing to.

The filter toolbar is the one piece of markup allowed to disappear rather
than degrade in place: it has nothing to enhance without the script that
drives it, so the page's own stylesheet hides it by default
(`.toolbar { display: none }`), and the script's own first statement undoes
that — adding a `js` class to `<html>`, which `html.js .toolbar { display:
flex }` matches. This is the one place on the page where a class toggle
gates *visibility* rather than content, permitted by
[§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes)'s
contract because nothing reviewer-authored depends on whether the class is
present.

### 5.3 What the script does

Three responsibilities, and nothing beyond them:

**Anchor routing.** On page load, and again on every `hashchange`, the
script reads `location.hash`, strips the leading `#`, percent-decodes it —
a copied or typed fragment can arrive percent-encoded,
[§8.2](#82-the-href-percent-encoded-and-why-thats-the-reversal) is specific
about when — and looks for an element whose `id` matches, exactly, either
the raw or the decoded form. Once found, it walks every `<details>`
ancestor of that element, via `Element.closest("details")` repeated up the
tree, sets each ancestor's `open` property to `true`, and scrolls the
target into view. This is the whole fix for this section's opening finding:
the browser's own scroll-to-fragment behavior still runs unmodified — what
was false was only the claim that it also opens `<details>` ancestors — and
the script supplies the one piece the platform does not.

**Expand all / collapse all.** Two buttons, each walking every
`<details class="finding">` on the page and setting its `open` property
uniformly. No field of any finding is read to decide this — every element
gets identical treatment — which is what keeps this responsibility inside
[§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes)'s
contract trivially: there is no reviewer content for this code path to be
influenced by.

**Filtering, by priority band, category, and lens.** A row of toggle
chips, one per value actually present among this report's own findings —
[§7.3](#73-findings-and-what-collapses-by-default) already computes the
priority band per comment, category is the closed seven-value enum
[review-document.md §9](../review-document.md#9-category) defines, and
"lens" is the profile-or-ordinal-tag facet
[§8](#8-anchors-ids-and-what-a-copied-link-contains) defines. Clicking a
chip toggles its own `aria-pressed` state and re-evaluates every finding
against the union of active chips within each facet, intersected across
facets — a `pri-mustfix` chip and a `cat-security` chip both active shows
must-fix security findings, not the union of the two groups. Every
comparison the script makes here is against a `data-*` attribute already
constrained to an enum member or an integer, per
[§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes).

### 5.4 Filtering's safety requirement: a filtered report must not read as a clean one

The cheapest working implementation of filtering hides elements outright
and says nothing else about it — one CSS rule, `display: none` on every
finding that fails the active filters, full stop. That is real, working
filtering, and it is the wrong thing to ship, because a filtered report
looks **structurally identical** to a clean one: the submissions index
still says `total: 7`, the envelope strip still reports every field
truthfully, and nothing in the page's own chrome tells a reader that four
of a report's five must-fix findings are off-screen because a chip is
still pressed from an earlier look — the exact misreading
[cli.md §1](../cli.md#1-overview)'s entire project exists to prevent, now
applied to this renderer's own interface rather than to a reviewer's
document.

Three things close that gap, all mandatory, none of them polish added
later if there's room:

- **A visible-versus-total count** — "4 of 36 findings shown" — rendered
  always, whether a filter is active or not, never appearing only on
  request.
- **A banner, shown only while a filter is active**, stating in words that
  findings are currently hidden — not a color change on the count alone,
  which a skimming reader can miss entirely, but a sentence a skim cannot.
- **A one-action reset** that clears every active chip in a single click,
  so returning to the full report never costs re-deselecting each facet by
  hand.

None of this touches the data underneath it: filtering is presentation
only, over the identical, unfiltered `collect.Result`
[§2](#2-the-amendment-this-falls-under) already establishes this renderer
never re-derives, and the count the banner reports is computed client-side
from the DOM the server already rendered in full — never a second query,
never a re-filtered fetch.

**Hiding uses the `hidden` content attribute, not a CSS class.** The
prototype hides a filtered finding with a stylesheet class
(`filtered-hidden { display: none }`), which removes it visually but
leaves it in the accessibility tree in a way browsers and screen readers
are inconsistent about — some announce it as present, some don't, and
nothing in the relevant specifications promises either way for a plain
class-driven `display: none`. The `hidden` attribute — `el.hidden = true`
on a finding's `<details>`, toggled directly by the script, never a class
the stylesheet also has to remember to wire a rule for — is the one HTML
already defines to mean "this content is not currently relevant," with a
default `display: none` rendering *and* a defined removal from the
accessibility tree that assistive technology is expected to honor. The
renderer this document specifies uses `hidden`, not the prototype's class;
this correction is stated explicitly because it is exactly the kind of
detail a test asserting on rendered bytes alone would never catch — both
choices render invisibly to the eye, and only one of them tells a screen
reader the truth.

**`hidden`'s `display: none` is a UA-stylesheet default, not a floor, and
this feature's own stylesheet is exactly the kind of thing that can sit
above it.** The cascade does not give the `hidden` attribute any special
weight — it wins only because no author rule beats it, and any ordinary
rule that sets `display` on the same element outranks it by the normal
cascade, author styles over the user-agent sheet, no `!important` or
elevated specificity required. `details.finding { display: flex }` in
this renderer's own CSS — a plausible rule for laying out a finding's
header row, nothing exotic or careless about it — silently defeats
filtering the moment it exists: the element still carries `hidden="true"`,
correctly, exactly as the script set it, and a reader still sees it on
screen, because `display: flex` wins the cascade regardless of what the
attribute says. This is the same misreading [§5.4](#54-filterings-safety-requirement-a-filtered-report-must-not-read-as-a-clean-one)
exists to prevent — a filtered report that looks clean — reached by a
route this section did not otherwise name: not a missing banner, not a
stale count, but a `display` rule that never mentions filtering at all.
It is also invisible to a markup-only assertion: a test that checks the
`hidden` attribute's presence and value, the kind
[§2.2.1](#221-how-html-output-is-tested)'s structural assertions run,
would see the attribute set correctly and pass, because the attribute
*is* correct — the element's computed `display` is a rendering-level fact
no attribute inspection touches, which is precisely what makes this
failure mode worth writing down rather than trusting a passing test to
have already ruled out.

The defence is one rule in this feature's own stylesheet:
**`[hidden] { display: none !important }`**, stated once, near the top of
the sheet, ahead of any rule that might otherwise style a `<details>`.
The alternative — an absolute prohibition on author `display` rules for
any element that can carry `hidden` — asks eight implementers, and every
edit after them, to keep remembering a constraint the stylesheet itself
cannot enforce; a single misplaced `display` declaration months from now
would reopen exactly this gap, silently, with no test the way to catch it
short of re-deriving this reasoning from scratch. The CSS rule instead
puts the guarantee where the failure would occur: `!important` makes
`[hidden]`'s `display: none` outrank any author rule regardless of
selector specificity or source order, so the property this section
depends on holds even if a future rule targets `.finding` more
specifically than `[hidden]` does. One line to audit beats a prohibition
that has to be remembered.

### 5.5 The two non-baseline APIs this script leans on

Two DOM APIs this script uses sit outside the most conservative "works
everywhere, always has" browser-support set an offline tool with a wide
readership might otherwise hold itself to: `Element.closest`, which anchor
routing uses to walk `<details>` ancestors, and `CSS.escape`, used when
routing needs to build a CSS attribute selector out of a fragment value
that `document.getElementById` tolerates but a selector string would not.
Both have shipped in every evergreen browser for years and neither is
exotic, but they are the two things to check first if this renderer is
ever asked to support a browser target older than "whatever ships today" —
everything else the script does (`addEventListener`, `classList`, template
literals) is safe further back than that.

## 6. Syntax highlighting: chroma, token API only

`github.com/alecthomas/chroma/v2` (pinned at v2.27.0) tokenizes source code
by language. This is the most consequential technical decision in this
document, because chroma's own convenient API is the wrong one to use here,
and using it would silently reopen the exact hole [§4](#4-escaping-html-template-never-hand-rolled)
exists to close.

### 6.1 Why not chroma's HTML formatter

Chroma ships `formatters/html`, which takes a token stream and returns a
complete, styled `<span>`-wrapped HTML fragment as a string, ready to
`fmt.Fprint`. It is the obvious thing to reach for, and it is disqualified
for exactly one reason: chroma's formatter emits HTML markup as a Go
`string`. To place that string into a `text/template`-derived page without
`html/template` re-escaping the `<span>` tags into visible text, the string
has to be wrapped in `template.HTML` — the type `html/template` treats as
"already safe, do not escape." That is precisely the bypass
[§4](#4-escaping-html-template-never-hand-rolled)'s whole argument is against:
`template.HTML` tells the contextual autoescaper to trust the string as
markup, unconditionally, and chroma's formatter builds that string by
concatenating its own generated `<span>` tags around **the source text
being highlighted** — which, on a `comment.code` or `suggestion.code` field,
is caller-authored content
([review-document.md §6.1](../review-document.md#61-code-before-and-after)).
Wrapping chroma's formatted output in `template.HTML` would mean trusting a
string that contains attacker-influenced bytes as markup — not because
chroma has a bug, but because the moment any caller-influenced byte reaches
a `template.HTML` value, decision 1's entire guarantee has a hole in it,
regardless of how carefully chroma itself escapes the source text it wraps.
This is not a hypothetical: it is the identical shape of mistake `formatters/html`
exists to make convenient, and using it here would mean this document spent
[§4](#4-escaping-html-template-never-hand-rolled) arguing for contextual
autoescaping and then handed one field a path around it.

The fix is to never call the HTML formatter at all, and use chroma's
tokenizer directly instead:

```go
lex := lexers.Match(path)      // or lexers.Fallback when nil
if lex == nil {
    lex = lexers.Fallback
}
it, err := lex.Tokenise(nil, source)
for _, t := range it.Tokens() {
    class := classFor(t.Type)  // §6.2 — "" for the plain fallback
    // t.Value is passed to the template as plain text; only `class`
    // reaches an attribute.
}
```

`t.Value` is the source text, unmodified, exactly the substring chroma
matched as one token. It goes through the template as ordinary text content
— `{{.Value}}` inside a `<span class="{{.Class}}">` — subject to the same
contextual autoescaping as `Body`, `Summary`, or any other field. `t.Type`
is a `chroma.TokenType`, a small enum value; only a CSS class name derived
from it, never anything chroma read out of the source, reaches an HTML
attribute. Nothing chroma produces is ever trusted as markup, and nothing
about this path resembles `formatters/html`'s convenience — it costs one
loop and a lookup table, in exchange for keeping every byte of code content
inside the one escaping discipline the rest of the page already uses.

This is also where [§4](#4-escaping-html-template-never-hand-rolled)'s
closing point pays off directly: a token's `Value` can itself contain any
byte the source file contains, carriage returns included — a Windows-authored
source file quoted verbatim in a `code` excerpt is not a hypothetical.
Because that text lands inside a `<pre>` element as escaped text content,
never as markup, and HTML's grammar assigns no meaning to a byte's position
within a line the way CommonMark's block grammar does, there is no
`escapeMarkdown`-shaped bug for a stray `\r` to exploit here. The class of
vulnerability `refinery-xlp.22` names cannot recur through this path, not
because it was specifically checked for, but because nothing in `<pre>`
text content is position-sensitive in the way that bug depended on.

### 6.2 The class mapping and the CSS palette

Chroma's own bundled styles (`styles.Get(name)`) return a fixed palette of
concrete colors per token type — probed directly against `styles.Get("github")`,
which resolves `chroma.TextWhitespace` to `#ffffff`, one theme's choice
baked into a value this renderer would otherwise have to either accept
outright or override token by token. Emitting chroma's own colors inline —
`style="color:#ffffff"` per span — bakes that one theme into every report
this tool ever produces, with no way for a reader's browser or OS-level
dark-mode preference to override it, and inflates the page with a `style`
attribute per token instead of one small stylesheet.

Instead, every token maps to one of a small, fixed set of CSS classes,
defined once, at the top of the page:

| Class | Chroma categories | Covers |
| --- | --- | --- |
| `c-kw` | `Keyword` and its subtypes | `if`, `func`, `class`, `import`, … |
| `c-str` | `String`, `LiteralString` and subtypes | Quoted literals, including multi-line ones |
| `c-cm` | `Comment` and subtypes | Line and block comments |
| `c-nm` | `Name` and subtypes (`NameFunction`, `NameBuiltin`, `NameClass`, …) | Identifiers |
| `c-num` | `Number`, `LiteralNumber` and subtypes | Numeric literals |
| `c-pn` | `Punctuation`, `Operator` | Braces, commas, `+`, `:=`, … |
| *(none)* | Everything else — `Text`, `Whitespace`, `Background`, `Error`, `Other` | Plain text: no `<span>`, no class, no color |

**The obvious way to check "is this token a string" is the wrong one, and
it fails silently rather than loudly.** Chroma groups its `TokenType`
values in ranges of 1000 for a top-level category and ranges of 100 for a
subcategory within it — `chroma/types.go`'s own comment states this
directly. `LiteralString` occupies 3100–3199 and `LiteralNumber` occupies
3200–3299, both of them subcategories of the same top-level `Literal`
category, 3000–3999. `chroma.TokenType.InCategory` checks membership by
dividing by 1000, which means it cannot tell `LiteralString` and
`LiteralNumber` apart — they are the same category by that arithmetic,
different only in their hundreds digit — and this was verified directly,
not inferred from reading the source: `chroma.LiteralNumberInteger.InCategory(chroma.LiteralString)`
evaluates to `true`. A `classFor` written the obvious way, checking
`t.InCategory(chroma.LiteralString)` for `c-str`, silently paints every
numeric literal with the string color, because every `LiteralNumber*`
token type also satisfies that check. **`InSubCategory`, which divides by
100 instead of 1000, is the check that actually distinguishes them**, and
it is the one this renderer uses for both `c-str` and `c-num`. The
prototype shipped the `InCategory` version first, and the bug was caught
only because the fixture used to build it happened to contain zero number
literals in its two Go excerpts — a fixture with even one integer constant
would have shown a numeric literal rendered in the string color, and
nothing about the page would have looked obviously wrong, because a wrong
color is not a rendering error a test framework notices on its own. This
is stated as a named trap, not folded silently into the table above,
because the table alone reads as though `InCategory` would be sufficient —
it lists `String` and `Number` as if they were peer top-level categories,
which chroma's own type layout does not actually make them.

A token whose type falls outside the six named categories gets no wrapping
`<span>` at all — its `Value` is written straight into the `<pre>` as
escaped text, styled by nothing but the surrounding monospace font. This is
the deliberate answer to "what happens to token types outside it": nothing
happens to them, by design, rather than a seventh bucket being invented to
give them a color no reader asked for. It is also what closes the specific
`TextWhitespace: #ffffff` case the probe surfaced: whitespace tokens land in
the uncolored fallback, so there is no code path in this renderer through
which whitespace can be assigned a background color at all — not "assigned
the right one," structurally incapable of being assigned one.

Colors for the six classes are defined once, in a `<style>` block embedded
in the page, using CSS custom properties so a `prefers-color-scheme: dark`
media query can redefine the same six names without touching the class
mapping or the token loop:

```css
:root { --c-kw:#a626a4; --c-str:#50a14f; --c-cm:#a0a1a7; --c-nm:#383a42; --c-num:#986801; --c-pn:#383a42; }
@media (prefers-color-scheme: dark) {
  :root { --c-kw:#c678dd; --c-str:#98c379; --c-cm:#5c6370; --c-nm:#abb2bf; --c-num:#d19a66; --c-pn:#abb2bf; }
}
.c-kw{color:var(--c-kw)} .c-str{color:var(--c-str)} .c-cm{color:var(--c-cm)}
.c-nm{color:var(--c-nm)} .c-num{color:var(--c-num)} .c-pn{color:var(--c-pn)}
```

This is smaller than emitting a `style` attribute per token — six rules
instead of one per span — and it is the only way light/dark theming happens
without asking the one script this page ships
([§5](#5-the-script-what-it-may-touch-and-what-it-must-never-touch)) to
detect a preference and swap a stylesheet: nothing in
[§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes)'s three
responsibilities is "watch for a preference change," so the swap has to be
something the browser already does unprompted, which `prefers-color-scheme`
is.

### 6.3 Language inference

A `code` field carries no language of its own — `code`
([review-document.md §6.1](../review-document.md#61-code-before-and-after))
is soft, illustrative, and unstructured by design. The only signal
available is the owning comment's `anchors[].file`, via `lexers.Match(path)`,
which chooses a lexer from the file extension. Three cases the request
explicitly asks not to be left open:

- **No anchor at all.** An architectural finding may carry zero anchors
  ([review-document.md §5](../review-document.md#5-anchor-object)). With
  nothing to infer from, the excerpt renders through `lexers.Fallback` —
  chroma's plaintext lexer, which never returns nil and tokenizes
  everything as one undifferentiated run of `Text`. The excerpt still
  displays, in the monospace `<pre>` block, with no color at all — a
  strictly worse-looking but never broken result.
- **Several anchors, naming different languages.** `code` is one excerpt
  attached to the comment as a whole, not to any one anchor, so there is no
  per-anchor language to pick between even in principle. The decision is to
  use the **first anchor in document order** — `anchors[0].file` — and
  nothing else. First, not "most common extension" or some other inferred
  rule, because the reviewer wrote the anchors in an order, and the first
  one is the one most likely to be the excerpt's actual home; a rule that
  tried to be cleverer than "first" would be inferring intent the document
  never stated. `suggestion.code` — the "after" excerpt, which carries no
  anchors of its own at all
  ([review-document.md §6.1](../review-document.md#61-code-before-and-after)) —
  uses the **parent comment's** first anchor for the identical reason: it
  is the only file identity available, and the excerpt is illustrating a
  change to code the comment already anchored.
- **An extension chroma doesn't know.** `lexers.Match` returns `nil` for an
  unrecognized extension; the renderer treats `nil` identically to "no
  anchor," falling back to `lexers.Fallback`. No error, no warning line in
  the output — an unrecognized extension is not a defect in the review, and
  [§12](#12-failure-and-degradation) is explicit that this renderer never
  invents a failure mode `collect-reviews`'s JSON form doesn't already have.

**The renderer does not surface chroma's own lexer name to the reader.**
The obvious label — the matched lexer's own `Config().Name`, what the
prototype used — is unsafe: chroma's `.sql` lexer reports its
`Config().Name` as `"MySQL"` unconditionally, so a SQLite schema
(`internal/store/sql/schema.sql`, this project's own) renders a block
labeled `MYSQL` — a wrong, confident-sounding claim for the one extension
this project has direct proof of, with no reason to expect `.sql` is the
only such case across chroma's lexer set. **Decision: never display
`Config().Name`.** When an anchor exists, the label is the file's own
extension instead — `.sql`, not `MySQL` — read directly off
`anchors[0].file`, the same field that selected the lexer, never off
anything chroma computed from it: a fact about the filename the reviewer
wrote, verifiable against the anchor line already on the page, rather than
a fact about chroma's lexer catalog leaking onto it. With no anchor to
read an extension from, the block carries no label at all — the same
absence-over-invention posture [§12](#12-failure-and-degradation) already
takes for a missing `code` excerpt, extended one level in.

### 6.4 The dependency this costs

Chroma pulls exactly one runtime transitive dependency:
`github.com/dlclark/regexp2/v2`, used internally by several of chroma's
lexers for backreference and lookaround support Go's own `regexp` package
doesn't provide. This project's `go.mod` lists seven direct requirements
today, a number [cli.md §7.3](../cli.md#73-dependencies) treats as
deliberate — "keep the tree shallow — this binary is invoked in tight
loops." Adding chroma adds exactly **one** line to that list: chroma
itself. `regexp2` does not join it — `go.mod`'s indirect block still marks
`github.com/dlclark/regexp2/v2 // indirect`, because nothing in this
module imports it directly; chroma depending on it internally does not
promote it, and `go mod tidy` has no reason to move a package no package
in this module names in an `import` statement. An earlier draft of this
section assumed the opposite — that a direct dependency's own transitive
requirements ride along with it into the direct block — which is not how
Go's module tooling works: promotion follows the import graph this
module's own code forms, not the one chroma forms internally.

A second line joins it from an unrelated direction, and it is not
optional to mention just because it isn't chroma's fault.
[§2.2.1](#221-how-html-output-is-tested)'s structural assertions parse
this renderer's output with `golang.org/x/net/html` rather than a regular
expression. `golang.org/x/net` is already in this module's graph today —
`go.mod` carries it at v0.57.0, marked `// indirect`, pulled in by
something else in the tree, not by this feature. But that marker means
only "no package in this module imports it directly yet," and a test
that does import `x/net/html` directly is enough to end that: `go mod
tidy` promotes the requirement from the indirect block to the direct one
the moment any package in the module — a test package included — imports
it, regardless of what else already depended on it transitively. This
feature doesn't add `x/net` to the module graph; the graph already had
it. What this feature adds is the direct import that moves its existing
line, and `go.mod`'s direct-requirement list does not have a way to
record "already present, just not like this" — a requirement is either
direct or it isn't.

This is worth stating as a real cost rather than waving past, and worth
distinguishing from a dependency already in the tree that looks similar in
size but isn't the same kind of cost: `github.com/sqlc-dev/sqlc` is a build-
time code generator, tracked in `internal/tools/tools.go` behind the
`tools` build tag ([cli.md §7.3](../cli.md#73-dependencies)) — it is never
imported by code that ships in the binary, and its own enormous transitive
tree (a SQL parser, a Postgres driver, a MySQL driver, a CEL evaluator)
never executes when a user runs `loam-refinery`. Chroma is not that kind of
dependency, and it is a narrower claim than "the HTML path imports it" to
say why. `internal/render/html_highlight.go` imports
`github.com/alecthomas/chroma/v2/lexers` at file scope, and `internal/cli`
imports `internal/render` for the JSON and Markdown paths too, not only
HTML's ([collect_reviews.go](../../internal/cli/collect_reviews.go),
[interfaces.go](../../internal/cli/interfaces.go),
[reviews.go](../../internal/cli/reviews.go)). That import graph means
chroma's package initializer — `lexers.GlobalLexerRegistry`, which globs
and parses every embedded `.xml` lexer definition into memory — runs once
per process, on every `loam-refinery` invocation, `submit-review` included,
regardless of `--format`; it is not gated behind the flag the way the
tokenizing work itself is. Calling chroma's `Tokenise` on a specific
`code` excerpt *is* confined to the one code path that needs it — it only
happens when a comment carries a `code` field and `--format html` was
asked for — but the package-load cost, and the binary-size cost of
shipping chroma at all, are paid by every invocation regardless of format.
[cli.md §7.3](../cli.md#73-dependencies) records the binary-size figure
this costs. `x/net/html` sits closer to `sqlc` than to chroma on
that axis — [§2.2.1](#221-how-html-output-is-tested)'s parser runs only
inside `go test`, never inside a shipped `loam-refinery` binary — but it is
not tracked behind the `tools` build tag the way `sqlc` is, because it is
not a code generator invoked once at build time; it is an ordinary test
import, and Go's module tooling makes no distinction between a test-only
direct import and a runtime one when deciding what belongs in the direct
block. The line in `go.mod` reads the same either way, which is exactly
why it has to be counted the same way. The honest accounting is: this
feature costs two new lines in `go.mod`'s direct-requirement list —
chroma for the highlighting path, `x/net` for the test path that checks
it. `regexp2` is not a third: it ships in the binary as chroma's own
transitive dependency, but no line for it moves in `go.mod`, because
nothing in this module imports it by name — see above. `x/net`'s cost is
not the same kind of cost as chroma's, in the other direction: it never
executes in the shipped binary, but the `go.mod` line itself is paid by
every build of this module the moment the test that imports it exists,
flag or no flag, because a direct requirement is a property of the
module's dependency list, not of what one particular invocation happens
to touch.

## 7. Layout: effective use of horizontal space

The page uses a wide, centered container — generous enough to hold a
two-column suggestion comparison and an unwrapped code line, not the
narrow, prose-width column a typical single-essay markdown-to-HTML render
would use. The report is a data artifact with tables, code, and paired
options in it, not an essay, and a column tuned for reading paragraphs is
the wrong shape for any of those three.

```
┌──────────────────────────────────────────────────────────────────────┐
│ collect-reviews: 4f2c1a9…  repo: …/loam-refinery (known)             │
│ store: enabled  head_check: repo, is_head, 0 diverged  total: 2      │  envelope strip
├──────────────────────────────────────────────────────────────────────┤
│ #1 backend   request_changes  mixed   max=9 must-fix=1               │
│ #2 security  comment          strong  none                           │  submissions index
├──────────────────────────────────────────────────────────────────────┤
│ ▼ Dropped user context in the retry loop  `dropped-context-1`  P9    │
│   body …                                                              │
│   ┌──────────────────────────────────────────────────────────┐      │
│   │ colorized code, horizontal scroll if a line is too long   │      │  one finding
│   └──────────────────────────────────────────────────────────┘      │
│   ┌─────────────────────┐  ┌─────────────────────┐                   │
│   │ suggestion 1         │  │ suggestion 2         │                  │  suggestions,
│   │ pros | cons          │  │ pros | cons          │                  │  side by side
│   └─────────────────────┘  └─────────────────────┘                   │
│ ▶ Input not validated before query  `…validated-2`  P5  (collapsed)  │
└──────────────────────────────────────────────────────────────────────┘
```

### 7.1 The envelope strip

`ref`, `repo`, `store.enabled`, `head_check`, `total`, `unreadable` — the
same fields [combined-reviews.md §8.1](combined-reviews.md#81-shape) puts
on the JSON envelope and the markdown renderer's header line already
carries — render as one full-width strip using flexbox, wrapping onto a
second line only at narrow viewports. This is the one section never
collapsed: it is small, it is what orients a reader before anything else,
and hiding it behind a click would spend the reader's first action on
un-hiding information the whole rest of the page assumes they already have.

### 7.2 The submissions index

One row per submission, laid out as a CSS grid so `ordinal`, `profile`,
`verdict`, `assessment`, and `severity` sit in fixed columns across the full
width — the same information
[combined-reviews.md §8.1](combined-reviews.md#81-shape)'s worked example
and the markdown renderer's bullet list already carry, given a table's
horizontal layout instead of a list's vertical one, because five short
fields read faster as aligned columns than as five labels in a row of
prose, and there is width to spare for it. `severity` renders as
[markdown.go](../../internal/render/markdown.go)'s `formatSeverity` already
formats it — `max=9, must-fix=1` — not reinvented as a chart; a bar or
sparkline is exactly the kind of visual embellishment this document
declines to add for four numbers a reader can read as text faster than a
chart, and per-submission color already comes from the verdict badge, not
from a second graphic. Never collapsed, for the same first-thing-a-reader-
needs reason as [§7.1](#71-the-envelope-strip).

### 7.3 Findings, and what collapses by default

Every comment is one `<details>` element, but what heads its `<summary>` is
not the qualified id. A comment carries no title field to head it with in
the first place — [review-document.md §2](../review-document.md#2-concepts)
lists exactly what one carries: a reviewer-authored id, a priority, a
category, a body, zero or more anchors, zero or more suggestions, and
nothing else — so any renderer that wants a headline has to derive one,
and this renderer's first draft derived it from the field that reads most
like a label: the id itself, `backend:dropped-context-1` and its kin,
set as the `<summary>` text. A working prototype built against that draft
is what settled the question the other way. The id repeats what
[§7.2](#72-the-submissions-index)'s profile column already says one row
up, so putting it in the title again says nothing new; and a slug is not
what a reader scanning a findings list needs first — it names which
reviewer and which sequence number filed a finding, never what the
finding actually says. The id itself should not be the emphasis.

**The `<summary>`'s emphasis is the finding itself: the first sentence of
`body`, verbatim reviewer text, truncated at a word boundary to a
140-rune cap.** Truncation is a cut, never a rewrite — nothing here
invents, summarises, rewords, or reorders reviewer prose, and the full
`body` still renders unabridged in the open comment below, unchanged. An
ellipsis appears only when the cut actually happened: a first sentence
that already fits under the cap renders whole, with no trailing `…`
implying a truncation that never occurred. The one exception to cutting
at a word boundary is a single unbroken token — an identifier, a URL —
longer than the cap by itself; that token is cut mid-token rather than
either overflowing the summary uncut or vanishing outright, since there
is no earlier word boundary to cut it at.

The qualified id does not disappear, it moves: it renders as a small
monospace chip beside the headline, still the element's `id` attribute
and still the anchor a copied link resolves to
([§8.1](#81-the-id-attribute-the-qualified-id-verbatim-no-substitution)),
carrying the full qualified id again in a `title` attribute so hovering
it — or a screen reader that surfaces `title` — still gives the exact
string a reader might need to search for or paste elsewhere. Nothing
becomes unaddressable; the id just stops being the first, or the only,
thing a reader reads. The profile is not folded into this chip, or into
the headline, at all — it stays its own tag, the same one
[§7.2](#72-the-submissions-index)'s submissions index already renders,
which is exactly what makes an id-as-title redundant in the first place:
a `backend:` prefix on a headline says nothing a `backend` tag one
element over hasn't already said. For an ordinal-qualified finding
([§8.2](#82-the-href-percent-encoded-and-why-thats-the-reversal)) the
chip shows the origin id alone — `dropped-context-1`, not
`#3:dropped-context-1` — and the ordinal renders as its own tag next to
where the profile tag would sit, the same separation of "what this is"
from "which submission it came from" the profile tag already draws for
the profile-qualified case.

The absence this works around is worth naming rather than quietly
routing past:
[review-document.md §2](../review-document.md#2-concepts)'s comment
object has no title field, and every renderer that wants a headline —
this one included — has to derive one from `body` because there is
nothing else to derive it from. That absence is a property of the review
document format itself, and fixing it is out of scope here; what belongs
in this document is only that this renderer's derivation stays
disciplined about the gap — a truncation of what a reviewer actually
wrote, never a paraphrase of it.

A report where every finding starts open is a wall of text a reader has
to scroll past to get anywhere; one where every finding starts closed
makes the reader click through every row before seeing a single body.
Neither default survives contact with what a reader actually does first,
which is: skim the submissions index, then open the findings that matter
before the ones that don't.

**Default open: priority 7 and above** — the `should-fix` and `must-fix`
bands [review-document.md §8](../review-document.md#8-priority) already
defines, the identical boundary
[combined-reviews.md §8.1](combined-reviews.md#81-shape)'s `severity.max`
computation already uses. **Default closed: priority 6 and below.** This
reuses a boundary the codebase already computes rather than inventing a new
one, and it matches what a reader who just read the submissions index'
severity summary does next — opens the alarming findings, defers the
optional ones. As [§2](#2-the-amendment-this-falls-under) states, this
default is a pure function of `Comment.Priority`, a field the JSON form
already serializes; nothing about which findings collapse is decided by a
second trip to the store, and the same report rendered twice from an
unchanged `collect.Result` opens exactly the same set of `<details>`
elements both times.

A finding a caller reaches by a direct link — the routing case
[combined-reviews.md §6.2](combined-reviews.md#62-routing-feedback-back-to-a-reviewer)
exists for — opens regardless of this default, by the anchor-routing script
[§5.3](#53-what-the-script-does) describes, which walks every `<details>`
ancestor of the fragment's target open on load and on `hashchange`. That
routing is what makes a closed default safe to have at all: without it, a
direct link to a finding this section defaulted closed would land the
reader on an invisible target, exactly the gap
[§5](#5-the-script-what-it-may-touch-and-what-it-must-never-touch) opens
with. With JavaScript disabled, that gap reopens — the browser still
scrolls to the target, `open` still does not flip — which is one more
reason [§5.2](#52-progressive-enhancement-is-mandatory-not-a-nicety)'s
floor matters: a reader without script sees every finding already present
in the DOM, so a fragment link that fails to open anything still lands
somewhere the content is already sitting on the page, not somewhere blank.
The default only governs what a reader sees on first load with no fragment
in the URL.

Suggestions inside an open comment render open, unconditionally — a
collapsed comment already hides them, and a comment a reader chose to open
is a comment they came to read, suggestions included.

### 7.4 Suggestions as cards, pros and cons side by side

[review-document.md §6](../review-document.md#6-suggestion-object) frames
`suggestions` as a menu — "the useful case is offering a consumer a
choice" — and the markdown renderer necessarily presents that choice as a
vertical bulleted list, one suggestion after another, because CommonMark's
list model has no native side-by-side layout. HTML does. Each comment's
suggestions render as a row of cards using CSS grid
(`grid-template-columns: repeat(auto-fit, minmax(320px, 1fr))`), so two or
three competing options sit next to each other instead of stacked — the
comparison the format has always modeled in data, finally laid out as a
comparison. Within one card, `pros` and `cons` sit in their own two-column
sub-grid rather than one list under the other, so a reader's eye moves
left-right between "why" and "what it costs" instead of scrolling down past
every pro before reaching the first con. `effort` and `scope` — both
structurally-constrained enums
([combined-reviews.md §8.3.2](combined-reviews.md#832-escaping-and-fencing-caller-authored-text))
— render as small badges on the card header, not prose, for the same
skimmability reason `severity` renders as a compact string rather than a
paragraph. A suggestion's own `code` excerpt, when present, sits at the
bottom of its card, in the same colorized, horizontally-contained block
[§7.5](#75-code-blocks-that-dont-widen-the-page) specifies for every other
code excerpt on the page.

### 7.5 Code blocks that don't widen the page

Every `<pre>` this renderer emits — `comment.code`, `suggestion.code` — sets
`overflow-x: auto` on the block itself and `white-space: pre` on its
content, inside a container whose own width never exceeds its parent's. A
line too long to fit scrolls **within its own block**, with its own
horizontal scrollbar, never forcing the page itself to grow wider than the
viewport. This is a hard requirement stated once, here, because a single
unbounded-width code block is exactly the kind of thing that silently
breaks "effective use of horizontal space" for the entire page around it —
one wide `<pre>` with no `overflow-x` constraint stretches its ancestor
`<details>`, its ancestor comment section, and the page body itself, and
every other section's careful column layout goes with it. `max-width: 100%`
on the block and `overflow-x: auto` are the two declarations that prevent
that, and nothing about the rest of the layout works if either is dropped.

## 8. Anchors, ids, and what a copied link contains

[combined-reviews.md §6.1](combined-reviews.md#61-the-qualified-id)'s
qualified id is the natural choice for the HTML `id` attribute on each
comment's `<details>` element — it is already globally unique across one
combined output, already the thing a reader and an orchestrator both refer
to a finding by, and turning it into a fragment link is exactly what makes
[§6.2](combined-reviews.md#62-routing-feedback-back-to-a-reviewer)'s routing
case work in a browser. But the two qualifier forms
[§6.1](combined-reviews.md#61-the-qualified-id) defines don't survive
becoming a URL fragment identically, and the `id` attribute and the `href`
that points at it turn out not to need the identical treatment either.

### 8.1 The id attribute: the qualified id, verbatim, no substitution

**The HTML `id` attribute is always exactly the qualified id
[§6.1](combined-reviews.md#61-the-qualified-id) assigns the comment —
`backend:dropped-context-1`, or `#3:dropped-context-1` with its leading
`#` intact — never modified, never substituted, in either form.** This is
possible because the HTML specification's `id` attribute grammar permits
any character at all except ASCII whitespace, and requires only that the
value be non-empty and unique on the page; `#` and `:` are both perfectly
legal there; nothing about placing `id="#3:dropped-context-1"` on a
`<details>` element is invalid HTML, unusual as it looks next to a
CSS-selector intuition that reads a leading `#` as special. This single
decision is what keeps every representation of an id — JSON's
`comments[].id`, Markdown's heading text, and now the HTML `id` attribute —
spelled identically: [§2.2](#22-the-tests-that-make-the-claim-true-not-merely-stated)'s
parity test asserts plain string equality against `comments[].id`, with no
encoding or substitution to undo first, because there is none to undo.

### 8.2 The href: percent-encoded, and why that's the reversal

The `href` that links to that same `id` is a different story, because a
raw `#` cannot appear unencoded inside a URL fragment — RFC 3986's fragment
grammar is `*( pchar / "/" / "?" )`, and `#` is in none of those sets.
`href="##3:dropped-context-1"` mostly works while a reader stays inside the
page, since browsers are lenient about matching a raw fragment against an
element id, but the moment a reader copies the resolved address — "copy
link," or the address bar after following it — URL serialization
percent-encodes the stray `#`, landing `report.html#%233:dropped-context-1`
on the clipboard, not obviously derivable by eye from what the page
displayed. `:`, by contrast, *is* in `pchar` and needs no encoding in
either qualifier form.

**An earlier draft of this section proposed substituting `@` for the
leading `#` in both the `id` and the `href`, trading byte-identity for
legibility. That decision is reversed: the `href` percent-encodes the `#`
instead, deliberately, and the `id` attribute is left alone entirely, per
[§8.1](#81-the-id-attribute-the-qualified-id-verbatim-no-substitution).**
A copied link to `#3:dropped-context-1` now reads
`report.html#%233:dropped-context-1` — exactly what a browser's own
fragment-serialization would have produced from the raw form anyway, made
deliberate instead of incidental, and, critically, the encoding a reader's
browser still resolves correctly with
[§5.2](#52-progressive-enhancement-is-mandatory-not-a-nicety)'s script
disabled: percent-encoding is native fragment-navigation behavior the
platform already understands, where `@`-substitution depended entirely on
this renderer's own script recognizing `@` as an alias and mapping it
back. Percent-encoding needs no such reversal: the id and the href that
points at it already agree, byte for byte, once the one standard escape
is decoded, so [§5.3](#53-what-the-script-does)'s routing script has
nothing to reconcile and carries no fallback attribute to do it with — no
`data-alt-id`, no second id to map the first one onto. A page whose
correctness depended on script recognizing and reversing a substitution,
when a native encoding already resolves the same link without either,
would have been choosing the weaker property on purpose.

**What the decision actually rests on does not depend on why an id
ended up ordinal-qualified, and that is worth stating plainly rather than
leaning on a characterization that only half holds.**
[§6.1](combined-reviews.md#61-the-qualified-id) uses the ordinal form in
exactly two cases: a submission with no `profile` at all, or a submission
that carries `superseded_by`. Both occur in real data, and they are not
the same kind of finding. In one collected panel (`f6951ff`), all six
submissions carried a profile, and every ordinal-qualified id present came
from the second case alone — submission #3 (`profile: "go"`,
`superseded_by: 4`), superseded by a later `go` resubmission. In another
(`10974f7`), two of seven submissions carried no `profile` at all, and the
ordinal ids in that panel's output came from those instead. The
"stale half of a revise-and-resubmit" characterization is accurate for the
first case — a superseded submission's findings are answered by a
different, profile-qualified id belonging to whatever replaced it, and an
orchestrator routing feedback back to a reviewer
([§6.2](combined-reviews.md#62-routing-feedback-back-to-a-reviewer)) wants
that current id, not the stale one. It gives **no comfort at all** for the
second case: a submission with no `profile` is not stale, it is current,
and if a real reviewer simply ran without `--profile`, every finding that
reviewer filed is ordinal-qualified and every one of them is a live
finding someone may well want to deep-link. Treating "ordinal-qualified"
as a synonym for "low-value link target" would be wrong for exactly that
submission.

That is why this decision does not lean on which case produced the id.
Percent-encoding wins on two properties that hold regardless of cause: it
keeps **one spelling of an id** across JSON, Markdown, and HTML — the same
byte sequence `comments[].id` carries, recoverable from the href by
decoding a single, standard, three-character escape rather than reversing
a bespoke convention this renderer invented — and it is the form that
**still resolves with the script disabled**
([§5.2](#52-progressive-enhancement-is-mandatory-not-a-nicety)), because
percent-encoding is native fragment-navigation behavior the platform
already understands, where `@`-substitution depended entirely on this
renderer's own script recognizing `@` as an alias and mapping it back.
Both properties are unconditional: they hold for the superseded case, and
they hold just as well for the profile-less case the "stale finding"
framing does not cover. What an ordinal-qualified id names in practice is
worth knowing — it says this choice is low-stakes on average, since the
common case in a well-run panel is the superseded one — but it is colour,
not support: the decision would be identical if every ordinal id in every
panel turned out to name a live, unprofiled submission's findings instead.

### 8.3 `template.URL`: the one deliberate bypass, and why it's safe here

Producing exactly that `href` — the profile-qualified form byte-identical
to the `id` attribute, the ordinal form identical except for its leading
`#` becoming `%23` — turns out not to be `html/template`'s default
behavior in either of the two obvious template shapes, and this was
verified directly against the Go standard library rather than assumed.
With `id := "tests:band-sub-one-untested-1"`:

- `<a href="{{.}}">`, the whole attribute as one dynamic pipeline, renders
  `href="#ZgotmplZ"` — `html/template`'s `urlFilter`, the same machinery
  that blocks a `javascript:` scheme, reads the text before the first `:`
  as a URL scheme and refuses it. Every qualified id contains exactly one
  colon, so this shape is unusable for any id at all, not only the
  ordinal-qualified ones.
- `<a href="#{{.}}">`, a static `#` prefix with the id as a plain dynamic
  suffix, avoids `urlFilter` (the attribute is no longer *only* a dynamic
  pipeline) but still runs the default URL-context escaper on the suffix,
  which renders `href="#tests%3aband-sub-one-untested-1"` — the colon
  percent-encoded, which is correct for nothing this section wants: it
  breaks the profile-qualified case's byte-identical claim
  ([§8.1](#81-the-id-attribute-the-qualified-id-verbatim-no-substitution))
  and it still would not, on its own, produce the ordinal case's
  narrower "encode only the illegal `#`" rule.
- The identical `<a href="#{{.}}">` shape, with the dynamic value wrapped
  in `template.URL`, renders the exact, unencoded id.

**`template.URL` is therefore required**, and it is the single deliberate
bypass of the escaping mechanism [§4](#4-escaping-html-template-never-hand-rolled)
and [§6](#6-syntax-highlighting-chroma-token-api-only) argue for everywhere
else on this page. It is confined to exactly one helper function, which
computes the fragment value itself — percent-encoding only the ordinal
form's leading `#`, passing every other character through untouched — and
hands `html/template` the finished string as `template.URL`, trusted
verbatim. This is safe specifically because the value passed to
`template.URL` is never reviewer prose: it is built exclusively from the
qualifier and origin id [§6.1](combined-reviews.md#61-the-qualified-id)
already constrains structurally — the same "safe by construction" category
[§4](#4-escaping-html-template-never-hand-rolled)'s closing argument
establishes for `id` and `profile` — and this helper is never called with
`Body`, `Summary`, `Pros`, `Cons`, or any other free-text field. A reader
must not come away thinking `html/template`'s autoescaping is optional
anywhere else on this page: it is optional here, for this reason, and
nowhere else.

### 8.4 `anchor.file` spans stay inert

`anchor.file` spans — the inline `file:line` badges the markdown renderer
already sets in a code span
([combined-reviews.md §8.3.2](combined-reviews.md#832-escaping-and-fencing-caller-authored-text)) —
are not link targets at all in this renderer; they are inert text inside a
`<code>` element, escaped exactly like every other verbatim field, per
[§4](#4-escaping-html-template-never-hand-rolled). `loam-refinery` has no
way to know what a reader's local checkout is rooted at, so linking them to
anything — a `file://` URL, a forge's blob viewer — would be inventing an
address this offline, read-only tool ([§9](#9-self-contained-no-network))
has no basis to construct.

## 9. Self-contained: no network, no external assets

The single-file requirement is not only a delivery convenience — it follows
directly from [cli.md §1](../cli.md#1-overview)'s first design principle,
"offline, and read-only about the repository... no network, ever." A report
that referenced a CDN-hosted stylesheet, a web font, or any other external
URL would make opening it in a browser the first time this tool's output
ever reaches outside the machine it ran on — exactly the boundary that
design principle exists to hold. So every asset this page needs is inline:
the `<style>` block ([§6.2](#62-the-class-mapping-and-the-css-palette)) is
embedded, not linked; there is no `<link rel="stylesheet">` to anywhere;
type is set in the browser's own system font stack (`ui-sans-serif`,
`ui-monospace`, and their fallbacks), never a downloaded web font; and the
one `<script>` [§5](#5-the-script-what-it-may-touch-and-what-it-must-never-touch)
now ships is inline, not a `<script src="...">` pointed at anywhere —
self-containment does not mean *no script*, which
[§5](#5-the-script-what-it-may-touch-and-what-it-must-never-touch) already
established is no longer true, it means **the script never fetches, never
loads, and never phones out**: no `fetch`, no `XMLHttpRequest`, no
`WebSocket`, no dynamically inserted `<script>` or `<link>` element, and
nothing in [§5.3](#53-what-the-script-does)'s three responsibilities needs
any of them — routing, expand/collapse, and filtering are all pure DOM
manipulation over content the page already has. One `.html` file is the
entire artifact — copy it, email it, open it from a `file://` URL with the
network disabled, and it renders and behaves identically every time,
because nothing it depends on, script included, is anywhere but inside it.

## 10. Determinism

Two `collect-reviews --format html` calls against an unchanged store, for
the same `--ref`, must produce byte-identical output. This is not a new
requirement this feature invents — it follows from
[§2](#2-the-amendment-this-falls-under)'s "pure projection" claim: a
projection that varies between two calls over the same input isn't pure,
and every promise this document makes about parity and forgery tests
assumes a fixed, reproducible output to test against in the first place.
**This is checked directly, not merely argued for**: a test invokes
`collect-reviews --format html` twice in the same process against one
unchanged store and one `--ref`, and asserts the two byte slices are
identical — not structurally equivalent, not equal after normalizing
whitespace, the same bytes. Concretely, that forbids:

- **Any wall-clock content.** No "generated at" line, no timestamp footer.
  Nothing on the page may read the current time. `submissions[].at` was
  already removed from the envelope entirely for a related reason
  ([combined-reviews.md §8.1](combined-reviews.md#81-shape)) — this
  extends the same abstention to the renderer's own output, not just the
  data it renders.
- **Map iteration.** Every element this renderer emits comes from ranging
  over one of `collect.Result`'s own ordered slices — `Submissions`,
  `Comments`, `Anchors`, `Suggestions` — never from ranging over a Go map.
  Go's `text/template` (and `html/template`, built on it) does sort map
  keys automatically when ranging over one, specifically to prevent this
  class of nondeterminism, but this renderer's design doesn't lean on that
  safety net: nothing in `collect.Result`'s shape is a map to begin with,
  and the class lookup [§6.2](#62-the-class-mapping-and-the-css-palette)
  uses is a fixed switch over a small enum, consulted per token, never
  iterated for output.
- **Any per-run identifier.** No randomly generated element id for a
  `<details>`/`<summary>` pair — every id this renderer emits is derived
  from a comment's own qualified id ([§8](#8-anchors-ids-and-what-a-copied-link-contains)),
  never from a counter seeded by anything but array position, and array
  position is itself fixed by
  [combined-reviews.md §8.1](combined-reviews.md#81-shape)'s ordering
  rules for the same store state.
- **Nothing about the script varies, because nothing about it is
  computed.** [§5.1](#51-the-contract-one-static-script-zero-interpolated-bytes)'s
  zero-interpolated-data contract means the `<script>` element's bytes are
  a fixed constant this renderer emits unchanged on every call — the same
  property [§2.2.1](#221-how-html-output-is-tested)'s script-purity pin
  checks statically, and determinism gets it for free as a consequence
  rather than needing its own separate check: a value that is never
  computed from input cannot vary between two calls over different input,
  let alone the same input twice.

What is explicitly *not* forbidden: the page may embed `loam-refinery`'s own
version string, the same static, per-binary value `version`
([cli.md §2.5](../cli.md#25-version)) already prints — that value is fixed
for a given binary and varies only across binaries, which is a different
axis from "varies between two runs of the same binary against the same
store," the one this section is actually about.

## 11. Budgets and audience

[cli.md §6.1](../cli.md#61-budgets) already carries one exempt row —
`collect-reviews --format markdown | unbudgeted | Rare; human reading, or
embedding somewhere a human reads it.` HTML needs its own row, and it needs
to be exempt more emphatically than unbudgeted, because unbudgeted still
reads, on a page dense with ceilings, as an oversight next to a table whose
entire point is that nothing escapes measurement. Stated plainly instead:

| Call | Budget | Frequency |
| --- | --- | --- |
| `collect-reviews --format html` | **exempt — not agent-facing** | Rare; opened in a browser by a person |

`--format html` is for a human looking at a browser, full stop, and that
has to be said rather than implied by silence, because silence next to a
budget table reads as "nobody thought about this one" rather than "this one
was deliberately excluded." Restating the three-way split from
[§1](#1-what-this-adds): **JSON** is for a machine — an orchestrator,
another tool in a pipeline, anything that parses the output and acts on it
programmatically, and it is the only form anything automated should ever
consume, per
[combined-reviews.md §8.3.3](combined-reviews.md#833-the-test-that-pins-it)'s
existing restriction extended to a second non-JSON form. **Markdown** is for
an agent in a loop, or a human reading pass-through content a loop
produced — a PR comment, a chat message — read once, by a person, but
generated by, and often consumed adjacent to, an automated process.
**HTML** is for a person, directly, with a browser open, and never for
anything else: nothing should generate it inside a loop that expects to
read the result back, nothing should feed it to a model as context instead
of the JSON form it was built from, and no future feature should add a flag
that parses one back into structured data — the same restriction
[combined-reviews.md §8.3.3](combined-reviews.md#833-the-test-that-pins-it)
states for markdown applies here for the identical reason, one grammar
over.

### 11.1 Measured size

Exempt from a token budget does not mean nobody should know what this
costs, and guessing invites optimizing the wrong part of it later. This
section has now carried three generations of measured-size figures — a
prototype build's, then the shipped renderer's, each replacing the last
because nothing kept either one true — and it does not restate a fourth
set of exact bytes here for the same reason the second set went stale
before this sentence was written: none of them is checked by a test, and
this renderer's own syntax-highlighting path was under active revision at
the time of this correction. `refinery-t1c.13` proposes checking in a
sized fixture and pinning the two costs that are actually fixed and
content-independent — the `<style>` and `<script>` block byte lengths —
the way `internal/cli/budget_test.go` already pins `describe` and clean
`submit-review`'s token counts; once that lands, this section can cite the
pinning test by name instead of a number nothing enforces.

**The panel this section used to cite — "the same 7-submission,
36-comment panel §2.2's tests already use as a fixture" — was never a
fixture that exists in this repository.** [§2.2](#22-the-tests-that-make-the-claim-true-not-merely-stated)'s
parity, fidelity, and forgery tests run against `docs121Envelope`, which
carries 2 submissions and 2 comments; no 36-comment fixture is checked in
anywhere for a reader to re-measure against. Whatever produced that
number was a one-off, unrecorded measurement, not a fixture this section
can point to — which is exactly the gap `refinery-t1c.13` closes going
forward.

What is true regardless of the exact byte counts, and does not need
re-measuring to state: the CSS and script are fixed costs, paid once per
page no matter how many findings it holds, because neither is generated
per finding; visible review text — `body`, `summary`, `pros`, `cons`,
suggestion text — is the one thing that scales with the review itself,
and the one thing this renderer has no license to trim, elide, or
summarize, since every byte of it is what the reviewer wrote and
[§4](#4-escaping-html-template-never-hand-rolled)'s whole argument depends
on carrying it through unmodified. HTML costs more bytes than the Markdown
projection of the same content — real, per-finding structure (`<details>`,
badges, suggestion cards) that Markdown expresses more cheaply with
bullets and headings — but this section no longer states a multiplier,
because the true one moves with decisions still being made in
[§6](#6-syntax-highlighting-chroma-token-api-only) and depends on the
panel measured. To see the actual current cost of a report, render one
and measure it directly: `collect-reviews --format html --ref=SHA |
tee report.html | wc -c`, and `gzip -c report.html | wc -c` for the
compressed size — a number read off the binary a reader is actually
running is worth more than one copied from this paragraph.

## 12. Failure and degradation

This renderer invents no new failure mode `collect-reviews`'s existing
result value doesn't already carry — it mirrors
[combined-reviews.md §8.1](combined-reviews.md#81-shape) and
[§8.3](combined-reviews.md#83-the-markdown-projection)'s own choices rather
than making fresh ones:

| Case | Behavior |
| --- | --- |
| An unreadable stored submission | Counted in the envelope's `unreadable` figure ([combined-reviews.md §9](combined-reviews.md#9-empty-and-failure-cases)), same as JSON and markdown — never a broken or partial page, never a silently dropped row a reader has no way to notice. |
| A submission with no comments (`approve`, filed clean) | A row in the submissions index, `severity` rendered as `none` (`formatSeverity`'s own treatment of `Severity.Max == nil`), no entries in the findings section — there is nothing to collapse or expand for a submission that filed nothing. |
| A submission with no `assessment` | Renders as `(none)`, the identical placeholder the markdown renderer already uses for the same absent-field case — never one of the four real grade words standing in for silence. |
| A comment with no anchors | The anchors badge row renders `(none)`, matching `markdownAnchors`'s existing treatment; [§6.3](#63-language-inference) covers what that does to code highlighting specifically. |
| A comment with no `code` excerpt | No code block rendered at all — not an empty `<pre>`, which would visually claim there was something to show. |
| `head_check.source` is `none` or `unavailable` | Rendered in the envelope strip exactly as reported, the same honesty [cli.md §5.2](../cli.md#52-the-result-object)'s `verification.source` and `combined-reviews.md`'s `head_check.source` already require — never silently omitted, never presented as if verification ran. |
| An empty store, or a ref with zero submissions | A well-formed page: the envelope strip, an explicit "no submissions found for this ref" empty state, no findings section. Not an error, matching [combined-reviews.md §9](combined-reviews.md#9-empty-and-failure-cases)'s exit-0 treatment of the identical case in JSON and markdown. |

Every row above resolves to a decision `collect-reviews` already made for
its other two output forms. Nowhere does this renderer choose to fail, warn,
or omit a case they don't; the exit-code table
([combined-reviews.md §9](combined-reviews.md#9-empty-and-failure-cases))
is unchanged by this feature, because nothing about which case a run falls
into depends on which `--format` was asked for.
