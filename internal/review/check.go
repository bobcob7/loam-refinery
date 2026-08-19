package review

// Tier is which question a check answers: is this a document, is it about
// anything real, or is it a good review.
type Tier string

const (
	// TierStructural checks make a document usable. Always run, always hard.
	TierStructural Tier = "structural"
	// TierVerification checks anchor claims against a repository. Hard when they run.
	TierVerification Tier = "verification"
	// TierAdvisory checks review quality. Always run, never hard.
	TierAdvisory Tier = "advisory"
)

// Check is one named check and the explanation describe renders for it. Check
// names are API: they appear in diagnostics, in --disable and --warn-only
// arguments, and in pointer lines a caller copies and runs.
type Check struct {
	Name    string
	Tier    Tier
	Summary string
	Title   string
	Body    string
	Related []string
	Aliases []string
}
