package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file backs refinery-a96.22: every JSON example in docs/config.md
// and docs/cli.md is reproduced by a real command and compared, by shape
// rather than by value, against the example itself. The comparison reads
// the documentation file fresh every run, rather than copying its example
// into a Go literal here, precisely so that an edited example changes what
// a test expects without anyone touching this file — the "vice versa" half
// of the acceptance criteria: an example that drifts from the tool fails
// the build exactly as a tool that drifts from the example does. Values
// are never compared, since docs/config.md and docs/cli.md's examples use
// invented SHAs and paths chosen for readability, not reality — only the
// set of keys present, their nesting, and which fields are present versus
// absent, which is what carries meaning here (docs/config.md §6.1: a
// field's absence is itself part of the contract).

// docBlockComment strips the one non-JSON decoration this repository's
// docs use inside an otherwise-JSON fenced block: a "the rest look like
// the row above" placeholder written as a /* ... */ comment. Removing it
// leaves valid JSON (an empty array, in every case it appears), so the
// envelope around it — the only thing that block exists to show — still
// parses.
var docBlockComment = regexp.MustCompile(`/\*.*?\*/`)

// docJSONBlock returns the occurrence-th fenced ```json block that appears
// after anchor in the document at path, as a parsed value. anchor is
// ordinarily a section heading, so a stale line number never has to be
// maintained here — only the section title, which is what a docs edit
// would also have to change.
func docJSONBlock(t *testing.T, path, anchor string, occurrence int) any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	at := strings.Index(text, anchor)
	require.GreaterOrEqualf(t, at, 0, "anchor %q not found in %s", anchor, path)
	rest := text[at:]
	pos := 0
	for n := 1; ; n++ {
		open := strings.Index(rest[pos:], "```json")
		require.GreaterOrEqualf(t, open, 0, "json block #%d after %q not found in %s", occurrence, anchor, path)
		start := pos + open + len("```json")
		closeAt := strings.Index(rest[start:], "```")
		require.GreaterOrEqualf(t, closeAt, 0, "unterminated json block after %q in %s", anchor, path)
		block := strings.TrimSpace(rest[start : start+closeAt])
		pos = start + closeAt + len("```")
		if n == occurrence {
			block = docBlockComment.ReplaceAllString(block, "")
			var parsed any
			require.NoErrorf(t, json.Unmarshal([]byte(block), &parsed),
				"json block #%d after %q in %s did not parse: %s", occurrence, anchor, path, block)
			return parsed
		}
	}
}

// docJSONFragment is docJSONBlock for a fenced block that is valid JSON
// only once wrapped in an object — docs/config.md §6.3's "unreadable": 2,
// for instance, which is shown alone because it is described as sitting
// "on the envelope, next to total" rather than as a document of its own.
func docJSONFragment(t *testing.T, path, anchor string, occurrence int) any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	at := strings.Index(text, anchor)
	require.GreaterOrEqualf(t, at, 0, "anchor %q not found in %s", anchor, path)
	rest := text[at:]
	pos := 0
	for n := 1; ; n++ {
		open := strings.Index(rest[pos:], "```json")
		require.GreaterOrEqualf(t, open, 0, "json block #%d after %q not found in %s", occurrence, anchor, path)
		start := pos + open + len("```json")
		closeAt := strings.Index(rest[start:], "```")
		require.GreaterOrEqualf(t, closeAt, 0, "unterminated json block after %q in %s", anchor, path)
		block := strings.TrimSpace(rest[start : start+closeAt])
		pos = start + closeAt + len("```")
		if n == occurrence {
			var parsed any
			require.NoErrorf(t, json.Unmarshal([]byte("{"+block+"}"), &parsed),
				"json fragment #%d after %q in %s did not parse once wrapped: %s", occurrence, anchor, path, block)
			return parsed
		}
	}
}

// realJSON runs a real reviews or validate invocation through the App and
// parses its stdout, so "the tool" in "compared against real command
// output" means exactly that — the same JSON encoder path production
// traffic goes through, not a hand-written stand-in for it.
func realJSON(t *testing.T, stdout string) any {
	t.Helper()
	var parsed any
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed), "output did not parse as json: %s", stdout)
	return parsed
}

// jsonShape reduces a parsed JSON value to its structure: for an object,
// the set of keys, each mapped to its own value's shape; for an array, the
// shape of its first element alone (docs/config.md's examples show one
// representative row, not an exhaustive one, so comparing further would
// compare array length rather than row shape); for anything else, a label
// naming its JSON kind. Two shapes being equal means two documents agree
// on which keys exist, how they nest, and what kind of thing each holds —
// never on the values themselves.
func jsonShape(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = jsonShape(vv)
		}
		return out
	case []any:
		if len(val) == 0 {
			return "array"
		}
		return []any{jsonShape(val[0])}
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case float64:
		return "number"
	default:
		return fmt.Sprintf("%T", val)
	}
}

// assertShapeMatchesDoc compares got's shape against the doc example at
// path/anchor/occurrence, failing with both shapes rendered so a real
// disagreement — not just a typo in the anchor — is legible.
func assertShapeMatchesDoc(t *testing.T, got any, path, anchor string, occurrence int, msg string) {
	t.Helper()
	want := jsonShape(docJSONBlock(t, path, anchor, occurrence))
	assert.Equal(t, want, jsonShape(got), msg)
}
