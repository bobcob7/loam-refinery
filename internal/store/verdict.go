package store

// Verdicts lists the values a review's verdict may hold: the runs table's
// verdict CHECK constraint (config.md §4.5.2, sql/schema.sql) and
// review.schema.json's own verdict enum both name exactly these three
// words. It is the one place that list is spelled out in Go — callers like
// cmd/loam-refinery's validVerdict consult it rather than retyping the
// words themselves — and TestVerdictsMatchTheSchemaAndConstraint pins it
// against the other two artifacts, so a verdict added to one without the
// others shows up as a failing test rather than a silently dropped column.
func Verdicts() []string {
	return []string{"approve", "request_changes", "comment"}
}
