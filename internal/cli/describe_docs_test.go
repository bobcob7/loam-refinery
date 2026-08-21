package cli

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readFile reads path as a string, failing the test loudly rather than
// letting a missing or unreadable file surface as a confusing empty-set
// comparison further down.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	return string(data)
}

// describeDoc is describe.txt itself, read fresh at test time rather than
// through the embedded describeText: the point of this file is to catch a
// docs/cli.md design principle 2 (see the amendment there) gap between the
// hand-written summary and the schema, so the test has to read the same
// bytes a person editing describe.txt would.
const describeDoc = "describe.txt"

// fieldLine matches one field row in describe.txt's fixed-width blocks,
// e.g. "  ref       40-char lowercase commit SHA; required, read by every
// anchor". Exactly two leading spaces before the name is what tells a real
// field row apart from a wrapped continuation line — describe.txt's
// "category" row wraps its enum onto a further-indented second line with no
// field name of its own ("               | documentation | style"), and
// requiring the name to start in column three, immediately after the
// required two spaces, is what excludes it without needing a total-column
// count.
var fieldLine = regexp.MustCompile(`^  ([a-z][a-z_]*)\s`)

// describeBlockFields extracts the field names describe.txt lists under
// heading (one of "root", "comment", "suggestion") — a line consisting of
// exactly that word, followed by indented field rows, up to the next blank
// line. Loud failure at every step, the same discipline
// TestParseAcceptsExactlyTheDocumentedConfigKeys uses on docs/config.md's
// key table (cf9f47a): a heading that moves or a block that goes empty
// must fail this test, not silently compare two empty sets and pass.
func describeBlockFields(t *testing.T, text, heading string) []string {
	t.Helper()
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if line == heading {
			start = i
			break
		}
	}
	require.GreaterOrEqualf(t, start, 0, "no %q heading line found in %s", heading, describeDoc)
	var fields []string
	for _, line := range lines[start+1:] {
		if strings.TrimSpace(line) == "" {
			break
		}
		if m := fieldLine.FindStringSubmatch(line); m != nil {
			fields = append(fields, m[1])
		}
	}
	require.NotEmptyf(t, fields, "extracted zero fields from the %q block in %s — its shape likely changed", heading, describeDoc)
	return fields
}

// schemaProperties returns the sorted property names of node — the
// top-level review schema, or one of its $defs — read from
// schema.Annotated() so this compares against the same bytes submit-review
// validates against, not a copy.
func schemaProperties(t *testing.T, node map[string]any) []string {
	t.Helper()
	properties, ok := node["properties"].(map[string]any)
	require.Truef(t, ok, "schema node has no properties object")
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	require.NotEmpty(t, names)
	return names
}

// annotatedSchemaDoc decodes schema.Annotated() once per call.
func annotatedSchemaDoc(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(schema.Annotated(), &doc))
	return doc
}

// schemaDef returns $defs.name from doc, failing loudly if either is
// missing — a $defs section that gets renamed or restructured must not
// silently make the comparisons below pass by comparing against nothing.
func schemaDef(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	defs, ok := doc["$defs"].(map[string]any)
	require.Truef(t, ok, "schema has no $defs section")
	def, ok := defs[name].(map[string]any)
	require.Truef(t, ok, "schema $defs has no %q", name)
	return def
}

func sortedCopy(fields []string) []string {
	out := append([]string(nil), fields...)
	sort.Strings(out)
	return out
}

// TestDescribeRootFieldsMatchSchema pins describe.txt's root block against
// review.schema.json's own top-level properties. describe --lens renders
// from the schema through the entry providers, but describe's summary
// (describe.txt) is hand-written prose connected to the schema by nothing
// mechanical — the gap docs/cli.md's design principle 2 amendment
// describes, and the one that let the "ref" optional-versus-required drift
// and the missing "profile" field both ship unnoticed. This closes it in
// one direction the way TestParseAcceptsExactlyTheDocumentedConfigKeys
// closes it for config keys: not by generating the prose, but by pinning
// the field *list* against the real artifact, so a field added to (or
// removed from) either side without the other fails here.
func TestDescribeRootFieldsMatchSchema(t *testing.T) {
	t.Parallel()
	data := readFile(t, describeDoc)
	got := sortedCopy(describeBlockFields(t, data, "root"))
	want := schemaProperties(t, annotatedSchemaDoc(t))
	assert.Equal(t, want, got, "describe.txt's root block must list exactly review.schema.json's top-level properties")
}

// TestDescribeCommentFieldsMatchSchema is TestDescribeRootFieldsMatchSchema
// for the "comment" block against $defs.comment.
func TestDescribeCommentFieldsMatchSchema(t *testing.T) {
	t.Parallel()
	data := readFile(t, describeDoc)
	got := sortedCopy(describeBlockFields(t, data, "comment"))
	want := schemaProperties(t, schemaDef(t, annotatedSchemaDoc(t), "comment"))
	assert.Equal(t, want, got, "describe.txt's comment block must list exactly review.schema.json's $defs.comment properties")
}

// TestDescribeSuggestionFieldsMatchSchema is
// TestDescribeRootFieldsMatchSchema for the "suggestion" block against
// $defs.suggestion.
func TestDescribeSuggestionFieldsMatchSchema(t *testing.T) {
	t.Parallel()
	data := readFile(t, describeDoc)
	got := sortedCopy(describeBlockFields(t, data, "suggestion"))
	want := schemaProperties(t, schemaDef(t, annotatedSchemaDoc(t), "suggestion"))
	assert.Equal(t, want, got, "describe.txt's suggestion block must list exactly review.schema.json's $defs.suggestion properties")
}
