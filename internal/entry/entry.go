// Package entry holds the registry describe renders from: named entries in
// namespaces, contributed by providers, resolved by bare name or qualified.
package entry

import (
	"fmt"
	"sort"
	"strings"
)

// Namespace groups entries and gives a qualified name its prefix.
type Namespace string

const (
	// NamespaceField holds one document field each.
	NamespaceField Namespace = "field"
	// NamespaceCheck holds one check each: structural, verification or advisory.
	NamespaceCheck Namespace = "check"
	// NamespaceTopic holds cross-cutting concepts.
	NamespaceTopic Namespace = "topic"
)

// order is the order namespaces are listed in, for the index and for candidate
// lists. It is presentation only: resolution never prefers one over another.
var order = []Namespace{NamespaceField, NamespaceCheck, NamespaceTopic}

// Entry is one thing the binary can explain.
type Entry struct {
	Name      string
	Namespace Namespace
	Aliases   []string
	Title     string
	Body      string
	Example   string
	Related   []string
	Provider  string
}

// Qualified is the entry's namespaced name.
func (e Entry) Qualified() string {
	return string(e.Namespace) + ":" + e.Name
}

// Registry resolves lens names to entries. It is assembled at construction, so
// a test can build a registry holding one entry.
type Registry struct {
	entries  []Entry
	byKey    map[string]int
	bySuffix map[string][]int
}

// UnknownLensError means no entry carries that name or alias.
type UnknownLensError struct {
	Name string
}

func (e *UnknownLensError) Error() string {
	return fmt.Sprintf("unknown lens %q", e.Name)
}

// AmbiguousLensError means a name could mean more than one entry — typically a
// bare final segment two field paths share. Never guessed at: the caller re-runs
// with one of the candidates.
type AmbiguousLensError struct {
	Name       string
	Candidates []string
}

func (e *AmbiguousLensError) Error() string {
	return fmt.Sprintf("ambiguous lens %q: %s", e.Name, strings.Join(e.Candidates, ", "))
}

// NewRegistry assembles the entries every provider contributes.
func NewRegistry(providers ...provider) (*Registry, error) {
	registry := &Registry{byKey: map[string]int{}, bySuffix: map[string][]int{}}
	for _, p := range providers {
		entries, err := p.Entries()
		if err != nil {
			return nil, fmt.Errorf("collecting %s entries: %w", p.Name(), err)
		}
		for _, e := range entries {
			e.Provider = p.Name()
			if err := registry.add(e); err != nil {
				return nil, err
			}
		}
	}
	return registry, nil
}

func (r *Registry) add(e Entry) error {
	index := len(r.entries)
	r.entries = append(r.entries, e)
	for _, name := range append([]string{e.Name}, e.Aliases...) {
		key := string(e.Namespace) + ":" + name
		if _, taken := r.byKey[key]; taken {
			return fmt.Errorf("duplicate entry name %q", key)
		}
		r.byKey[key] = index
		r.index(name, index)
	}
	return nil
}

// index files a name under its own final segment, so comments.suggestions.effort
// is reachable as effort whenever nothing else ends in effort.
func (r *Registry) index(name string, entry int) {
	segment := name
	if _, last, dotted := cutLast(name, "."); dotted {
		segment = last
	}
	for _, existing := range r.bySuffix[segment] {
		if existing == entry {
			return
		}
	}
	r.bySuffix[segment] = append(r.bySuffix[segment], entry)
}

func cutLast(value, separator string) (string, string, bool) {
	i := strings.LastIndex(value, separator)
	if i < 0 {
		return value, "", false
	}
	return value[:i], value[i+len(separator):], true
}

// Resolve turns a lens name into an entry. A qualified name resolves directly.
// A bare name resolves when exactly one entry answers to it — by its own name,
// by an alias, or as the final segment of a field path — and anything that could
// mean more than one entry is an error listing the candidates rather than a
// guess.
func (r *Registry) Resolve(name string) (Entry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Entry{}, &UnknownLensError{Name: name}
	}
	if namespace, bare, qualified := strings.Cut(name, ":"); qualified {
		index, found := r.byKey[namespace+":"+bare]
		if !found {
			return Entry{}, &UnknownLensError{Name: name}
		}
		return r.entries[index], nil
	}
	matches := r.matching(name)
	switch len(matches) {
	case 0:
		return Entry{}, &UnknownLensError{Name: name}
	case 1:
		return r.entries[matches[0]], nil
	}
	return Entry{}, &AmbiguousLensError{Name: name, Candidates: r.candidates(matches)}
}

// matching returns every entry a bare name reaches: by its own name, by an
// alias, or as the final segment of a path.
func (r *Registry) matching(name string) []int {
	matches := []int{}
	seen := map[int]bool{}
	for _, namespace := range order {
		if index, found := r.byKey[string(namespace)+":"+name]; found && !seen[index] {
			seen[index] = true
			matches = append(matches, index)
		}
	}
	for _, index := range r.bySuffix[name] {
		if !seen[index] {
			seen[index] = true
			matches = append(matches, index)
		}
	}
	return matches
}

// candidates names each entry the shortest way that resolves, so every line the
// caller is offered is a line it can run.
func (r *Registry) candidates(matches []int) []string {
	names := make([]string, 0, len(matches))
	for _, index := range matches {
		names = append(names, r.shortest(index))
	}
	sort.Strings(names)
	return names
}

// Group is one namespace's entry names, for the index.
type Group struct {
	Namespace Namespace
	Names     []string
}

// Index returns every entry grouped by namespace, aliases excluded, each named
// in the shortest form that resolves to it — so every name printed is a name the
// caller can type back verbatim.
func (r *Registry) Index() []Group {
	groups := make([]Group, 0, len(order))
	for _, namespace := range order {
		names := []string{}
		for i, e := range r.entries {
			if e.Namespace == namespace {
				names = append(names, r.shortest(i))
			}
		}
		if len(names) == 0 {
			continue
		}
		if namespace != NamespaceCheck {
			// Checks are listed structural, then verification, then advisory,
			// which is information. Nothing orders the others, so they read
			// better alphabetically.
			sort.Strings(names)
		}
		groups = append(groups, Group{Namespace: namespace, Names: names})
	}
	return groups
}

// shortest is the least a caller can type and still reach this entry: its final
// segment where nothing else answers to that, its full name where that resolves,
// and its qualified name otherwise.
func (r *Registry) shortest(index int) string {
	entry := r.entries[index]
	if _, last, dotted := cutLast(entry.Name, "."); dotted && r.reaches(last, index) {
		return last
	}
	if r.reaches(entry.Name, index) {
		return entry.Name
	}
	return entry.Qualified()
}

// reaches reports whether a bare name resolves to exactly this entry.
func (r *Registry) reaches(name string, index int) bool {
	matches := r.matching(name)
	return len(matches) == 1 && matches[0] == index
}

// All returns every entry in registration order.
func (r *Registry) All() []Entry {
	return r.entries
}
