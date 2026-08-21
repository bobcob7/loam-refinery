package entry

import "github.com/bobcob7/loam-refinery/internal/review"

// ChecksProvider contributes check:* entries straight from the check
// registries, so a check is explainable the moment it is registered.
type ChecksProvider struct {
	checks []review.Check
}

// NewChecksProvider takes the registries in reporting order.
func NewChecksProvider(groups ...[]review.Check) *ChecksProvider {
	checks := []review.Check{}
	for _, group := range groups {
		checks = append(checks, group...)
	}
	return &ChecksProvider{checks: checks}
}

// Name identifies the provider.
func (p *ChecksProvider) Name() string {
	return "checks"
}

// Entries returns one entry per check, in registry order. check.Summary is
// deliberately not carried over: nothing renders it (docs/cli.md §7.2 —
// refinery-rvw), and a field this provider does not read cannot leak into
// an entry only to drift silently from the doc that once claimed a
// consumer for it.
func (p *ChecksProvider) Entries() ([]Entry, error) {
	entries := make([]Entry, 0, len(p.checks))
	for _, check := range p.checks {
		entries = append(entries, Entry{
			Name:      check.Name,
			Namespace: NamespaceCheck,
			Aliases:   check.Aliases,
			Title:     check.Title,
			Body:      check.Body,
			Related:   check.Related,
		})
	}
	return entries, nil
}
