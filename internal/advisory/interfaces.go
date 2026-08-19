package advisory

import "github.com/bobcob7/loam-refinery/internal/review"

// checkFunc is one advisory's implementation over a parsed document. It returns
// the diagnostics it raised and the checks it could not run.
type checkFunc func(doc *review.Document) ([]review.Diagnostic, []review.Skipped)
