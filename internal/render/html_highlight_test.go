// internal/render/html_highlight_test.go — bead .4's own tests for
// html_highlight.go: the InSubCategory pin against the InCategory trap
// §6.2 names directly, the plain-text fallback's empty class, span
// merging measured against chroma's raw token count, byte-for-byte
// fidelity of the concatenated Values, and §6.3's three language-
// inference cases including the "MySQL" lexer-name leak this project's
// own schema.sql proves. Bead .7's own suite
// (internal/render/html_test.go) owns parity, fidelity, forgery,
// determinism, and the one golden file at the whole-page altitude; this
// file covers only what html_highlight.go itself is responsible for.
package render

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/bobcob7/loam-refinery/internal/collect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassForDistinguishesNumberFromString is §6.2's named trap,
// pinned directly: chroma.LiteralNumberInteger.InCategory(chroma.LiteralString)
// is true, because LiteralString (3100-3199) and LiteralNumber
// (3200-3299) are both subcategories of the same 3000-3999 Literal
// category and InCategory divides by 1000. A classFor written against
// InCategory would silently paint every numeric literal with the string
// color; this test proves the trap is real, then proves classFor avoids
// it.
func TestClassForDistinguishesNumberFromString(t *testing.T) {
	t.Parallel()
	require.True(t, chroma.LiteralNumberInteger.InCategory(chroma.LiteralString), "the trap itself: InCategory cannot tell LiteralNumber from LiteralString")
	assert.Equal(t, "c-num", classFor(chroma.LiteralNumberInteger), "x := 42's \"42\" must get the number class, not the string class")
	assert.Equal(t, "c-str", classFor(chroma.LiteralString), "a quoted literal must still get the string class")
}

// TestClassForCoversEverySubtype checks every named category's
// subtypes map to the same class as their parent, per §6.2's table.
func TestClassForCoversEverySubtype(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		tokenType chroma.TokenType
		want      string
	}{
		"KeywordType":          {chroma.KeywordType, "c-kw"},
		"KeywordReserved":      {chroma.KeywordReserved, "c-kw"},
		"LiteralStringDouble":  {chroma.LiteralStringDouble, "c-str"},
		"LiteralStringHeredoc": {chroma.LiteralStringHeredoc, "c-str"},
		"CommentMultiline":     {chroma.CommentMultiline, "c-cm"},
		"CommentPreproc":       {chroma.CommentPreproc, "c-cm"},
		"NameFunction":         {chroma.NameFunction, "c-nm"},
		"NameBuiltin":          {chroma.NameBuiltin, "c-nm"},
		"NameVariableInstance": {chroma.NameVariableInstance, "c-nm"},
		"LiteralNumberFloat":   {chroma.LiteralNumberFloat, "c-num"},
		"LiteralNumberHex":     {chroma.LiteralNumberHex, "c-num"},
		"Punctuation":          {chroma.Punctuation, "c-pn"},
		"Operator":             {chroma.Operator, "c-pn"},
		"OperatorWord":         {chroma.OperatorWord, "c-pn"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, classFor(tc.tokenType))
		})
	}
}

// TestClassForReturnsEmptyForPlainTypes is §6.2's own acceptance
// criterion: classFor returns "" for Text, TextWhitespace, Background,
// Error, and Other — no span for any of them.
func TestClassForReturnsEmptyForPlainTypes(t *testing.T) {
	t.Parallel()
	for _, tt := range []chroma.TokenType{chroma.Text, chroma.TextWhitespace, chroma.Background, chroma.Error, chroma.Other} {
		assert.Equal(t, "", classFor(tt))
	}
}

// TestSpansPlainTextExcerptHasZeroClassifiedSpans is §6.2's acceptance
// criterion at the excerpt level: an excerpt of only whitespace and
// plain text produces spans that are all Class == "" — code.gohtml
// wraps none of them in a <span>.
func TestSpansPlainTextExcerptHasZeroClassifiedSpans(t *testing.T) {
	t.Parallel()
	view := htmlCodeView{Code: "just some   plain\ntext, no code at all"}
	for _, s := range view.Spans() {
		assert.Equal(t, "", s.Class)
	}
}

// TestMergeCodeSpansCollapsesAdjacentSameClassTokens is §6.2's merging
// requirement, measured directly: chroma emits one token per punctuation
// mark, so tokenizing "a[0], b[1]" as Go source produces many more raw
// tokens than the number of class transitions in the output. The
// assertion is against the transition count, not the token count.
func TestMergeCodeSpansCollapsesAdjacentSameClassTokens(t *testing.T) {
	t.Parallel()
	source := "arr[0], arr[1], arr[2], arr[3]"
	view := htmlCodeView{Code: source, Filename: "x.go"}
	lex := highlightLexer(view.Filename)
	it, err := lex.Tokenise(nil, source)
	require.NoError(t, err)
	rawTokens := it.Tokens()
	spans := mergeCodeSpans(rawTokens)
	wantTransitions := 1
	prevClass := classFor(rawTokens[0].Type)
	for _, tok := range rawTokens[1:] {
		class := classFor(tok.Type)
		if class != prevClass {
			wantTransitions++
			prevClass = class
		}
	}
	assert.Len(t, spans, wantTransitions, "span count must equal the number of class transitions, not the chroma token count")
	assert.Less(t, len(spans), len(rawTokens), "merging must actually reduce the span count below the raw token count for this fixture")
	t.Logf("spans saved by merging: %d raw tokens -> %d spans (%d fewer)", len(rawTokens), len(spans), len(rawTokens)-len(spans))
}

// TestSpansConcatenationReproducesSourceByteForByte pins
// docs/features/html-report.md §2.2's fidelity test at this file's own
// altitude: concatenating every returned span's Text, spans stripped,
// reproduces the original source exactly, carriage returns included —
// merging only moves span boundaries, it never drops, adds, or reorders
// a byte.
func TestSpansConcatenationReproducesSourceByteForByte(t *testing.T) {
	t.Parallel()
	source := "func main() {\r\n\tx := 42 // a comment\r\n\tprint(\"hi\\r\\n\")\r\n}\r\n"
	view := htmlCodeView{Code: source, Filename: "main.go"}
	var got strings.Builder
	for _, s := range view.Spans() {
		got.WriteString(s.Text)
	}
	assert.Equal(t, source, got.String())
}

// TestHighlightLexerNoAnchorFallsBackWithoutError is §6.3's first case:
// no anchor at all — an empty filename — falls back to lexers.Fallback,
// which never returns nil, and Label renders no label at all.
func TestHighlightLexerNoAnchorFallsBackWithoutError(t *testing.T) {
	t.Parallel()
	view := htmlCodeView{Code: "some architectural finding text.", Filename: ""}
	require.NotNil(t, highlightLexer(view.Filename))
	assert.Equal(t, "", view.Label())
	spans := view.Spans()
	require.NotEmpty(t, spans)
	for _, s := range spans {
		assert.Equal(t, "", s.Class, "the fallback lexer tokenizes everything as one undifferentiated run of Text")
	}
}

// TestHighlightLexerUnknownExtensionFallsBackWithoutError is §6.3's
// third case: lexers.Match returns nil for an extension chroma doesn't
// know, treated identically to no anchor — no panic, no error, and
// (unlike the no-anchor case) the label still shows the extension,
// because Label reads Filename directly rather than anything chroma
// resolved from it.
func TestHighlightLexerUnknownExtensionFallsBackWithoutError(t *testing.T) {
	t.Parallel()
	view := htmlCodeView{Code: "whatever this format is", Filename: "notes.zzqx"}
	require.NotNil(t, highlightLexer(view.Filename))
	assert.Equal(t, ".zzqx", view.Label())
	assert.NotPanics(t, func() { view.Spans() })
}

// TestHighlightLexerSQLSchemaNeverLeaksMySQL is §6.3's decision, pinned
// against this project's own fixture: chroma's matched lexer for
// internal/store/sql/schema.sql reports its own Config().Name as
// "MySQL" unconditionally (verified directly against the matched
// lexer), which is exactly why this renderer must never display it.
// Label renders the file's own extension instead.
func TestHighlightLexerSQLSchemaNeverLeaksMySQL(t *testing.T) {
	t.Parallel()
	filename := "internal/store/sql/schema.sql"
	lex := highlightLexer(filename)
	require.NotNil(t, lex)
	require.Equal(t, "MySQL", lex.Config().Name, "this pin only means something if chroma's own matched lexer actually does report MySQL for a .sql file")
	view := htmlCodeView{Code: "CREATE TABLE runs (id INTEGER PRIMARY KEY) STRICT;", Filename: filename}
	assert.Equal(t, ".sql", view.Label())
	assert.NotContains(t, view.Label(), "MySQL")
	assert.NotContains(t, view.Label(), "MYSQL")
	for _, s := range view.Spans() {
		assert.NotContains(t, s.Text, "MySQL")
		assert.NotContains(t, s.Text, "MYSQL")
	}
}

// TestSpansTokeniseErrorDegradesToOneUnstyledSpan documents Spans'
// defensive fallback (§12: never invent a failure mode) — this asserts
// the shape of the degradation, not that any real lexer currently
// errors, since none in ordinary operation does.
func TestSpansTokeniseErrorDegradesToOneUnstyledSpan(t *testing.T) {
	t.Parallel()
	view := htmlCodeView{Code: "plain text that any lexer tokenizes without error"}
	spans := view.Spans()
	require.NotEmpty(t, spans)
}

// jungleHangingExcerpt is bead .11's own reproduction, at the same
// shape and length the bead reports: a 47-byte excerpt whose bare "!"
// (none of Jungle's "var" state rules match it, and its own fallback
// rule pops with zero width) sends the state machine into a push/pop
// cycle that never advances position and never reaches EOF. Verified
// directly against chroma v2.27.0, outside this package: lex.Tokenise
// followed by Tokens() on this string does not return.
const jungleHangingExcerpt = "x = !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"

// TestHighlightLexerDeniesJungleAndJSONata pins deniedLexerNames at the
// highlightLexer altitude: lexers.Match itself still resolves ".jungle"
// and ".jsonata" to the real Jungle and JSONata lexers (the first two
// assertions prove the test fixture is real, not a typo'd extension
// that would have fallen back regardless) but highlightLexer refuses
// both and returns lexers.Fallback instead.
func TestHighlightLexerDeniesJungleAndJSONata(t *testing.T) {
	t.Parallel()
	require.Equal(t, "Jungle", lexers.Match("x.jungle").Config().Name, "fixture check: .jungle must still resolve to the real pathological lexer")
	require.Equal(t, "JSONata", lexers.Match("x.jsonata").Config().Name, "fixture check: .jsonata must still resolve to the real pathological lexer")
	assert.Equal(t, lexers.Fallback, highlightLexer("x.jungle"))
	assert.Equal(t, lexers.Fallback, highlightLexer("x.jsonata"))
}

// TestTokeniseWithinDeadlineAbandonsAPathologicalLexer is bead .11's
// deadline pinned directly against the real Jungle lexer — bypassing
// highlightLexer's own denylist via lexers.Get, so this test still
// means something if that denylist is ever loosened or a lexer chroma
// adds later has the same defect and nobody has denied it yet. A single
// "!" reproduces the hang in one byte (jungleHangingExcerpt's own
// comment explains why); tokeniseWithinDeadline must return ok == false
// well inside highlightTimeout's own budget, not by hanging until the
// test's own timeout kills the whole test binary.
func TestTokeniseWithinDeadlineAbandonsAPathologicalLexer(t *testing.T) {
	t.Parallel()
	lex := lexers.Get("Jungle")
	require.NotNil(t, lex, "fixture check: chroma must still ship a lexer literally named Jungle")
	start := time.Now()
	tokens, ok := tokeniseWithinDeadline(lex, "!")
	elapsed := time.Since(start)
	assert.False(t, ok, "a lexer whose token stream never reaches EOF must not be reported as having tokenised successfully")
	assert.Nil(t, tokens)
	assert.Less(t, elapsed, highlightTimeout+time.Second, "tokeniseWithinDeadline must return once highlightTimeout elapses, not hang past it")
}

// TestSpansJungleAnchorDegradesInsteadOfHanging is bead .11's own
// acceptance criterion end to end, through the real Spans method a
// finding's code partial actually calls: a review with anchor file
// x.jungle and jungleHangingExcerpt's own 47 bytes of code renders in
// bounded time, as a single unstyled span carrying the excerpt
// verbatim, rather than never returning. The bound is generous relative
// to highlightTimeout only to keep this test itself from being flaky
// under load; the fix this pins is that Spans returns at all.
func TestSpansJungleAnchorDegradesInsteadOfHanging(t *testing.T) {
	t.Parallel()
	view := htmlCodeView{Code: jungleHangingExcerpt, Filename: "x.jungle"}
	doneCh := make(chan []htmlCodeSpan, 1)
	start := time.Now()
	go func() { doneCh <- view.Spans() }()
	select {
	case spans := <-doneCh:
		elapsed := time.Since(start)
		t.Logf("x.jungle excerpt rendered in %v", elapsed)
		assert.Less(t, elapsed, 5*time.Second, "a hostile .jungle excerpt must degrade in bounded time, not hang the report")
		require.Len(t, spans, 1)
		assert.Equal(t, "", spans[0].Class, "a degraded excerpt is unstyled — coloring is what was given up, not the text")
		assert.Equal(t, jungleHangingExcerpt, spans[0].Text, "the excerpt itself must stay fully readable even when highlighting is abandoned")
	case <-time.After(10 * time.Second):
		t.Fatal("Spans on a .jungle anchor did not return within 10s — the hang bead .11 exists to fix is still present")
	}
}

// TestSpansOversizedExcerptDegradesWithoutTokenising is bead .11's size
// cap: an excerpt over maxHighlightInputBytes never reaches Tokenise at
// all, on a plain .go anchor with nothing pathological about the lexer
// — the amplification bead .11 measured (13x, one span per token plus
// per-span markup) happens on ordinary source, not only on an
// adversarial lexer, so the cap must trigger on size alone.
func TestSpansOversizedExcerptDegradesWithoutTokenising(t *testing.T) {
	t.Parallel()
	oversized := strings.Repeat("x", maxHighlightInputBytes+1)
	view := htmlCodeView{Code: oversized, Filename: "main.go"}
	spans := view.Spans()
	require.Len(t, spans, 1)
	assert.Equal(t, "", spans[0].Class)
	assert.Equal(t, oversized, spans[0].Text)
}

// TestSpansExcessiveSpanCountDegradesToUnstyledFallback is bead .11's
// third, independent cap: an excerpt well under maxHighlightInputBytes
// that still tokenises into more than maxHighlightSpans class
// transitions degrades the same way. Alternating a one-rune identifier
// with a punctuation mark gives Go's own lexer no two adjacent tokens
// to merge, so the span count tracks the token count almost exactly —
// enough repetitions crosses maxHighlightSpans while the excerpt itself
// stays a few kilobytes, nowhere near maxHighlightInputBytes.
func TestSpansExcessiveSpanCountDegradesToUnstyledFallback(t *testing.T) {
	t.Parallel()
	source := strings.Repeat("a;", maxHighlightSpans)
	require.Less(t, len(source), maxHighlightInputBytes, "fixture check: this must exercise the span cap, not the size cap")
	view := htmlCodeView{Code: source, Filename: "main.go"}
	spans := view.Spans()
	require.Len(t, spans, 1)
	assert.Equal(t, "", spans[0].Class)
	assert.Equal(t, source, spans[0].Text)
}

// TestTrimSyntheticTrailingNewlineRestoresByteFidelityForDiffAnchor is
// bead .12's own reproduction and fix: chroma's Diff lexer sets
// Config().EnsureNL, a setting Spans' own tokeniseOptions (EnsureLF)
// cannot reach, so without trimSyntheticTrailingNewline this renders a
// trailing "\n" the reviewer never wrote — verified directly against
// chroma v2.27.0 outside this package. Unlike
// TestSpansConcatenationReproducesSourceByteForByte, whose main.go
// fixture matches a lexer with the knob off, this uses notes.diff,
// where lexers.Match("notes.diff").Config().EnsureNL is true, so this
// test is false before the fix and true after it — the shape the bead
// itself asks for.
func TestTrimSyntheticTrailingNewlineRestoresByteFidelityForDiffAnchor(t *testing.T) {
	t.Parallel()
	require.True(t, lexers.Match("notes.diff").Config().EnsureNL, "fixture check: the Diff lexer must still set the knob this test exists to route around")
	source := "--- a\n+++ b\n-old\n+new"
	view := htmlCodeView{Code: source, Filename: "notes.diff"}
	var got strings.Builder
	for _, s := range view.Spans() {
		got.WriteString(s.Text)
	}
	assert.Equal(t, source, got.String(), "the rendered excerpt must not gain a trailing newline the reviewer never wrote")
}

// numberLiteralEnvelope is a one-comment fixture whose code excerpt
// contains a numeric literal, the exact shape §6.2's acceptance
// criterion names: "given the source x := 42 in a .go excerpt, the
// token carrying \"42\" is emitted with class c-num, not c-str."
func numberLiteralEnvelope() CollectReviewsEnvelope {
	return CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo", IsHead: ptrTo(true), Diverged: []CollectReviewsDiverged{}},
		Result: &collect.Result{
			Comments: []collect.Comment{
				{
					ID: "backend:magic-number-1", Profile: "backend", Priority: 5, Category: "maintainability",
					Body:    "42 is a magic number, not a named constant.",
					Anchors: []collect.Anchor{{File: "internal/fetch/client.go", Line: 12}},
					Code:    "x := 42",
				},
			},
		},
	}
}

// TestRenderedPageGivesNumericLiteralTheNumberClassNotTheStringClass
// pins §6.2's acceptance criterion at the whole-page altitude, through
// the real HTML renderer rather than by calling classFor directly: a
// <span class="c-num"> containing "42" exists, and no <span class="c-str">
// contains it.
func TestRenderedPageGivesNumericLiteralTheNumberClassNotTheStringClass(t *testing.T) {
	t.Parallel()
	_, doc := renderHTML(t, numberLiteralEnvelope())
	numberSpans := htmlNodesWithClass(doc, "c-num")
	require.NotEmpty(t, numberSpans, "expected at least one c-num span in the rendered page")
	found := false
	for _, n := range numberSpans {
		if htmlNodeText(n) == "42" {
			found = true
		}
	}
	assert.True(t, found, "the \"42\" token must carry class c-num")
	for _, n := range htmlNodesWithClass(doc, "c-str") {
		assert.NotEqual(t, "42", htmlNodeText(n), "\"42\" must never carry class c-str")
	}
}

// TestRenderedPageMergesAdjacentTokensAndEmitsNoStyleAttribute renders
// the real report over findingEnvelope's Go excerpt
// ("c.do(context.Background(), req)") and checks three whole-page
// properties at once: at least one classified span exists (highlighting
// actually ran), no span wraps only whitespace text, and no style=
// attribute appears anywhere in the output — chroma's own inline colors
// (§6.2: styles.Get("github") resolves TextWhitespace to #ffffff) are
// never emitted, only the six CSS classes are.
func TestRenderedPageMergesAdjacentTokensAndEmitsNoStyleAttribute(t *testing.T) {
	t.Parallel()
	out, doc := renderHTML(t, findingEnvelope())
	assert.NotContains(t, out, ` style="`, "chroma's own per-token colors must never reach a style attribute")
	classified := 0
	for _, class := range []string{"c-kw", "c-str", "c-cm", "c-nm", "c-num", "c-pn"} {
		classified += len(htmlNodesWithClass(doc, class))
	}
	assert.Positive(t, classified, "the code excerpt in findingEnvelope must produce at least one classified span")
	for _, class := range []string{"c-kw", "c-str", "c-cm", "c-nm", "c-num", "c-pn"} {
		for _, n := range htmlNodesWithClass(doc, class) {
			assert.NotEqual(t, "", strings.TrimSpace(htmlNodeText(n)), "no classified span should wrap only whitespace")
		}
	}
}

// TestRenderedPageNeverMentionsChromasSQLLexerName is §6.3's own
// acceptance criterion: a comment anchored at
// internal/store/sql/schema.sql renders the label ".sql", and the
// strings "MySQL"/"MYSQL" appear nowhere in the output — the real repo
// schema file's own content included, byte for byte, so this is not
// just a check on a hand-written fixture string.
func TestRenderedPageNeverMentionsChromasSQLLexerName(t *testing.T) {
	t.Parallel()
	schema := readSchemaFileForHighlightTest(t)
	envelope := CollectReviewsEnvelope{
		Ref: "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d", RepoName: "github.com/bobcob7/loam-refinery",
		RepoKnown: true, StoreEnabled: true,
		HeadCheck: CollectReviewsHeadCheck{Source: "repo", IsHead: ptrTo(true), Diverged: []CollectReviewsDiverged{}},
		Result: &collect.Result{
			Comments: []collect.Comment{
				{
					ID: "backend:schema-1", Profile: "backend", Priority: 4, Category: "maintainability",
					Body:    "The schema's own column order drifted from the migration.",
					Anchors: []collect.Anchor{{File: "internal/store/sql/schema.sql", Line: 1}},
					Code:    schema,
				},
			},
		},
	}
	out, doc := renderHTML(t, envelope)
	assert.NotContains(t, out, "MySQL")
	assert.NotContains(t, out, "MYSQL")
	labels := htmlNodesWithClass(doc, "code-lang")
	require.NotEmpty(t, labels)
	assert.Equal(t, ".sql", htmlNodeText(labels[0]))
}

func readSchemaFileForHighlightTest(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../store/sql/schema.sql")
	require.NoError(t, err)
	return string(data)
}
