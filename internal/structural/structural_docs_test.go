package structural

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docStructuralCheckNames finds the check table in docs/review-document.md
// §11.1 — the hard-checks table whose first column names the checks
// (document-unparseable, schema, id-unique, ...) — and returns those names
// verbatim, backticks and surrounding whitespace stripped. It reads the
// section between the "### 11.1 Structural checks — hard" heading and the
// next "### " heading, so a table moved into a different section is not
// silently picked up from somewhere else.
//
// This is deliberately strict rather than best-effort: if the heading, the
// header row, or the separator row is not found exactly where expected, the
// test fails loudly via require rather than falling through to an empty
// result that would make TestChecksMatchTheDocumentedTable pass by comparing
// two empty sets.
func docStructuralCheckNames(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "review-document.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	text := string(data)
	const heading = "### 11.1 Structural checks — hard"
	start := strings.Index(text, heading)
	require.GreaterOrEqualf(t, start, 0, "heading %q not found in %s", heading, path)
	section := text[start+len(heading):]
	if end := strings.Index(section, "\n### "); end >= 0 {
		section = section[:end]
	}
	lines := strings.Split(section, "\n")
	headerIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "| Check | Rule |" {
			headerIdx = i
			break
		}
	}
	require.GreaterOrEqualf(t, headerIdx, 0, "no %q table header found under %q in %s", "| Check | Rule |", heading, path)
	require.Greaterf(t, len(lines), headerIdx+1, "table header %q has no separator row in %s", strings.TrimSpace(lines[headerIdx]), path)
	separator := strings.TrimSpace(lines[headerIdx+1])
	require.Truef(t, strings.HasPrefix(separator, "|") && strings.Trim(separator, "|- ") == "",
		"row after %q is not a %q separator row (got %q) in %s", "| Check | Rule |", "| --- | --- |", separator, path)
	var names []string
	for _, line := range lines[headerIdx+2:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		cells := strings.Split(trimmed, "|")
		require.GreaterOrEqualf(t, len(cells), 3, "malformed table row %q in %s", trimmed, path)
		name := strings.Trim(strings.TrimSpace(cells[1]), "`")
		require.NotEmptyf(t, name, "empty check-name cell in row %q in %s", trimmed, path)
		names = append(names, name)
	}
	require.NotEmptyf(t, names, "extracted zero checks from the %q table in %s — the table's shape likely changed", heading, path)
	return names
}

// TestChecksMatchTheDocumentedTable pins the structural check registry
// against docs/review-document.md §11.1's own table instead of a
// hand-written list retyped into this file. A retyped list only catches code
// drift; it stays green when a row is added to the doc table and nothing
// implements it, which is exactly the gap that let profile-format sit in the
// doc with no Go check behind it. Reading the table itself makes the pin
// bidirectional: a row added to the table without a matching check fails
// here, and a check added to the registry without a documenting row fails
// the same way.
func TestChecksMatchTheDocumentedTable(t *testing.T) {
	t.Parallel()
	documented := docStructuralCheckNames(t)
	code := []string{}
	for _, check := range Checks() {
		code = append(code, check.Name)
	}
	assert.ElementsMatch(t, documented, code)
}
