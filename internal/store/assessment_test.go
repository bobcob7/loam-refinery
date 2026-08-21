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

// checkConstraintAssessmentPattern matches the runs table's assessment CHECK
// constraint in the embedded sql/schema.sql, capturing the comma-separated,
// single-quoted list inside "IN (...)" — mirroring checkConstraintPattern in
// verdict_test.go.
var checkConstraintAssessmentPattern = regexp.MustCompile(`assessment\s+TEXT\s+CHECK\s*\(assessment IN \(([^)]*)\)\)`)

// checkConstraintAssessments extracts the values the runs table's assessment
// CHECK constraint accepts from the embedded sql/schema.sql, mirroring
// checkConstraintVerdicts. It fails loudly via require rather than
// returning an empty slice when the constraint is not found exactly where
// expected, so a rewritten constraint that this regex no longer matches
// cannot make TestAssessmentsMatchTheSchema pass by comparing against
// nothing.
func checkConstraintAssessments(t *testing.T) []string {
	t.Helper()
	m := checkConstraintAssessmentPattern.FindStringSubmatch(schema)
	require.NotNilf(t, m, "assessment CHECK constraint not found in embedded sql/schema.sql (looked for %s)", checkConstraintAssessmentPattern)
	var out []string
	for _, raw := range strings.Split(m[1], ",") {
		out = append(out, strings.Trim(strings.TrimSpace(raw), "'"))
	}
	return out
}

// schemaEnumAssessments extracts review.schema.json's properties.assessment.enum
// from the embedded schema internal/schema serves, the same bytes
// `loam-refinery schema` publishes — mirroring schemaEnumVerdicts in
// verdict_test.go.
func schemaEnumAssessments(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Properties struct {
			Assessment struct {
				Enum []string `json:"enum"`
			} `json:"assessment"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(reviewschema.Annotated(), &doc), "decoding the embedded review schema")
	require.NotEmpty(t, doc.Properties.Assessment.Enum, "review.schema.json's properties.assessment has no enum")
	return doc.Properties.Assessment.Enum
}

// TestAssessmentsMatchTheSchema pins Assessments() against both
// review.schema.json's enum and the runs table's CHECK constraint, the way
// TestVerdictsMatchTheSchemaAndConstraint pins Verdicts() against the same
// two artifacts. refinery-dbk.6 added the assessment column and its CHECK,
// completing the three-way pin this test was left two-way for; add a level
// to only one of the three artifacts and this test is what turns red.
func TestAssessmentsMatchTheSchema(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, Assessments(), checkConstraintAssessments(t), "Assessments() and the runs table's CHECK constraint (sql/schema.sql) disagree")
	assert.ElementsMatch(t, Assessments(), schemaEnumAssessments(t), "Assessments() and review.schema.json's assessment enum disagree")
}
