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
package render

import (
	"path"

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

// highlightLexer resolves §6.3's language inference to one chroma.Lexer.
// An empty filename — no anchor to infer from — and a filename whose
// extension lexers.Match does not recognize (nil) are treated
// identically: both fall back to lexers.Fallback, chroma's plaintext
// lexer, which never returns nil and never errors. Neither case is a
// defect in the review (§12), so neither produces an error or a warning
// line here.
func highlightLexer(filename string) chroma.Lexer {
	if filename == "" {
		return lexers.Fallback
	}
	if lex := lexers.Match(filename); lex != nil {
		return lex
	}
	return lexers.Fallback
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
// failure mode collect-reviews's JSON form doesn't already carry).
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
func (c htmlCodeView) Spans() []htmlCodeSpan {
	lex := highlightLexer(c.Filename)
	it, err := lex.Tokenise(tokeniseOptions, c.Code)
	if err != nil {
		return []htmlCodeSpan{{Text: c.Code}}
	}
	return mergeCodeSpans(it.Tokens())
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
