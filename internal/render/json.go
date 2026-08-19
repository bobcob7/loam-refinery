package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/bobcob7/refinery/internal/entry"
	"github.com/bobcob7/refinery/internal/review"
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

type jsonSkipped struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type jsonDiagnostic struct {
	Severity string `json:"severity"`
	Name     string `json:"name"`
	Comment  string `json:"comment,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// Result writes the whole result object to stdout.
func (j *JSON) Result(stdout, _ io.Writer, result *review.Result) error {
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
		Skipped:     []jsonSkipped{},
		Diagnostics: []jsonDiagnostic{},
		Lenses:      result.Lenses(),
	}
	for _, skipped := range result.Skipped {
		payload.Skipped = append(payload.Skipped, jsonSkipped{Name: skipped.Name, Reason: skipped.Reason})
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
	return write(stdout, payload)
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

// Index writes the lens index, no bodies.
func (j *JSON) Index(w io.Writer, groups []entry.Group) error {
	index := map[string][]string{}
	for _, group := range groups {
		index[string(group.Namespace)] = group.Names
	}
	return write(w, struct {
		Index map[string][]string `json:"index"`
	}{Index: index})
}

func write(w io.Writer, payload any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("encoding json output: %w", err)
	}
	return nil
}
