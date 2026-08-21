package config

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file backs refinery-a96.22 and refinery-a96.23 for the config file
// (docs/config.md §3): the full example object is reproduced by parsing it
// for real, and the accepted key set is pinned so a key added beside
// version/store.enabled/store.path/store.repos fails a test rather than
// shipping unnoticed. docs/config.md §3's second example — the
// defaults-only file — is already pinned byte for byte by
// TestMaterialize_FirstRunWritesExactDefaults; this covers the one example
// in that section nothing else does.

const configDoc = "../../docs/config.md"

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
// code block is never read as a heading, even when it starts with "#".
// Bounding a search to [anchorPos, docSectionEnd(...)) is what stops a
// deleted block from silently rolling into the next section's own fenced
// block and comparing against the wrong thing, the same way
// internal/cli/docs_shape_test.go's docSectionEnd does.
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
// Returns the offset where the closing line begins (the block's own end)
// and the offset just past its trailing newline (where a subsequent search
// should resume), or (-1, -1) if no such line exists.
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

// docJSONBlock returns the occurrence-th fenced ```json block after anchor,
// and before the next heading, in the document at path, as raw bytes —
// reading the documentation fresh each run rather than copying its example
// into a Go literal, so an edited example changes what this test expects
// without anyone touching this file. The search is bounded to anchor's own
// section (docSectionEnd) and closes each fence by line shape
// (findFenceClose), matching internal/cli/docs_shape_test.go's
// docFencedBlock: an edited example must not be silently answered by a
// later section's own block.
func docJSONBlock(t *testing.T, path, anchor string, occurrence int) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	at := strings.Index(text, anchor)
	require.GreaterOrEqualf(t, at, 0, "anchor %q not found in %s", anchor, path)
	rest := text[at:docSectionEnd(text, at)]
	const fence = "```json"
	closeLen := backtickRun(fence)
	pos := 0
	for n := 1; ; n++ {
		open := strings.Index(rest[pos:], fence)
		require.GreaterOrEqualf(t, open, 0, "json block #%d after %q not found before the next heading in %s", occurrence, anchor, path)
		openLineEnd := pos + open + len(fence)
		if nl := strings.IndexByte(rest[openLineEnd:], '\n'); nl >= 0 {
			openLineEnd += nl + 1
		} else {
			openLineEnd = len(rest)
		}
		blockEnd, nextPos := findFenceClose(rest, openLineEnd, closeLen)
		require.GreaterOrEqualf(t, blockEnd, 0, "unterminated json block after %q in %s", anchor, path)
		block := strings.TrimSpace(rest[openLineEnd:blockEnd])
		pos = nextPos
		if n == occurrence {
			return []byte(block)
		}
	}
}

// TestParse_AcceptsTheDocumentedFullExample reproduces docs/config.md §3's
// first example — the one with all three store keys set — through parse
// itself, so a key the doc shows and parse no longer accepts (or vice
// versa) fails here.
func TestParse_AcceptsTheDocumentedFullExample(t *testing.T) {
	t.Parallel()
	raw := docJSONBlock(t, configDoc, "## 3. The config file", 1)
	cfg, err := parse(raw, "config.json")
	require.NoError(t, err, "docs/config.md §3's full example must still parse: %s", raw)
	assert.Equal(t, "1", cfg.Version)
	assert.True(t, cfg.Store.Enabled)
	assert.True(t, strings.HasSuffix(cfg.Store.Path, "/reviews"), "store.path %q must expand ~/reviews", cfg.Store.Path)
	assert.Equal(t, map[string]string{"/Users/me/src/refinery": "github.com/bobcob7/loam-refinery"}, cfg.Store.Repos)
	// The example's own keys, read back out of the same bytes just parsed,
	// must be exactly the four documented ones — the structural half of
	// the comparison, independent of the values above.
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &root))
	topKeys := sortedKeys(root)
	assert.Equal(t, []string{"store", "version"}, topKeys, "docs/config.md §3's example must itself use only the documented top-level keys")
	var store map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(root["store"], &store))
	assert.Equal(t, []string{"enabled", "path", "repos"}, sortedKeys(store),
		"docs/config.md §3's example must itself use only the documented store keys")
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// docConfigKeyTable finds the key table in docs/config.md §3 — the one
// whose first column names the accepted keys ("version", "store.enabled",
// ...) — and returns its first-column values verbatim, backticks and
// surrounding whitespace stripped. It reads the section between the "## 3.
// The config file" heading and the next "## " heading, so a table moved
// into a different section is not silently picked up from somewhere else.
//
// This is deliberately strict rather than best-effort: if the heading, the
// header row, or the separator row is not found exactly where expected, the
// test fails loudly via require rather than falling through to an empty
// result that would make TestParseAcceptsExactlyTheDocumentedConfigKeys
// pass by comparing two empty sets.
func docConfigKeyTable(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	const heading = "## 3. The config file"
	start := strings.Index(text, heading)
	require.GreaterOrEqualf(t, start, 0, "heading %q not found in %s", heading, path)
	section := text[start+len(heading):]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	lines := strings.Split(section, "\n")
	headerIdx := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "| Key ") || strings.TrimSpace(line) == "| Key |" {
			headerIdx = i
			break
		}
	}
	require.GreaterOrEqualf(t, headerIdx, 0, "no %q table header found under %q in %s", "| Key |", heading, path)
	require.Greaterf(t, len(lines), headerIdx+1, "table header %q has no separator row in %s", strings.TrimSpace(lines[headerIdx]), path)
	separator := strings.TrimSpace(lines[headerIdx+1])
	require.Truef(t, strings.HasPrefix(separator, "|") && strings.Trim(separator, "|- ") == "",
		"row after %q is not a %q separator row (got %q) in %s", "| Key |", "| --- |", separator, path)
	var keys []string
	for _, line := range lines[headerIdx+2:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		cells := strings.Split(trimmed, "|")
		require.GreaterOrEqualf(t, len(cells), 3, "malformed table row %q in %s", trimmed, path)
		key := strings.Trim(strings.TrimSpace(cells[1]), "`")
		require.NotEmptyf(t, key, "empty key cell in row %q in %s", trimmed, path)
		keys = append(keys, key)
	}
	require.NotEmptyf(t, keys, "extracted zero keys from the %q table in %s — the table's shape likely changed", heading, path)
	return keys
}

// splitDocConfigKeys separates docConfigKeyTable's raw first-column values
// ("version", "store.enabled", ...) into the set of top-level keys and the
// set of store.* keys, the same split topLevelKeys and storeKeys make in
// config.go.
func splitDocConfigKeys(t *testing.T, rawKeys []string) (top []string, store []string) {
	t.Helper()
	topSeen := map[string]bool{}
	storeSeen := map[string]bool{}
	for _, raw := range rawKeys {
		parts := strings.SplitN(raw, ".", 2)
		if len(parts) == 1 {
			if !topSeen[parts[0]] {
				topSeen[parts[0]] = true
				top = append(top, parts[0])
			}
			continue
		}
		require.Equalf(t, "store", parts[0], "documented key %q has an unrecognised top-level prefix", raw)
		if !topSeen[parts[0]] {
			topSeen[parts[0]] = true
			top = append(top, parts[0])
		}
		if !storeSeen[parts[1]] {
			storeSeen[parts[1]] = true
			store = append(store, parts[1])
		}
	}
	sort.Strings(top)
	sort.Strings(store)
	return top, store
}

// TestParseAcceptsExactlyTheDocumentedConfigKeys pins docs/config.md §3's
// closed set of accepted keys: version, store.enabled, store.path, and
// store.repos, and nothing beside them. parse rejects unknown keys
// outright (TestLoadFile_UnknownKeyNamesTheKey and
// TestLoadFile_TopLevelUnknownKey already cover that a typo is caught),
// but neither of those notices a wholly new key someone starts accepting
// on purpose. Unlike PRAGMA table_info(runs) or a flag.FlagSet, parse has
// no built-in reflectable list to walk, so config.go exposes its own —
// topLevelKeys and storeKeys — and this test compares those slices against
// keys extracted from docs/config.md §3's key table itself (docConfigKeyTable),
// rather than a copy of the key names retyped into this file. That is what
// makes this a real, bidirectional pin: a key added to either slice in
// config.go without a matching doc update fails the equality checks below,
// and a key added to (or removed from) the doc's table without a matching
// code change fails them too.
func TestParseAcceptsExactlyTheDocumentedConfigKeys(t *testing.T) {
	t.Parallel()
	docTop, docStore := splitDocConfigKeys(t, docConfigKeyTable(t, configDoc))
	gotTop := append([]string(nil), topLevelKeys...)
	sort.Strings(gotTop)
	gotStore := append([]string(nil), storeKeys...)
	sort.Strings(gotStore)
	assert.Equal(t, docTop, gotTop,
		"docs/config.md §3's top-level keys must match topLevelKeys in config.go")
	assert.Equal(t, docStore, gotStore,
		"docs/config.md §3's store keys must match storeKeys in config.go")
	base := map[string]any{
		"version": "1",
		"store": map[string]any{
			"enabled": true,
			"path":    "/tmp/store",
			"repos":   map[string]string{},
		},
	}
	raw, err := json.Marshal(base)
	require.NoError(t, err)
	_, err = parse(raw, "config.json")
	require.NoError(t, err, "the four documented keys together must parse")
	const extra = "undocumented-key"
	require.NotContains(t, topLevelKeys, extra)
	require.NotContains(t, storeKeys, extra)
	t.Run("top-level "+extra, func(t *testing.T) {
		t.Parallel()
		mutated := map[string]any{"version": "1", "store": base["store"], extra: "x"}
		raw, err := json.Marshal(mutated)
		require.NoError(t, err)
		_, err = parse(raw, "config.json")
		assert.Error(t, err, "a key %q outside topLevelKeys must be rejected", extra)
	})
	t.Run("store "+extra, func(t *testing.T) {
		t.Parallel()
		store := map[string]any{"enabled": true, "path": "/tmp/store", "repos": map[string]string{}, extra: "x"}
		mutated := map[string]any{"version": "1", "store": store}
		raw, err := json.Marshal(mutated)
		require.NoError(t, err)
		_, err = parse(raw, "config.json")
		assert.Error(t, err, "a key %q outside storeKeys must be rejected", extra)
	})
}
