package store

// Assessments lists the values a review's assessment may hold:
// review.schema.json's own assessment enum names exactly these four words.
// It is the one place that list is spelled out in Go, the same role
// Verdicts() plays for verdict, and TestAssessmentsMatchTheSchema pins it
// against all three artifacts that name it — this list, the runs table's
// assessment CHECK constraint (sql/schema.sql), and review.schema.json's
// assessment enum — so a level added to one without the others shows up as
// a failing test rather than a silently dropped word.
func Assessments() []string {
	return []string{"strong", "sound", "mixed", "weak"}
}
