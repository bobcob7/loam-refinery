// Package render turns a validate result or a set of entries into the text a
// caller reads: terse by default, machine-readable on request.
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/bobcob7/refinery/internal/entry"
	"github.com/bobcob7/refinery/internal/review"
)

const (
	width         = 80
	severityWidth = 10
	nameWidth     = 26
)

// Text renders for a caller reading the output, or a model paying for it.
type Text struct{}

// NewText returns the text renderer.
func NewText() *Text {
	return &Text{}
}

// Result writes the status line to stdout and every diagnostic to stderr, so a
// caller can capture pass/fail cheaply and still surface detail on failure.
func (t *Text) Result(stdout, stderr io.Writer, result *review.Result) error {
	if _, err := fmt.Fprintln(stdout, statusLine(result)); err != nil {
		return fmt.Errorf("writing status line: %w", err)
	}
	if len(result.Diagnostics) == 0 && len(result.Skipped) == 0 {
		return nil
	}
	body := &strings.Builder{}
	body.WriteString("\n")
	for _, diagnostic := range result.Diagnostics {
		body.WriteString(diagnosticLines(diagnostic))
	}
	body.WriteString(skippedLines(result.Skipped))
	if lenses := result.Lenses(); len(lenses) > 0 {
		body.WriteString("\nrefinery describe --lens=" + strings.Join(lenses, ",") + "\n")
	}
	if _, err := io.WriteString(stderr, body.String()); err != nil {
		return fmt.Errorf("writing diagnostics: %w", err)
	}
	return nil
}

func statusLine(result *review.Result) string {
	counts := []string{}
	if result.Valid {
		counts = append(counts, plural(result.Comments, "comment"))
	} else {
		counts = append(counts, plural(result.Errors(), "error"))
	}
	counts = append(counts, advisoryCount(result.Advisories()))
	if len(result.Skipped) > 0 {
		counts = append(counts, fmt.Sprintf("%d skipped", len(result.Skipped)))
	}
	verdict := "INVALID"
	if result.Valid {
		verdict = "VALID"
	}
	return fmt.Sprintf("%s  %s  %s", verdict, strings.Join(counts, ", "), anchorStatus(result.Verification))
}

func anchorStatus(verification review.Verification) string {
	if verification.Source != "repo" {
		reason := verification.Reason
		if reason == "" {
			reason = "no repository"
		}
		return fmt.Sprintf("[anchors unverified: %s]", reason)
	}
	return fmt.Sprintf("[anchors verified: %d of %d]", verification.Verified, verification.Anchors)
}

// diagnosticLines renders one diagnostic: severity, check name, then the
// comment id it concerns — a JSON Pointer for schema failures, and nothing at
// all for a check about the document as a whole.
func diagnosticLines(diagnostic review.Diagnostic) string {
	subject := diagnostic.Comment
	if subject == "" && (diagnostic.Name == "schema" || strings.HasPrefix(diagnostic.Path, "/comments/")) {
		subject = diagnostic.Path
	}
	header := pad(string(diagnostic.Severity), severityWidth) + pad(diagnostic.Name, nameWidth) + subject
	return strings.TrimRight(header, " ") + "\n" + block(strings.Repeat(" ", severityWidth), diagnostic.Message, width)
}

// skippedLines groups skipped checks by reason so a shared cause is stated once.
func skippedLines(skipped []review.Skipped) string {
	reasons := []string{}
	byReason := map[string][]string{}
	for _, skip := range skipped {
		if _, seen := byReason[skip.Reason]; !seen {
			reasons = append(reasons, skip.Reason)
		}
		byReason[skip.Reason] = append(byReason[skip.Reason], skip.Name)
	}
	out := &strings.Builder{}
	for _, reason := range reasons {
		names := fmt.Sprintf("%s (%s)", strings.Join(byReason[reason], ", "), reason)
		out.WriteString(block("skipped  ", names, width))
	}
	return out.String()
}

// Entries writes one entry in full, each self-contained.
func (t *Text) Entries(w io.Writer, entries []entry.Entry) error {
	out := &strings.Builder{}
	for i, e := range entries {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString(e.Qualified() + " — " + e.Title + "\n\n")
		out.WriteString(reflow(e.Body, width))
		if e.Example != "" {
			out.WriteString("\nexample: " + e.Example + "\n")
		}
		if len(e.Related) > 0 {
			out.WriteString("related: " + strings.Join(e.Related, ", ") + "\n")
		}
	}
	if _, err := io.WriteString(w, out.String()); err != nil {
		return fmt.Errorf("writing entries: %w", err)
	}
	return nil
}

// Index writes every entry name grouped by namespace, no bodies.
func (t *Text) Index(w io.Writer, groups []entry.Group) error {
	out := &strings.Builder{}
	for _, group := range groups {
		out.WriteString(block(pad(string(group.Namespace), 7), strings.Join(group.Names, ", "), width))
	}
	if _, err := io.WriteString(w, out.String()); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}
	return nil
}

func pad(value string, size int) string {
	if len(value) >= size {
		return value + " "
	}
	return value + strings.Repeat(" ", size-len(value))
}

// block writes text under a prefix, continuation lines aligned under the first.
func block(prefix, text string, column int) string {
	indent := strings.Repeat(" ", len(prefix))
	out := &strings.Builder{}
	for i, line := range wrapLines(text, column-len(prefix)) {
		if i == 0 {
			out.WriteString(prefix + line + "\n")
			continue
		}
		out.WriteString(indent + line + "\n")
	}
	return out.String()
}

// wrapLines breaks text into lines no wider than the column.
func wrapLines(text string, column int) []string {
	lines := []string{}
	line := ""
	for _, word := range strings.Fields(text) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if len(candidate) > column && line != "" {
			lines = append(lines, line)
			line = word
			continue
		}
		line = candidate
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// reflow rewraps prose paragraphs while leaving indented fragments alone, so a
// code example keeps the shape its author gave it.
func reflow(body string, column int) string {
	out := &strings.Builder{}
	paragraph := []string{}
	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		out.WriteString(block("", strings.Join(paragraph, " "), column))
		paragraph = nil
	}
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			flush()
			out.WriteString("\n")
		case strings.HasPrefix(line, "  "):
			flush()
			out.WriteString(line + "\n")
		default:
			paragraph = append(paragraph, strings.TrimSpace(line))
		}
	}
	flush()
	return out.String()
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func advisoryCount(n int) string {
	if n == 1 {
		return "1 advisory"
	}
	return fmt.Sprintf("%d advisories", n)
}
