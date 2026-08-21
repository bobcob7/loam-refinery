package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/bobcob7/loam-refinery/internal/collect"
)

// Markdown renders collect-reviews's envelope for a human reader —
// docs/cli.md §5.1's one narrow exception (docs/features/combined-reviews.md
// §8.3). It is a pure projection of the identical CollectReviewsEnvelope
// value JSON.CollectReviews already serializes (§8.3.1): both renderers are
// downstream of internal/collect.Assemble's return value, never of the
// store directly, and this type computes nothing the JSON path does not
// already have — no re-sorting, no re-filtering, no re-deriving a count.
//
// Markdown is for a human, or for pass-through embedding somewhere a human
// reads it — a PR comment body, a chat message. It is never a second
// machine-interchange format (§8.3.3): nothing here parses Markdown back
// into structured data, and nothing downstream should either. A caller
// that wants collect-reviews's findings programmatically uses
// --format json, unconditionally.
type Markdown struct{}

// NewMarkdown returns the Markdown renderer.
func NewMarkdown() *Markdown {
	return &Markdown{}
}

// commonMarkEscapableAnywhere is the set of characters that can change
// CommonMark's meaning no matter where they sit inside a line of free text
// (§8.3.2): inline constructs whose effect does not depend on position.
const commonMarkEscapableAnywhere = "\\`*_[]<&"

// commonMarkLineStartMarkers is the set of single characters that open a
// block construct only when they are the first non-whitespace character on
// a line (§8.3.2): ATX heading, bullet or thematic break, bullet,
// blockquote, setext underline, fence, table row. A run of digits followed
// by "." or ")" — the ordered-list marker — is a shape rather than a
// single character and is handled by orderedListMarker instead.
const commonMarkLineStartMarkers = "#-+>=~|"

// CollectReviews writes collect-reviews's envelope as Markdown to w.
func (md *Markdown) CollectReviews(w io.Writer, envelope CollectReviewsEnvelope) error {
	var b strings.Builder
	writeMarkdownEnvelope(&b, envelope)
	writeMarkdownSubmissions(&b, envelope.Result.Submissions)
	for _, c := range envelope.Result.Comments {
		writeMarkdownComment(&b, c)
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing markdown output: %w", err)
	}
	return nil
}

// writeMarkdownEnvelope writes the document title and the envelope-level
// facts §8.1 defines — ref, repo, store.enabled, head_check, total, and
// unreadable. None of these originate from caller-authored review content
// (they come from the repository and the store's own bookkeeping), so
// none of them fall under §8.3.2's three categories and none are escaped.
func writeMarkdownEnvelope(b *strings.Builder, envelope CollectReviewsEnvelope) {
	fmt.Fprintf(b, "# collect-reviews: %s\n\n", envelope.Ref)
	fmt.Fprintf(b, "**repo** %s (known: %t) · **store.enabled** %t\n", envelope.RepoName, envelope.RepoKnown, envelope.StoreEnabled)
	b.WriteString("**head_check** source=" + envelope.HeadCheck.Source)
	if envelope.HeadCheck.IsHead != nil {
		fmt.Fprintf(b, " · is_head=%t", *envelope.HeadCheck.IsHead)
	}
	if envelope.HeadCheck.Diverged != nil {
		fmt.Fprintf(b, " · diverged=%d", len(envelope.HeadCheck.Diverged))
	}
	b.WriteString("\n")
	total := len(envelope.Result.Submissions) + envelope.Result.Unreadable
	fmt.Fprintf(b, "**total** %d · **unreadable** %d\n\n", total, envelope.Result.Unreadable)
}

// writeMarkdownSubmissions writes the Submissions list as a bold label
// (never an ATX heading — every "## " heading this renderer writes is a
// qualified comment id, and nothing else, so Parity's own heading parse
// (§8.3.3) never has to filter out a non-id heading). ordinal, profile,
// verdict, and superseded_by are structurally-constrained (§8.3.2) and are
// written as-is; summary is free-text prose and is escaped.
func writeMarkdownSubmissions(b *strings.Builder, submissions []collect.Submission) {
	b.WriteString("**Submissions**\n\n")
	for _, s := range submissions {
		profile := s.Profile
		if profile == "" {
			profile = "(none)"
		}
		fmt.Fprintf(b, "- **#%d** %s · %s", s.Ordinal, profile, s.Verdict)
		if s.SupersededBy != nil {
			fmt.Fprintf(b, " · superseded_by=#%d", *s.SupersededBy)
		}
		b.WriteString("\n\n  " + escapeMarkdown(s.Summary) + "\n\n")
	}
}

// writeMarkdownComment writes one comment's section: an "## <id>" heading,
// a metadata line (priority, category, and every anchor as an inline code
// span), the escaped body, and — when the comment carries one — a fenced
// code excerpt, then any suggestions. This is the exact shape
// docs/features/combined-reviews.md §12.3 pins for a comment with no
// suggestions; §12.3's own fixture is this bead's regression test.
func writeMarkdownComment(b *strings.Builder, c collect.Comment) {
	fmt.Fprintf(b, "## %s\n\n", c.ID)
	fmt.Fprintf(b, "**priority** %d · **category** %s · **anchors** %s\n\n", c.Priority, c.Category, markdownAnchors(c.Anchors))
	b.WriteString(escapeMarkdown(c.Body) + "\n\n")
	if c.Code != "" {
		b.WriteString(markdownFencedBlock(c.Code) + "\n\n")
	}
	if len(c.Suggestions) > 0 {
		writeMarkdownSuggestions(b, c.Suggestions)
	}
}

// writeMarkdownSuggestions writes a comment's suggestions. summary, pros,
// and cons are free-text and escaped; effort and scope are
// structurally-constrained enums (§8.3.2) and are not; code, when present,
// is a verbatim field fenced the same way comment.code is.
func writeMarkdownSuggestions(b *strings.Builder, suggestions []collect.Suggestion) {
	b.WriteString("**Suggestions**\n\n")
	for _, s := range suggestions {
		fmt.Fprintf(b, "- %s — effort: %s · scope: %s\n", escapeMarkdown(s.Summary), s.Effort, s.Scope)
		fmt.Fprintf(b, "  - pros: %s\n", escapeMarkdownList(s.Pros))
		fmt.Fprintf(b, "  - cons: %s\n", escapeMarkdownList(s.Cons))
		if s.Code != "" {
			b.WriteString("\n" + markdownFencedBlock(s.Code) + "\n")
		}
		b.WriteString("\n")
	}
}

// markdownAnchors renders every anchor as one inline code span,
// "file:line" or "file:line-end_line", joined by ", ". anchor.file is a
// verbatim field (§8.3.2): the span is never escaped, only fenced, one
// backtick longer than the longest run already inside it — and, first,
// run through sanitizeVerbatimSpan, since no fence length can protect an
// inline span from a line break inside it.
func markdownAnchors(anchors []collect.Anchor) string {
	spans := make([]string, 0, len(anchors))
	for _, a := range anchors {
		text := sanitizeVerbatimSpan(a.File) + ":" + strconv.Itoa(a.Line)
		if a.EndLine != nil {
			text += "-" + strconv.Itoa(*a.EndLine)
		}
		spans = append(spans, markdownCodeSpan(text))
	}
	return strings.Join(spans, ", ")
}

// sanitizeVerbatimSpan replaces every control character in s with a visible
// escape sequence (\n, \r, \t, or \xNN) before the content is wrapped in an
// inline code span. anchor.file is documented as verbatim — displaying
// exactly as written — but that guarantee has one exception: an inline
// code span cannot survive an embedded line break, no matter how long the
// backtick fence is (this bead's own P0), so a control character is the one
// thing this field cannot carry through unaltered. internal/schema's
// pattern and internal/structural's pathProblem both already reject a
// control character in anchor.file before it ever reaches the store — this
// is the renderer's own defence in depth, for the case where it did not.
func sanitizeVerbatimSpan(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsControl(r):
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// markdownCodeSpan wraps content in an inline code span, sized one
// backtick longer than the longest backtick run already inside it — the
// general "fence longer than any run inside" rule §8.3.2 names, applied at
// span scope rather than block scope. Content that starts or ends with a
// backtick is padded with a single space on each side, the standard
// technique that keeps the delimiter from visually fusing with content it
// wraps.
func markdownCodeSpan(content string) string {
	fence := strings.Repeat("`", longestBacktickRun(content)+1)
	if strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`") {
		content = " " + content + " "
	}
	return fence + content + fence
}

// markdownFencedBlock wraps a multi-line code excerpt (comment.code or
// suggestion.code) in a fenced code block, fence length one backtick
// longer than the longest run already inside the content, minimum three
// (§8.3.2) — long enough that the content can never close the fence
// early. Never escaped: escaping a verbatim field would corrupt content
// meant to display byte for byte.
func markdownFencedBlock(content string) string {
	length := longestBacktickRun(content) + 1
	if length < 3 {
		length = 3
	}
	fence := strings.Repeat("`", length)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return fence + "\n" + content + fence
}

// longestBacktickRun returns the length of the longest consecutive run of
// backtick characters in s, 0 when s has none.
func longestBacktickRun(s string) int {
	longest, current := 0, 0
	for _, r := range s {
		if r != '`' {
			current = 0
			continue
		}
		current++
		if current > longest {
			longest = current
		}
	}
	return longest
}

// escapeMarkdown backslash-escapes free-text prose per §8.3.2's
// position-based rule: derived from what CommonMark says each character
// can do, and where, rather than a hand-picked subset of the full
// escapable set. Characters that can change meaning anywhere in a line
// (commonMarkEscapableAnywhere) are always escaped; characters that only
// open a block when they are the first non-whitespace character on a line
// (commonMarkLineStartMarkers, plus the digit-run-then-"."-or-")"
// ordered-list marker) are escaped only there. This is the treatment every
// free-text prose field gets: body, summary, suggestion summary, pros, and
// cons. Structurally-constrained fields (id, profile, ordinal, verdict,
// category, effort, scope) never pass through this function; their
// grammars already exclude every character it would touch.
func escapeMarkdown(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = escapeMarkdownLine(line)
	}
	return strings.Join(lines, "\n")
}

// escapeMarkdownLine escapes one line: the anywhere set is escaped at every
// position, and whatever block-opening construct — if any — begins at the
// first non-whitespace character is escaped there too.
func escapeMarkdownLine(line string) string {
	runes := []rune(line)
	lead := 0
	for lead < len(runes) && (runes[lead] == ' ' || runes[lead] == '\t') {
		lead++
	}
	var b strings.Builder
	b.Grow(len(line) + 2)
	b.WriteString(string(runes[:lead]))
	i := lead
	end, delimiter, isOrderedList := orderedListMarker(runes, lead)
	switch {
	case isOrderedList:
		b.WriteString(string(runes[lead:end]))
		b.WriteByte('\\')
		b.WriteRune(delimiter)
		i = end + 1
	case lead < len(runes) && strings.ContainsRune(commonMarkLineStartMarkers, runes[lead]):
		b.WriteByte('\\')
		b.WriteRune(runes[lead])
		i = lead + 1
	}
	for ; i < len(runes); i++ {
		if strings.ContainsRune(commonMarkEscapableAnywhere, runes[i]) {
			b.WriteByte('\\')
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

// orderedListMarker reports whether runes[i:] opens with a run of ASCII
// digits immediately followed by "." or ")" — the ordered-list marker
// CommonMark recognises at the start of a line (§8.3.2). end is the index
// of the delimiter itself, so the caller can write the digits unescaped and
// the delimiter escaped.
func orderedListMarker(runes []rune, i int) (end int, delimiter rune, ok bool) {
	j := i
	for j < len(runes) && runes[j] >= '0' && runes[j] <= '9' {
		j++
	}
	if j == i || j >= len(runes) || (runes[j] != '.' && runes[j] != ')') {
		return 0, 0, false
	}
	return j, runes[j], true
}

// escapeMarkdownList escapes and joins a free-text list (pros or cons),
// "(none)" when empty rather than an empty line that would read as a
// missing field.
func escapeMarkdownList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	escaped := make([]string, 0, len(items))
	for _, item := range items {
		escaped = append(escaped, escapeMarkdown(item))
	}
	return strings.Join(escaped, "; ")
}
