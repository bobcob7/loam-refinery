package validate

import (
	"context"

	"github.com/bobcob7/loam-refinery/internal/review"
)

//go:generate moq -out moq_test.go . structuralChecker advisoryRunner repositoryFinder verifier

// structuralChecker runs the hard checks over a parsed document.
type structuralChecker interface {
	Check(doc *review.Document) []review.Diagnostic
}

// advisoryRunner runs every registered advisory. There is no way to silence
// one: advisories always run and are always reported.
type advisoryRunner interface {
	Run(doc *review.Document) ([]review.Diagnostic, []review.Skipped)
}

// repositoryFinder discovers the repository containing dir, the way git does.
type repositoryFinder interface {
	Find(ctx context.Context, dir string) (verifier, error)
}

// verifier resolves anchor claims against a repository.
type verifier interface {
	Verify(ctx context.Context, doc *review.Document) ([]review.Diagnostic, []review.Skipped, review.Verification)
}
