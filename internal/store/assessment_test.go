package store

import (
	"encoding/json"
	"testing"

	reviewschema "github.com/bobcob7/loam-refinery/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestAssessmentsMatchTheSchema pins Assessments() against review.schema.json's
// enum, the way TestVerdictsMatchTheSchemaAndConstraint pins Verdicts()
// against both the schema and the runs table's CHECK constraint.
//
// This test is deliberately two-way, not three-way: the runs table carries
// no assessment column yet, so there is no CHECK constraint to compare
// against. refinery-dbk.6 adds that column, nullable, with a CHECK naming
// the same four words (mirroring verdict's CHECK exactly, per its own
// notes). When it does, extend this test the way
// TestVerdictsMatchTheSchemaAndConstraint checks the runs table today —
// add a checkConstraintAssessments helper alongside checkConstraintVerdicts
// and assert Assessments() against it here, so a level added to only one of
// the three artifacts turns this test red instead of shipping unnoticed.
func TestAssessmentsMatchTheSchema(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, Assessments(), schemaEnumAssessments(t), "Assessments() and review.schema.json's assessment enum disagree")
}
