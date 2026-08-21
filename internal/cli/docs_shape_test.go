package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// backtickRun returns the number of leading backtick characters in s.
func backtickRun(s string) int {
	n := 0
	for _, r := range s {
		if r != '`' {
			break
		}
		n++
	}
	return n
}

// docSectionEnd returns the offset, within text, of the next heading line
// (any level) that follows the heading line found at anchorPos — the end
// of the section anchorPos's own heading opened. A line inside a fenced
// code block is never read as a heading, even when it starts with "#":
// §12.3's own worked example nests a rendered "## backend:..." line inside
// a fence to demonstrate the forgery defence, and CommonMark never reads
// that as a real heading either. Bounding a search to
// [anchorPos, docSectionEnd(...)) is what stops a deleted block from
// silently rolling into the next section's own fenced block and comparing
// against the wrong thing (refinery-xlp.9): without this bound, a stale
// occurrence count would still find *a* block, just the wrong one, and
// pass.
func docSectionEnd(text string, anchorPos int) int {
	lines := strings.SplitAfter(text[anchorPos:], "\n")
	pos := anchorPos
	fenceLen := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		run := backtickRun(trimmed)
		switch {
		case fenceLen == 0 && run >= 3:
			fenceLen = run
		case fenceLen > 0 && run >= fenceLen && run == len(trimmed):
			fenceLen = 0
		case fenceLen == 0 && i > 0 && strings.HasPrefix(trimmed, "#"):
			return pos
		}
		pos += len(line)
	}
	return len(text)
}

// findFenceClose scans lines starting at offset from in text for the first
// line whose trimmed content is entirely backticks, at least closeLen of
// them — CommonMark's actual fence-closing rule, matched by line shape
// rather than by the first bare "```" substring anywhere in the content.
// That distinction matters here: a fenced code excerpt can contain its own
// embedded backtick runs, mid-line or on a shorter fence of its own
// (§12.3's own fixture is built to demonstrate exactly this), and neither
// may close the real fence early. Returns the offset where the closing
// line begins (the block's own end) and the offset just past its trailing
// newline (where a subsequent search should resume), or (-1, -1) if no
// such line exists.
func findFenceClose(text string, from, closeLen int) (blockEnd, nextPos int) {
	lines := strings.SplitAfter(text[from:], "\n")
	pos := from
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && len(trimmed) >= closeLen && strings.Trim(trimmed, "`") == "" {
			return pos, pos + len(line)
		}
		pos += len(line)
	}
	return -1, -1
}

// docFencedBlock returns the occurrence-th fenced block whose opening line
// begins with fence (e.g. "```json" or the five-backtick "markdown" fence
// §12.3 uses), found after anchor and before the next heading, in the
// document at path, as raw trimmed text — no comment-stripping, no
// parsing. docJSONBlock,
// docJSONFragment, and a caller that wants a worked example's exact input
// or output bytes (refinery-xlp.9's "read the blocks from the file the way
// the output assertions already do") all build on this one search, and its
// close is found by findFenceClose rather than a bare "```" substring scan
// for the same reason docSectionEnd is fence-aware: content between the
// fences can itself contain backticks that must not end the block early.
func docFencedBlock(t *testing.T, path, anchor, fence string, occurrence int) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	at := strings.Index(text, anchor)
	require.GreaterOrEqualf(t, at, 0, "anchor %q not found in %s", anchor, path)
	rest := text[at:docSectionEnd(text, at)]
	closeLen := backtickRun(fence)
	pos := 0
	for n := 1; ; n++ {
		open := strings.Index(rest[pos:], fence)
		require.GreaterOrEqualf(t, open, 0, "%s block #%d after %q not found before the next heading in %s", fence, occurrence, anchor, path)
		openLineEnd := pos + open + len(fence)
		if nl := strings.IndexByte(rest[openLineEnd:], '\n'); nl >= 0 {
			openLineEnd += nl + 1
		} else {
			openLineEnd = len(rest)
		}
		blockEnd, nextPos := findFenceClose(rest, openLineEnd, closeLen)
		require.GreaterOrEqualf(t, blockEnd, 0, "unterminated %s block after %q in %s", fence, anchor, path)
		block := strings.TrimSpace(rest[openLineEnd:blockEnd])
		pos = nextPos
		if n == occurrence {
			return block
		}
	}
}

// docJSONBlock returns the occurrence-th fenced ```json block that appears
// after anchor, and before the next heading, in the document at path, as a
// parsed value. anchor is ordinarily a section heading, so a stale line
// number never has to be maintained here — only the section title, which
// is what a docs edit would also have to change.
func docJSONBlock(t *testing.T, path, anchor string, occurrence int) any {
	t.Helper()
	block := docBlockComment.ReplaceAllString(docFencedBlock(t, path, anchor, "```json", occurrence), "")
	var parsed any
	require.NoErrorf(t, json.Unmarshal([]byte(block), &parsed),
		"json block #%d after %q in %s did not parse: %s", occurrence, anchor, path, block)
	return parsed
}

// docJSONFragment is docJSONBlock for a fenced block that is valid JSON
// only once wrapped in an object — docs/config.md §6.3's "unreadable": 2,
// for instance, which is shown alone because it is described as sitting
// "on the envelope, next to total" rather than as a document of its own.
func docJSONFragment(t *testing.T, path, anchor string, occurrence int) any {
	t.Helper()
	block := docFencedBlock(t, path, anchor, "```json", occurrence)
	var parsed any
	require.NoErrorf(t, json.Unmarshal([]byte("{"+block+"}"), &parsed),
		"json fragment #%d after %q in %s did not parse once wrapped: %s", occurrence, anchor, path, block)
	return parsed
}

// realJSON runs a real reviews or submit-review invocation through the App and
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

// TestDocSectionEnd_BoundsToTheNextHeading is refinery-xlp.8's regression
// test for (c): docJSONBlock and docJSONFragment must stop searching for a
// fenced block at the next heading, not silently roll into a later
// section's own block and compare against the wrong thing. This pins
// docSectionEnd directly rather than routing the same check through
// docJSONBlock's own require-based failure: a failed subtest always fails
// its parent test regardless of what the parent asserts about the
// subtest's outcome afterward, so "docJSONBlock fails to find a block
// before the next heading" cannot be asserted from inside another test.
//
// Mutation this kills: reverting docSectionEnd to return len(text)
// unconditionally (the old "search to end of file" behavior) would leave
// the later section's own "```json" block inside the bounded slice, and
// the NotContains assertion below fails.
func TestDocSectionEnd_BoundsToTheNextHeading(t *testing.T) {
	t.Parallel()
	content := "### 1 First section\n\nno json block here at all.\n\n### 2 Second section\n\n```json\n{\"from\": \"second\"}\n```\n"
	at := strings.Index(content, "### 1 First section")
	require.GreaterOrEqual(t, at, 0)
	require.Contains(t, content[at:], "```json", "sanity: unbounded, the remainder does reach a later section's block")
	end := docSectionEnd(content, at)
	assert.True(t, strings.HasPrefix(content[end:], "### 2 Second section"), "the bound must land exactly at the next heading, got %q", content[end:end+20])
	assert.NotContains(t, content[at:end], "```json", "bounding to the next heading must exclude that later section's own block")
}

// TestDocJSONBlock_FindsItsOwnSectionsBlockEvenWithALaterSectionPresent is
// the positive complement above: docSectionEnd's bound must not be so tight
// that it also excludes a block that genuinely belongs to the anchored
// section, just because another section with its own block follows it.
func TestDocJSONBlock_FindsItsOwnSectionsBlockEvenWithALaterSectionPresent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "doc.md")
	content := "### 1 First section\n\n```json\n{\"from\": \"first\"}\n```\n\n### 2 Second section\n\n```json\n{\"from\": \"second\"}\n```\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	got := docJSONBlock(t, path, "### 1 First section", 1)
	assert.Equal(t, map[string]any{"from": "first"}, got)
}
