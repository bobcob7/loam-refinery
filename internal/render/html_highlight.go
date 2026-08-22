// internal/render/html_highlight.go — chroma's token API, used directly
// rather than chroma's own HTML formatter (§6.1: wrapping formatters/html's
// output in template.HTML would trust caller-influenced source text as
// markup, exactly the hole §4 exists to close). lexers.Match(anchor file),
// falling back to lexers.Fallback (§6.3), tokenizes a code excerpt; a
// fixed switch maps each chroma.TokenType to one of the six §6.2 CSS
// classes, using InSubCategory — never InCategory, which cannot tell
// LiteralString and LiteralNumber apart (§6.2's named trap) — so a
// numeric literal is never silently painted with the string color.
// Only the derived CSS class reaches an HTML attribute; t.Value, the
// source text itself, goes through the template as ordinary escaped text
// content, subject to the identical contextual autoescaping every other
// field on this page already gets.
//
// htmlCodeView itself (Code, Filename) is html_view_finding.go's own
// type (bead .9); Spans and Label are added to it here, as methods in a
// second file of the same package, rather than by adding a template
// FuncMap this file cannot register without editing html.go (out of
// scope, per docs/features/html-report.md's file-ownership map).
// code.gohtml calls {{range .Spans}} and {{.Label}} directly.
//
// Bead .4 owns this file's content.
//
// Bead .11's own additions live here too: highlightTimeout,
// maxHighlightInputBytes, maxHighlightSpans, maxHighlightTokens, and
// deniedLexerNames close the hang and the memory-amplification holes a
// reviewer-controlled excerpt and a reviewer-chosen anchor filename
// otherwise open — see Spans' own comment for how the four combine, and
// each constant's own comment for why it is sized the way it is. Bead
// .12's own addition, trimSyntheticTrailingNewline, undoes a lexer's
// Config().EnsureNL — a second, separate source-mutating knob
// tokeniseOptions cannot reach.
package render

import (
	"errors"
	"path"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// htmlCodeSpan is one rendered run of a code excerpt (§6.2): either a
// styled token run sharing one CSS class, or an unstyled run of
// plain-text token types (Class == "") that code.gohtml writes straight
// into the <pre> as escaped text content, with no wrapping <span> at
// all.
type htmlCodeSpan struct {
	Class string
	Text  string
}

// classFor maps a chroma.TokenType to one of §6.2's six CSS classes, or
// "" for every token type outside them — Text, TextWhitespace,
// Background, Error, Other, and anything else this table doesn't name.
//
// It uses InSubCategory, never InCategory, to tell LiteralString and
// LiteralNumber apart: chroma groups both as subcategories of the same
// 3000-3999 Literal category, so InCategory divides by 1000 and cannot
// distinguish them — chroma.LiteralNumberInteger.InCategory(chroma.LiteralString)
// evaluates true, which would silently paint every numeric literal with
// the string color. InSubCategory divides by 100 instead and is the
// check that actually separates them (§6.2's named trap;
// TestClassForDistinguishesNumberFromString in
// html_highlight_test.go is the pin, not this comment). Keyword, Name,
// Comment, Operator, and Punctuation carry no equivalent sibling clash —
// each occupies its own 1000-wide category with nothing else living in
// it — so InCategory is the correct, and simpler, check for those five.
func classFor(t chroma.TokenType) string {
	switch {
	case t.InSubCategory(chroma.LiteralString):
		return "c-str"
	case t.InSubCategory(chroma.LiteralNumber):
		return "c-num"
	case t.InCategory(chroma.Keyword):
		return "c-kw"
	case t.InCategory(chroma.Comment):
		return "c-cm"
	case t.InCategory(chroma.Name):
		return "c-nm"
	case t.InCategory(chroma.Operator), t.InCategory(chroma.Punctuation):
		return "c-pn"
	default:
		return ""
	}
}

// deniedLexerNames is bead .11's narrow denylist — not a broad allowlist
// of extensions this renderer is willing to highlight. This tool reviews
// whatever repository it is pointed at, so restricting Spans to some
// predicted set of "extensions we actually review" risks silently
// dropping colour for a legitimate language nobody on this list
// predicted; the deadline and the two size caps below already close
// both the hang and the memory-amplification holes for every lexer,
// known-bad or not yet discovered, which is what makes a broad
// allowlist unnecessary rather than merely undesirable.
//
// What this narrow list buys past that general defence: Jungle and
// JSONata are not merely slow on adversarial input, they never return —
// verified directly against chroma v2.27.0, tracing the actual cause
// past the bead's own "backtracks" description. Both grammars have a
// state (Jungle's "var", reached from "instruction") whose fallback rule
// is a zero-width, untyped pop with no other rule matching some
// non-identifier, non-punctuation byte (a bare "!" reproduces it in one
// byte); paired with the state that pushed it also being zero-width,
// the lexer's own position never advances and its own token stream
// never reaches EOF. Denying these two by chroma's own Config().Name —
// read off the lexer chroma.Match already resolved, not off the
// filename — means a .jungle or .jsonata anchor degrades before
// Tokenise is ever called at all, rather than only after
// highlightTimeout elapses.
//
// This does not shrink the binary or the package-init cost a sibling
// bead measured (chroma's own lexers.GlobalLexerRegistry parses every
// one of its 279 embedded lexers' <config> blocks unconditionally at
// package-import time, before highlightLexer runs at all — which lexer
// this function later refuses does not undo work the import already
// did); that cost needs a fix at the import-graph altitude, not this
// one.
var deniedLexerNames = map[string]bool{"Jungle": true, "JSONata": true}

// highlightLexer resolves §6.3's language inference to one chroma.Lexer.
// An empty filename — no anchor to infer from — a filename whose
// extension lexers.Match does not recognize (nil), and a filename that
// resolves to one of deniedLexerNames' own two entries are all treated
// identically: each falls back to lexers.Fallback, chroma's plaintext
// lexer, which never returns nil and never errors. None of the three is
// a defect in the review (§12), so none produces an error or a warning
// line here.
func highlightLexer(filename string) chroma.Lexer {
	if filename == "" {
		return lexers.Fallback
	}
	lex := lexers.Match(filename)
	if lex == nil || deniedLexerNames[lex.Config().Name] {
		return lexers.Fallback
	}
	return lex
}

// Spans tokenizes c.Code through chroma's token iterator only (§6.1) —
// never chroma's formatters/html, which builds a template.HTML-shaped
// string by concatenating markup around the source text itself, caller-
// authored content on a comment.code or suggestion.code field. Each
// token's Value is carried through unmodified; only classFor's derived
// class name reaches an attribute. Adjacent tokens sharing one class are
// merged into a single htmlCodeSpan (§6.2), so a run of N chroma tokens
// with no class change renders as one span rather than N.
//
// A Tokenise error — which lexers.Fallback's trivial plaintext rules
// never produce, and no lexer in ordinary operation is expected to
// either — degrades to one unstyled span carrying c.Code verbatim rather
// than failing the whole page's render (§12: this renderer invents no
// failure mode collect-reviews's JSON form doesn't already carry). Bead
// .11 widens that same degradation to three more cases a reviewer, not
// a bug, controls: an excerpt over maxHighlightInputBytes never reaches
// Tokenise at all; a tokenisation that does not finish within
// highlightTimeout is abandoned; and a tokenisation that finishes but
// produces more than maxHighlightSpans class transitions is discarded
// in favour of the same unstyled fallback. All three, like the error
// case above, keep the excerpt fully readable — only the colour is
// lost — and none of them fail the page's render.
//
// tokeniseOptions, not nil, is passed to Tokenise: chroma's own
// TokeniseOptions.EnsureLF defaults to true whenever options is nil
// (chroma's defaultOptions), which rewrites every "\r\n" and bare "\r"
// in the source to "\n" before a single token is emitted — verified
// directly, not assumed, because §6.1's own pseudocode calls
// Tokenise(nil, source) and that call silently drops the exact bytes
// this section argues a token's Value must carry through unmodified.
// Passing EnsureLF: false is what keeps a Windows-authored excerpt's
// carriage returns in the token stream, and is what makes the fidelity
// property §2.2's test 2 and this file's own
// TestSpansConcatenationReproducesSourceByteForByte actually hold.
//
// trimSyntheticTrailingNewline undoes bead .12's own knob —
// Config().EnsureNL, which TokeniseOptions.EnsureLF above cannot
// reach — before merging, so the byte-fidelity property this comment
// just named holds for a .diff (or Bash Session, console,
// shell-session, udiff) anchor too, not only for a filename whose
// matched lexer happens to leave EnsureNL unset.
func (c htmlCodeView) Spans() []htmlCodeSpan {
	if len(c.Code) > maxHighlightInputBytes {
		return []htmlCodeSpan{{Text: c.Code}}
	}
	lex := highlightLexer(c.Filename)
	tokens, ok := tokeniseWithinDeadline(lex, c.Code)
	if !ok {
		return []htmlCodeSpan{{Text: c.Code}}
	}
	spans := mergeCodeSpans(trimSyntheticTrailingNewline(tokens, c.Code))
	if len(spans) > maxHighlightSpans {
		return []htmlCodeSpan{{Text: c.Code}}
	}
	return spans
}

// highlightTimeout is bead .11's deadline on a single excerpt's
// tokenisation: no chroma lexer, known-pathological or not yet
// discovered, may hold Spans open past this. Reproduced directly
// against chroma v2.27.0: Jungle and JSONata do not just run slowly on
// adversarial input, their own token stream never reaches EOF at all
// (deniedLexerNames' own comment traces the actual cause), so
// tokeniseWithinDeadline is what stands between an excerpt from some
// other, not-yet-swept lexer and the same fate. 2s is comfortably above
// what any well-behaved lexer takes on an excerpt already capped at
// maxHighlightInputBytes, and comfortably below what turns one hostile
// finding into a report a human gives up waiting for.
const highlightTimeout = 2 * time.Second

// maxHighlightInputBytes is bead .11's size cap on the excerpt fed to
// chroma at all: above this, Spans never calls Tokenise and degrades
// straight to one unstyled span carrying c.Code verbatim (§12: degrade,
// never error, never truncate — the excerpt stays fully readable, only
// uncoloured). The bead's own measurement is the reason for the value:
// 1.1 MB and 4.4 MB excerpts on a plain .go anchor amplified roughly
// 13x into a 73.5 MB page at 740 MB peak RSS; capping the input two
// orders of magnitude below that keeps one excerpt's worst-case
// rendered output in the hundreds of kilobytes even at the same
// amplification factor, while sitting far above any excerpt a reviewer
// pastes to illustrate a finding.
const maxHighlightInputBytes = 64 * 1024

// maxHighlightSpans is bead .11's cap on rendered output, independent
// of maxHighlightInputBytes: an excerpt under the input cap that still
// tokenises into an excessive number of class transitions (a
// pathologically fragmented, not necessarily slow, token stream)
// degrades the same way, rather than emitting tens of thousands of
// <span> wrappers — each one markup overhead no byte of the excerpt
// itself accounts for — into the page.
const maxHighlightSpans = 5000

// maxHighlightTokens bounds tokeniseWithinDeadline's own goroutine, not
// Spans' return value: highlightTimeout only bounds how long Spans
// waits, not how long the goroutine it started keeps running after
// Spans has already returned the degraded fallback. For a lexer like
// Jungle whose token stream never reaches EOF, that abandoned goroutine
// would otherwise append to an ever-growing slice for as long as the
// process keeps running — trading a bounded-time failure for an
// unbounded-memory one. Capping the raw token count the goroutine will
// collect before giving up on its own, well above what any well-behaved
// lexer produces from an excerpt already capped at maxHighlightInputBytes,
// closes that off: the goroutine itself terminates instead of merely
// being ignored.
const maxHighlightTokens = 200_000

// errTooManyHighlightTokens is tokeniseWithinDeadline's own internal
// signal that its goroutine gave up after maxHighlightTokens — never
// compared against by a caller, only ever turned into ok == false.
var errTooManyHighlightTokens = errors.New("chroma tokenisation exceeded the token cap")

// tokeniseResult carries tokeniseWithinDeadline's goroutine's outcome
// back over its channel: either every token a completed tokenisation
// produced, or the error (chroma's own, or errTooManyHighlightTokens)
// that means none of them should be trusted.
type tokeniseResult struct {
	tokens []chroma.Token
	err    error
}

// tokeniseWithinDeadline runs lex.Tokenise, and the iteration that
// actually does chroma's regex work, on a separate goroutine, and
// reports ok == false if that has not produced a result within
// highlightTimeout. chroma's own RegexLexer.Tokenise returns almost
// immediately — it builds a LexerState and hands back its Iterator
// method, doing no matching itself — so the goroutine also drives the
// iterator to completion (or to maxHighlightTokens) itself, rather than
// timing only the cheap call and leaving the expensive one unmeasured.
//
// A lexer whose token stream never reaches EOF leaves its own goroutine
// running past highlightTimeout; maxHighlightTokens' own comment is why
// that goroutine still terminates on its own rather than leaking for
// the rest of the process's life.
func tokeniseWithinDeadline(lex chroma.Lexer, code string) ([]chroma.Token, bool) {
	resultCh := make(chan tokeniseResult, 1)
	go func() {
		it, err := lex.Tokenise(tokeniseOptions, code)
		if err != nil {
			resultCh <- tokeniseResult{err: err}
			return
		}
		tokens := make([]chroma.Token, 0, 64)
		for t := it(); t != chroma.EOF; t = it() {
			tokens = append(tokens, t)
			if len(tokens) > maxHighlightTokens {
				resultCh <- tokeniseResult{err: errTooManyHighlightTokens}
				return
			}
		}
		resultCh <- tokeniseResult{tokens: tokens}
	}()
	select {
	case r := <-resultCh:
		return r.tokens, r.err == nil
	case <-time.After(highlightTimeout):
		return nil, false
	}
}

// trimSyntheticTrailingNewline undoes chroma's own Config().EnsureNL
// (bead .12): a lexer's config, not TokeniseOptions, can append a "\n"
// to the text Tokenise sees whenever the source does not already end
// with one — Diff, Bash Session, console, shell-session, and udiff all
// set it, and Spans' own tokeniseOptions cannot reach it, because
// EnsureNL is a different field than the EnsureLF that comment already
// covers. chroma's own end-of-text bookkeeping
// (regexp.go's LexerState.newlineAdded) is meant to keep that synthetic
// byte out of the emitted tokens, but does not always succeed —
// reproduced directly: a notes.diff excerpt not ending in "\n" renders a
// <pre> whose text gained a trailing "\n" the reviewer never wrote.
//
// Rather than special-casing the five lexer names above, or reaching
// into chroma's own newlineAdded bookkeeping this package cannot see,
// this checks the one externally observable postcondition that
// actually matters: did tokenisation's own concatenated output grow by
// exactly one byte, and is that byte a trailing "\n" absent from the
// original? Only then is it trimmed back off the last token — which
// restores TestSpansConcatenationReproducesSourceByteForByte's own
// property for every lexer that sets the knob, present or future, not
// only the five named above.
func trimSyntheticTrailingNewline(tokens []chroma.Token, original string) []chroma.Token {
	if len(tokens) == 0 || strings.HasSuffix(original, "\n") {
		return tokens
	}
	total := 0
	for _, t := range tokens {
		total += len(t.Value)
	}
	if total != len(original)+1 {
		return tokens
	}
	last := len(tokens) - 1
	if !strings.HasSuffix(tokens[last].Value, "\n") {
		return tokens
	}
	tokens[last].Value = strings.TrimSuffix(tokens[last].Value, "\n")
	if tokens[last].Value == "" {
		return tokens[:last]
	}
	return tokens
}

// tokeniseOptions is Spans' own fixed TokeniseOptions: root state, not
// nested, and EnsureLF explicitly false — see Spans' own comment for why
// leaving this nil (chroma's default) would silently rewrite a source
// excerpt's line endings before tokenisation ever sees them.
var tokeniseOptions = &chroma.TokeniseOptions{State: "root", EnsureLF: false}

// mergeCodeSpans folds a chroma token slice into htmlCodeSpans, merging
// consecutive tokens whose classFor result is identical (§6.2) — the
// count of class transitions decides the span count, not the count of
// chroma tokens. Concatenating every returned span's Text, in order,
// reproduces the input tokens' Values byte for byte; only span
// boundaries move, no byte is added, dropped, or reordered.
func mergeCodeSpans(tokens []chroma.Token) []htmlCodeSpan {
	spans := make([]htmlCodeSpan, 0, len(tokens))
	for _, t := range tokens {
		class := classFor(t.Type)
		if n := len(spans); n > 0 && spans[n-1].Class == class {
			spans[n-1].Text += t.Value
			continue
		}
		spans = append(spans, htmlCodeSpan{Class: class, Text: t.Value})
	}
	return spans
}

// Label is the code block's own language label (§6.3): the file
// extension read directly off c.Filename — ".sql", never chroma's own
// matched-lexer Config().Name, which reports "MySQL" for that extension
// unconditionally regardless of what kind of .sql file it actually is
// (internal/store/sql/schema.sql, this project's own SQLite schema, is
// the proof). Label reflects Filename alone, independent of whether
// highlightLexer resolved a real lexer or fell back, so an unrecognized
// extension still shows as itself rather than silently losing its label.
// c.Filename == "" — no anchor to read one from — renders "", which
// code.gohtml treats as no label element at all (§12's absence-over-
// invention posture, extended one level in).
func (c htmlCodeView) Label() string {
	if c.Filename == "" {
		return ""
	}
	return path.Ext(c.Filename)
}
