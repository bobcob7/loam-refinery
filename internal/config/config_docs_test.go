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

// docJSONBlock returns the occurrence-th fenced ```json block after anchor
// in the document at path, as raw bytes — reading the documentation fresh
// each run rather than copying its example into a Go literal, so an edited
// example changes what this test expects without anyone touching this
// file.
func docJSONBlock(t *testing.T, path, anchor string, occurrence int) []byte {
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

// TestParseAcceptsExactlyTheDocumentedConfigKeys pins docs/config.md §3's
// closed set of accepted keys: version, store.enabled, store.path, and
// store.repos, and nothing beside them. parse rejects unknown keys
// outright (TestLoadFile_UnknownKeyNamesTheKey and
// TestLoadFile_TopLevelUnknownKey already cover that a typo is caught),
// but neither of those notices a wholly new key someone starts accepting
// on purpose. Unlike PRAGMA table_info(runs) or a flag.FlagSet, parse has
// no built-in reflectable list to walk, so config.go exposes its own —
// topLevelKeys and storeKeys — and parse rejects against those same
// slices rather than a separate literal. Reading topLevelKeys and
// storeKeys directly here, instead of retyping the key names, is what
// makes this a real pin: a key added to either slice in config.go without
// a matching doc update fails the equality checks below, and a key
// documented but never added to the slices fails to parse.
func TestParseAcceptsExactlyTheDocumentedConfigKeys(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"version", "store"}, topLevelKeys,
		"docs/config.md §3's top-level keys must match the set parse actually accepts")
	assert.Equal(t, []string{"enabled", "path", "repos"}, storeKeys,
		"docs/config.md §3's store keys must match the set parse actually accepts")
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
