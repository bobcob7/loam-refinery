// Package advisory holds the soft checks: quality signals about a review that
// are reported, named, and never fatal unless the caller asks with --strict.
package advisory

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/bobcob7/loam-refinery/internal/review"
)

// Advisory is one named soft check.
type Advisory struct {
	Meta review.Check
	Run  checkFunc
}

// Registry is the set of advisories one run evaluates. It is a value built at
// construction, never package state, so a test can hold exactly one advisory.
type Registry struct {
	advisories []Advisory
	log        *slog.Logger
}

// New returns a registry over the supplied advisories.
func New(log *slog.Logger, advisories []Advisory) *Registry {
	return &Registry{advisories: advisories, log: log}
}

// All returns every advisory, in reporting order.
func All() []Advisory {
	advisories := []Advisory{}
	advisories = append(advisories, consistency()...)
	advisories = append(advisories, substance()...)
	advisories = append(advisories, calibration()...)
	return advisories
}

// Checks returns the advisory registry as check metadata.
func Checks() []review.Check {
	checks := make([]review.Check, 0, len(All()))
	for _, advisory := range All() {
		checks = append(checks, advisory.Meta)
	}
	return checks
}

// Run evaluates every registered advisory. There is no way to disable one:
// one advisory failing to run never stops another.
func (r *Registry) Run(doc *review.Document) ([]review.Diagnostic, []review.Skipped) {
	diagnostics := []review.Diagnostic{}
	skipped := []review.Skipped{}
	for _, advisory := range r.advisories {
		raised, unrun := advisory.Run(doc)
		diagnostics = append(diagnostics, raised...)
		skipped = append(skipped, unrun...)
	}
	r.log.Debug("advisories complete", "diagnostics", len(diagnostics), "skipped", len(skipped))
	return diagnostics, skipped
}

func diagnostic(name string, comment review.Comment, path, message string) review.Diagnostic {
	return review.Diagnostic{
		Severity: review.SeverityAdvisory,
		Name:     name,
		Comment:  commentID(comment),
		Path:     path,
		Message:  message,
	}
}

func documentDiagnostic(name, path, message string) review.Diagnostic {
	return review.Diagnostic{Severity: review.SeverityAdvisory, Name: name, Path: path, Message: message}
}

func commentID(comment review.Comment) string {
	if comment.ID.OK {
		return comment.ID.Value
	}
	return ""
}

// normalize collapses whitespace so a check measures prose, not formatting.
func normalize(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// fold normalizes for comparison: lowercase, no surrounding punctuation.
func fold(text string) string {
	folded := strings.ToLower(normalize(text))
	return strings.TrimFunc(folded, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
}

// clip shortens a quoted fragment so one diagnostic cannot cost a paragraph.
func clip(text string, limit int) string {
	text = normalize(text)
	if len([]rune(text)) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}

// verb pairs a count with the right verb, so a grouped skip line reads as a
// sentence: "1 comment has unusable priority", "2 comments have".
func verb(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s has", noun)
	}
	return fmt.Sprintf("%d %ss have", n, noun)
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// prioritiesUsable reports whether the whole comment population is well-typed
// enough for an aggregate check to reason over it, and why not when it is not.
func prioritiesUsable(doc *review.Document) (bool, string) {
	if !doc.CommentsArray {
		return false, "comments is not an array"
	}
	unusable := 0
	for _, comment := range doc.Comments {
		if !comment.Priority.OK {
			unusable++
		}
	}
	if unusable > 0 {
		return false, fmt.Sprintf("%s unusable priority", verb(unusable, "comment"))
	}
	return true, ""
}

func skip(reason string, names ...string) []review.Skipped {
	skipped := make([]review.Skipped, 0, len(names))
	for _, name := range names {
		skipped = append(skipped, review.Skipped{Name: name, Reason: reason})
	}
	return skipped
}
