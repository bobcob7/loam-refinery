package review

import (
	"sort"
	"strconv"
	"strings"
)

// Severity is how much a diagnostic costs the caller.
type Severity string

const (
	// SeverityError is a diagnostic that makes the document unusable.
	SeverityError Severity = "error"
	// SeverityAdvisory is a diagnostic about review quality; non-fatal unless --strict.
	SeverityAdvisory Severity = "advisory"
)

// Diagnostic is one finding about a review document.
type Diagnostic struct {
	Severity Severity
	// Name is the check that fired. Check names are API.
	Name string
	// Comment is the comment id the diagnostic concerns, empty for
	// document-level checks.
	Comment string
	// Path is a JSON Pointer into the input document, empty for
	// document-level checks.
	Path string
	// Message is one sentence of prose, never an explanation: the explanation
	// is one describe --lens away.
	Message string
	// Lens is the entry name that explains this diagnostic. Empty means Name.
	Lens string
}

// LensName is the entry a caller should open to understand this diagnostic.
func (d Diagnostic) LensName() string {
	if d.Lens != "" {
		return d.Lens
	}
	return d.Name
}

// Skipped is a check that could not run, and why. A skipped check is never
// counted as a pass.
type Skipped struct {
	Name   string
	Reason string
}

// Verification records whether anchors were checked, and against what.
type Verification struct {
	// Source is "repo" when a repository answered, "none" when the run was not
	// inside one, and "unavailable" when there was one but it could not be
	// asked. "none" and "unavailable" both skip the anchor checks, but only
	// "none" is ordinary: a caller that treats them alike cannot tell a
	// document nothing checked from one a repository confirmed.
	Source string
	// Reason explains a source other than "repo".
	Reason   string
	Anchors  int
	Verified int
}

// Result is everything one validate run determined.
type Result struct {
	Valid        bool
	Strict       bool
	Comments     int
	Verification Verification
	Diagnostics  []Diagnostic
	Skipped      []Skipped
}

// Errors counts diagnostics that make the document unusable.
func (r *Result) Errors() int {
	n := 0
	for _, d := range r.Diagnostics {
		if d.Severity == SeverityError {
			n++
		}
	}
	return n
}

// Advisories counts soft diagnostics.
func (r *Result) Advisories() int {
	return len(r.Diagnostics) - r.Errors()
}

// Lenses is the deduplicated set of lens names covering the diagnostics, in the
// order the diagnostics appear.
func (r *Result) Lenses() []string {
	seen := map[string]bool{}
	names := []string{}
	for _, d := range r.Diagnostics {
		name := d.LensName()
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// SortDiagnostics orders errors before advisories, then document order.
func SortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i], diagnostics[j]
		if a.Severity != b.Severity {
			return a.Severity == SeverityError
		}
		return less(pathOrder(a.Path), pathOrder(b.Path))
	})
}

func pathOrder(path string) []int {
	order := []int{}
	for _, segment := range strings.Split(path, "/") {
		if n, err := strconv.Atoi(segment); err == nil {
			order = append(order, n)
		}
	}
	return order
}

func less(a, b []int) bool {
	for i := range a {
		if i >= len(b) {
			return false
		}
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
