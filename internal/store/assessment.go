package store

// Assessments lists the values a review's assessment may hold:
// review.schema.json's own assessment enum names exactly these four words.
// It is the one place that list is spelled out in Go, the same role
// Verdicts() plays for verdict, and TestAssessmentsMatchTheSchema pins it
// against the schema so a level added to one without the other shows up as
// a failing test rather than a silently dropped word.
//
// There is no runs table CHECK constraint to pin against yet — refinery-dbk.6
// adds the assessment column and its CHECK. TestAssessmentsMatchTheSchema is
// scoped to the two artifacts that exist now; see its comment for how that
// bead extends it.
func Assessments() []string {
	return []string{"strong", "sound", "mixed", "weak"}
}
