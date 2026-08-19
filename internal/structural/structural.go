// Package structural runs the hard checks: the ones that decide whether the
// input can be consumed as a review document at all.
package structural

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/bobcob7/refinery/internal/review"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Checker runs every structural check over one document.
type Checker struct {
	schema schemaValidator
	log    *slog.Logger
}

// New returns a Checker that validates against the supplied schema.
func New(schema schemaValidator, log *slog.Logger) *Checker {
	return &Checker{schema: schema, log: log}
}

// Check runs every structural check. No check gates another: each degrades to
// skipping the item whose inputs it cannot read.
func (c *Checker) Check(doc *review.Document) []review.Diagnostic {
	diagnostics := []review.Diagnostic{}
	diagnostics = append(diagnostics, c.idUnique(doc)...)
	diagnostics = append(diagnostics, c.anchorRangeOrdered(doc)...)
	diagnostics = append(diagnostics, c.anchorPathSafe(doc)...)
	diagnostics = append(diagnostics, c.refFormat(doc)...)
	covered := map[string]bool{}
	for _, d := range diagnostics {
		covered[d.Path] = true
	}
	c.log.Debug("structural checks complete", "diagnostics", len(diagnostics))
	return append(c.schemaCheck(doc, covered), diagnostics...)
}

// schemaCheck reports conformance failures, dropping any whose location a
// dedicated check already reported so one mistake costs one diagnostic.
func (c *Checker) schemaCheck(doc *review.Document, covered map[string]bool) []review.Diagnostic {
	diagnostics := []review.Diagnostic{}
	for _, failure := range c.schema.Validate(doc.Root) {
		if covered[failure.Path] {
			continue
		}
		lens := failure.Field
		if lens == "" {
			lens = "schema"
		}
		diagnostics = append(diagnostics, review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "schema",
			Path:     failure.Path,
			Message:  failure.Message,
			Lens:     lens,
		})
	}
	return diagnostics
}

func (c *Checker) idUnique(doc *review.Document) []review.Diagnostic {
	diagnostics := []review.Diagnostic{}
	first := map[string]int{}
	for _, comment := range doc.Comments {
		if !comment.ID.OK {
			continue
		}
		id := comment.ID.Value
		earlier, seen := first[id]
		if !seen {
			first[id] = comment.Index
			continue
		}
		diagnostics = append(diagnostics, review.Diagnostic{
			Severity: review.SeverityError,
			Name:     "id-unique",
			Comment:  id,
			Path:     comment.Path + "/id",
			Message:  fmt.Sprintf("declared by comments[%d] and comments[%d]", earlier, comment.Index),
		})
	}
	return diagnostics
}

func (c *Checker) anchorRangeOrdered(doc *review.Document) []review.Diagnostic {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		for _, anchor := range comment.Anchors {
			if !anchor.EndLine.OK {
				continue
			}
			switch {
			case !anchor.Line.Present:
				diagnostics = append(diagnostics, review.Diagnostic{
					Severity: review.SeverityError,
					Name:     "anchor-range-ordered",
					Comment:  commentID(comment),
					Path:     anchor.Path + "/end_line",
					Message:  fmt.Sprintf("end_line %d without line", anchor.EndLine.Value),
				})
			case anchor.Line.OK && anchor.EndLine.Value < anchor.Line.Value:
				diagnostics = append(diagnostics, review.Diagnostic{
					Severity: review.SeverityError,
					Name:     "anchor-range-ordered",
					Comment:  commentID(comment),
					Path:     anchor.Path + "/end_line",
					Message:  fmt.Sprintf("end_line %d is before line %d", anchor.EndLine.Value, anchor.Line.Value),
				})
			}
		}
	}
	return diagnostics
}

func (c *Checker) anchorPathSafe(doc *review.Document) []review.Diagnostic {
	diagnostics := []review.Diagnostic{}
	for _, comment := range doc.Comments {
		for _, anchor := range comment.Anchors {
			if !anchor.File.OK {
				continue
			}
			reason, unsafe := pathProblem(anchor.File.Value)
			if !unsafe {
				continue
			}
			diagnostics = append(diagnostics, review.Diagnostic{
				Severity: review.SeverityError,
				Name:     "anchor-path-safe",
				Comment:  commentID(comment),
				Path:     anchor.Path + "/file",
				Message:  fmt.Sprintf("file %q %s", anchor.File.Value, reason),
			})
		}
	}
	return diagnostics
}

func pathProblem(file string) (string, bool) {
	switch {
	case strings.HasPrefix(file, "/"):
		return "is absolute; anchors are repository-relative", true
	case strings.Contains(file, `\`):
		return "contains a backslash; anchors are POSIX paths", true
	}
	for _, segment := range strings.Split(file, "/") {
		if segment == ".." {
			return "escapes the repository", true
		}
	}
	return "", false
}

func (c *Checker) refFormat(doc *review.Document) []review.Diagnostic {
	if !doc.Ref.OK || shaPattern.MatchString(doc.Ref.Value) {
		return nil
	}
	return []review.Diagnostic{{
		Severity: review.SeverityError,
		Name:     "ref-format",
		Path:     "/ref",
		Message:  fmt.Sprintf("ref %q is not a 40-character lowercase commit SHA", doc.Ref.Value),
	}}
}

func commentID(comment review.Comment) string {
	if comment.ID.OK {
		return comment.ID.Value
	}
	return ""
}

// ValidSHA reports whether a ref is a full lowercase commit SHA.
func ValidSHA(ref string) bool {
	return shaPattern.MatchString(ref)
}
