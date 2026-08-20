package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/bobcob7/loam-refinery/internal/entry"
	"github.com/bobcob7/loam-refinery/internal/profile"
	"github.com/bobcob7/loam-refinery/internal/review"
)

// JSON renders for programmatic callers. The whole object goes to stdout;
// nothing is written to stderr except on a usage error.
type JSON struct{}

// NewJSON returns the JSON renderer.
func NewJSON() *JSON {
	return &JSON{}
}

type jsonResult struct {
	Valid        bool             `json:"valid"`
	Strict       bool             `json:"strict"`
	Verification jsonVerification `json:"verification"`
	Counts       jsonCounts       `json:"counts"`
	Skipped      []jsonSkipped    `json:"skipped"`
	Diagnostics  []jsonDiagnostic `json:"diagnostics"`
	Lenses       []string         `json:"lenses,omitempty"`
}

type jsonVerification struct {
	Source   string `json:"source"`
	Reason   string `json:"reason,omitempty"`
	Anchors  int    `json:"anchors"`
	Verified int    `json:"verified"`
}

type jsonCounts struct {
	Comments   int `json:"comments"`
	Errors     int `json:"errors"`
	Advisories int `json:"advisories"`
	Skipped    int `json:"skipped"`
}

// jsonSkipped groups the checks that stopped for the same cause. One cause
// commonly stops every check there is — an unparseable document stops all of
// them — and repeating its sentence once per check made the reason the bulk of
// the payload rather than the finding.
type jsonSkipped struct {
	Reason string   `json:"reason"`
	Checks []string `json:"checks"`
}

// groupSkipped collects checks by reason, keeping first-seen order so the
// output does not move between runs.
func groupSkipped(skipped []review.Skipped) []jsonSkipped {
	order := []string{}
	byReason := map[string][]string{}
	for _, skip := range skipped {
		if _, seen := byReason[skip.Reason]; !seen {
			order = append(order, skip.Reason)
		}
		byReason[skip.Reason] = append(byReason[skip.Reason], skip.Name)
	}
	out := make([]jsonSkipped, 0, len(order))
	for _, reason := range order {
		out = append(out, jsonSkipped{Reason: reason, Checks: byReason[reason]})
	}
	return out
}

type jsonDiagnostic struct {
	Severity string `json:"severity"`
	Name     string `json:"name"`
	Comment  string `json:"comment,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// Result writes the whole result object to stdout.
func (j *JSON) Result(w io.Writer, result *review.Result) error {
	payload := jsonResult{
		Valid:  result.Valid,
		Strict: result.Strict,
		Verification: jsonVerification{
			Source:   result.Verification.Source,
			Reason:   result.Verification.Reason,
			Anchors:  result.Verification.Anchors,
			Verified: result.Verification.Verified,
		},
		Counts: jsonCounts{
			Comments:   result.Comments,
			Errors:     result.Errors(),
			Advisories: result.Advisories(),
			Skipped:    len(result.Skipped),
		},
		Skipped:     groupSkipped(result.Skipped),
		Diagnostics: []jsonDiagnostic{},
		Lenses:      result.Lenses(),
	}
	for _, diagnostic := range result.Diagnostics {
		payload.Diagnostics = append(payload.Diagnostics, jsonDiagnostic{
			Severity: string(diagnostic.Severity),
			Name:     diagnostic.Name,
			Comment:  diagnostic.Comment,
			Path:     diagnostic.Path,
			Message:  diagnostic.Message,
		})
	}
	return write(w, payload)
}

type jsonEntry struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Example   string   `json:"example,omitempty"`
	Related   []string `json:"related,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Provider  string   `json:"provider"`
}

// Entries writes the requested entries as one object.
func (j *JSON) Entries(w io.Writer, entries []entry.Entry) error {
	payload := struct {
		Entries []jsonEntry `json:"entries"`
	}{Entries: []jsonEntry{}}
	for _, e := range entries {
		payload.Entries = append(payload.Entries, jsonEntry{
			Name:      e.Name,
			Namespace: string(e.Namespace),
			Title:     e.Title,
			Body:      e.Body,
			Example:   e.Example,
			Related:   e.Related,
			Aliases:   e.Aliases,
			Provider:  e.Provider,
		})
	}
	return write(w, payload)
}

// jsonGroup is one namespace of the index. The index is a list rather than an
// object because the registry orders its namespaces deliberately — fields
// first, then checks, then topics — and an object would let the encoder sort
// them alphabetically instead.
type jsonGroup struct {
	Namespace string   `json:"namespace"`
	Names     []string `json:"names"`
}

func groupIndex(groups []entry.Group) []jsonGroup {
	out := make([]jsonGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, jsonGroup{Namespace: string(group.Namespace), Names: group.Names})
	}
	return out
}

// Index writes the lens index, no bodies.
func (j *JSON) Index(w io.Writer, groups []entry.Group) error {
	return write(w, struct {
		Index []jsonGroup `json:"index"`
	}{Index: groupIndex(groups)})
}

// Summary writes the document-shape prose and the lens index together, which
// is what describe with no arguments has to say.
func (j *JSON) Summary(w io.Writer, text string, groups []entry.Group) error {
	return write(w, struct {
		Summary string      `json:"summary"`
		Index   []jsonGroup `json:"index"`
	}{Summary: text, Index: groupIndex(groups)})
}

// jsonProfile is one row of prime --list: name and description, no body
// (docs/cli.md §2.1.5 - bodies are never part of the index).
type jsonProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Profiles writes prime --list's profile index. profiles is never nil going
// in (internal/profile.Reader.List guarantees that), and the payload's own
// slice is initialized empty rather than left nil either way, so a missing
// or empty profile directory renders "profiles":[] and never "profiles":null.
func (j *JSON) Profiles(w io.Writer, profiles []profile.Profile) error {
	payload := struct {
		Profiles []jsonProfile `json:"profiles"`
	}{Profiles: []jsonProfile{}}
	for _, p := range profiles {
		payload.Profiles = append(payload.Profiles, jsonProfile{Name: p.Name, Description: p.Description})
	}
	return write(w, payload)
}

func write(w io.Writer, payload any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	// The document shape and every lens body are prose meant to be read. HTML
	// escaping turns the canonical example's "<40 hex>" into "\u003c40 hex\u003e",
	// which is what a caller would then copy.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("encoding json output: %w", err)
	}
	return nil
}
