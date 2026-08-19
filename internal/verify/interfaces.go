package verify

import "context"

//go:generate moq -out moq_test.go . gitRunner

// gitRunner runs one git command against the discovered repository. It is the
// only way this package touches a repository: object lookups, never content
// beyond a line count.
type gitRunner interface {
	run(ctx context.Context, args ...string) ([]byte, error)
}
