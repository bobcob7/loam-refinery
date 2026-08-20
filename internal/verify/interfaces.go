package verify

import "context"

//go:generate moq -out moq_test.go . gitRunner

// gitRunner runs one git command against the discovered repository, and
// answers whether an anchored file's working-tree copy has diverged from a
// ref. Object lookups aside, worktreeDiverged is the only place this package
// touches file contents, and it stays confined to hashing: it never reads
// what changed, only whether something did.
type gitRunner interface {
	run(ctx context.Context, args ...string) ([]byte, error)
	worktreeDiverged(ctx context.Context, ref, path string) (bool, error)
}
