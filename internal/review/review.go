// Package review holds the review document types, the lenient parser that keeps
// a malformed document usable, and the diagnostic values every check tier
// produces.
package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// Field is one document field: its value, whether the key was present at all,
// and whether the value was of the type the format requires. Checks read OK
// before Value so that a single ill-typed field skips only the checks that need
// it, never the whole document.
type Field[T any] struct {
	Value   T
	Present bool
	OK      bool
}

// Document is a parsed review document. Every field is optional and every field
// may be ill-typed; the parser records what it found rather than refusing it.
type Document struct {
	Root     any
	Object   map[string]any
	Version  Field[string]
	Verdict  Field[string]
	Summary  Field[string]
	Ref      Field[string]
	Comments []Comment
	// CommentsPresent reports that the key exists, CommentsArray that it holds
	// an array, and CommentsWellTyped that every element is an object.
	CommentsPresent   bool
	CommentsArray     bool
	CommentsWellTyped bool
}

// Comment is one finding.
type Comment struct {
	Index            int
	Path             string
	Object           bool
	ID               Field[string]
	Priority         Field[int]
	Category         Field[string]
	Body             Field[string]
	Code             Field[string]
	Anchors          []Anchor
	AnchorsPresent   bool
	AnchorsArray     bool
	Suggestions      []Suggestion
	SuggestionsArray bool
}

// Anchor is one location a comment applies to. It carries no ref of its own: a
// review is of one change at one revision, and that revision is the document's.
type Anchor struct {
	Index   int
	Path    string
	Object  bool
	File    Field[string]
	Line    Field[int]
	EndLine Field[int]
}

// Suggestion is one proposed fix.
type Suggestion struct {
	Index   int
	Path    string
	Object  bool
	Summary Field[string]
	Effort  Field[string]
	Scope   Field[string]
	Pros    Field[[]string]
	Cons    Field[[]string]
	Code    Field[string]
}

var (
	errNotJSON   = errors.New("input is not valid JSON")
	errNotObject = errors.New("input must be a single JSON object")
)

// Parse decodes a review document. It fails only when the input is not a single
// JSON object: every other defect is recorded on the returned Document so that
// checks can degrade per field and per item.
func Parse(data []byte) (*Document, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotJSON, err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content after the first value", errNotObject)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, errNotObject
	}
	doc := &Document{
		Root:    root,
		Object:  obj,
		Version: stringField(obj, "version"),
		Verdict: stringField(obj, "verdict"),
		Summary: stringField(obj, "summary"),
		Ref:     stringField(obj, "ref"),
	}
	raw, present := obj["comments"]
	doc.CommentsPresent = present
	items, isArray := raw.([]any)
	doc.CommentsArray = isArray
	doc.CommentsWellTyped = isArray
	for i, item := range items {
		c := parseComment(i, item)
		if !c.Object {
			doc.CommentsWellTyped = false
		}
		doc.Comments = append(doc.Comments, c)
	}
	return doc, nil
}

func parseComment(i int, raw any) Comment {
	c := Comment{Index: i, Path: fmt.Sprintf("/comments/%d", i)}
	obj, ok := raw.(map[string]any)
	if !ok {
		return c
	}
	c.Object = true
	c.ID = stringField(obj, "id")
	c.Priority = intField(obj, "priority")
	c.Category = stringField(obj, "category")
	c.Body = stringField(obj, "body")
	c.Code = stringField(obj, "code")
	anchors, present := obj["anchors"]
	c.AnchorsPresent = present
	anchorItems, isArray := anchors.([]any)
	c.AnchorsArray = isArray
	for j, item := range anchorItems {
		c.Anchors = append(c.Anchors, parseAnchor(c.Path, j, item))
	}
	suggestions := obj["suggestions"]
	suggestionItems, isArray := suggestions.([]any)
	c.SuggestionsArray = isArray
	for j, item := range suggestionItems {
		c.Suggestions = append(c.Suggestions, parseSuggestion(c.Path, j, item))
	}
	return c
}

func parseAnchor(parent string, i int, raw any) Anchor {
	a := Anchor{Index: i, Path: fmt.Sprintf("%s/anchors/%d", parent, i)}
	obj, ok := raw.(map[string]any)
	if !ok {
		return a
	}
	a.Object = true
	a.File = stringField(obj, "file")
	a.Line = intField(obj, "line")
	a.EndLine = intField(obj, "end_line")
	return a
}

func parseSuggestion(parent string, i int, raw any) Suggestion {
	s := Suggestion{Index: i, Path: fmt.Sprintf("%s/suggestions/%d", parent, i)}
	obj, ok := raw.(map[string]any)
	if !ok {
		return s
	}
	s.Object = true
	s.Summary = stringField(obj, "summary")
	s.Effort = stringField(obj, "effort")
	s.Scope = stringField(obj, "scope")
	s.Pros = stringsField(obj, "pros")
	s.Cons = stringsField(obj, "cons")
	s.Code = stringField(obj, "code")
	return s
}

func stringField(obj map[string]any, key string) Field[string] {
	raw, present := obj[key]
	if !present {
		return Field[string]{}
	}
	value, ok := raw.(string)
	return Field[string]{Value: value, Present: true, OK: ok}
}

func intField(obj map[string]any, key string) Field[int] {
	raw, present := obj[key]
	if !present {
		return Field[int]{}
	}
	number, ok := raw.(json.Number)
	if !ok {
		return Field[int]{Present: true}
	}
	value, err := strconv.Atoi(number.String())
	if err != nil {
		return Field[int]{Present: true}
	}
	return Field[int]{Value: value, Present: true, OK: true}
}

func stringsField(obj map[string]any, key string) Field[[]string] {
	raw, present := obj[key]
	if !present {
		return Field[[]string]{}
	}
	items, ok := raw.([]any)
	if !ok {
		return Field[[]string]{Present: true}
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return Field[[]string]{Present: true}
		}
		values = append(values, s)
	}
	return Field[[]string]{Value: values, Present: true, OK: true}
}

// AnchorCount reports how many well-formed anchors the document carries.
func (d *Document) AnchorCount() int {
	n := 0
	for _, c := range d.Comments {
		for _, a := range c.Anchors {
			if a.Object {
				n++
			}
		}
	}
	return n
}
