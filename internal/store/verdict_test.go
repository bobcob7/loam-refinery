package store

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	reviewschema "github.com/bobcob7/loam-refinery/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkConstraintPattern matches the runs table's verdict CHECK constraint
// in the embedded sql/schema.sql, capturing the comma-separated, single-
// quoted list inside "IN (...)".
var checkConstraintPattern = regexp.MustCompile(`verdict\s+TEXT\s+CHECK\s*\(verdict IN \(([^)]*)\)\)`)

// checkConstraintVerdicts extracts the values the runs table's verdict
// CHECK constraint accepts from the embedded sql/schema.sql. It fails
// loudly via require rather than returning an empty slice when the
// constraint is not found exactly where expected, so a rewritten
// constraint that this regex no longer matches cannot make
// TestVerdictsMatchTheSchemaAndConstraint pass by comparing against nothing.
func checkConstraintVerdicts(t *testing.T) []string {
	t.Helper()
	m := checkConstraintPattern.FindStringSubmatch(schema)
	require.NotNilf(t, m, "verdict CHECK constraint not found in embedded sql/schema.sql (looked for %s)", checkConstraintPattern)
	var out []string
	for _, raw := range strings.Split(m[1], ",") {
		out = append(out, strings.Trim(strings.TrimSpace(raw), "'"))
	}
	return out
}

// schemaEnumVerdicts extracts review.schema.json's properties.verdict.enum
// from the embedded schema internal/schema serves, the same bytes
// `loam-refinery schema` publishes.
func schemaEnumVerdicts(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Properties struct {
			Verdict struct {
				Enum []string `json:"enum"`
			} `json:"verdict"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(reviewschema.Annotated(), &doc), "decoding the embedded review schema")
	require.NotEmpty(t, doc.Properties.Verdict.Enum, "review.schema.json's properties.verdict has no enum")
	return doc.Properties.Verdict.Enum
}

// TestVerdictsMatchTheSchemaAndConstraint pins Verdicts() — the one source
// of truth cmd/loam-refinery's validVerdict consults — against the runs
// table's CHECK constraint and review.schema.json's enum, the two other
// places the same three words used to be retyped by hand with nothing
// comparing them (refinery-xlp.13). Add a fourth verdict to only one of the
// three and this test is what turns red.
func TestVerdictsMatchTheSchemaAndConstraint(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, Verdicts(), checkConstraintVerdicts(t), "Verdicts() and the runs table's CHECK constraint (sql/schema.sql) disagree")
	assert.ElementsMatch(t, Verdicts(), schemaEnumVerdicts(t), "Verdicts() and review.schema.json's verdict enum disagree")
}
