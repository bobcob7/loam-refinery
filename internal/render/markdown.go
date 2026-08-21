package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

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

// commonMarkEscapable is CommonMark's full ASCII-punctuation escapable set
// (§8.3.2), copied verbatim rather than hand-picked: escaping only "#" and
// "`" would leave a narrower forgery still possible through any of the
// other thirty characters.
const commonMarkEscapable = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

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
// backtick longer than the longest run already inside it.
func markdownAnchors(anchors []collect.Anchor) string {
	spans := make([]string, 0, len(anchors))
	for _, a := range anchors {
		text := a.File + ":" + strconv.Itoa(a.Line)
		if a.EndLine != nil {
			text += "-" + strconv.Itoa(*a.EndLine)
		}
		spans = append(spans, markdownCodeSpan(text))
	}
	return strings.Join(spans, ", ")
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

// escapeMarkdown backslash-escapes every character in commonMarkEscapable,
// one at a time (§8.3.2) — the treatment every free-text prose field gets:
// body, summary, suggestion summary, pros, and cons. Structurally-
// constrained fields (id, profile, ordinal, verdict, category, effort,
// scope) never pass through this function; their grammars already exclude
// every character it would touch.
func escapeMarkdown(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if strings.ContainsRune(commonMarkEscapable, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
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
