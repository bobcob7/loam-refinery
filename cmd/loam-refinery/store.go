package main

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"

	"github.com/bobcob7/loam-refinery/internal/cli"
	"github.com/bobcob7/loam-refinery/internal/config"
	"github.com/bobcob7/loam-refinery/internal/store"
)

// storeAdapter is the concrete implementation of internal/cli's documentStore
// interface (docs/config.md §5): it wires internal/config and internal/store
// together, lazily, on the one path that ever needs either — validate.
// prime, describe, schema, and version never construct one.
type storeAdapter struct {
	git *store.Git
}

// newStoreAdapter returns a documentStore that resolves and opens the store
// fresh on every call, per docs/config.md §2.2: nothing about a store is
// read or created until a validate run has something to keep.
func newStoreAdapter(log *slog.Logger) *storeAdapter {
	return &storeAdapter{git: store.NewGit(log)}
}

// Save implements internal/cli's documentStore. It loads the config file,
// checks store.repos for shape before anything touches a filesystem
// (docs/config.md §4.8), and — only when storing is enabled and this run has
// something to keep — materializes the config directory, opens or creates
// the store, writes the review or the rejected input, and records the row.
func (a *storeAdapter) Save(ctx context.Context, in cli.StoreInput) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := validateRepoOverrides(cfg.Store.Repos); err != nil {
		return err
	}
	if !cfg.Store.Enabled {
		return nil
	}
	if err := config.Materialize(cfg.ConfigPath); err != nil {
		return err
	}
	st, err := store.New(ctx, cfg.Store.Path, store.NewClock())
	if err != nil {
		return err
	}
	defer st.Close()
	repo, err := store.RepoName(ctx, a.git, in.Dir, cfg.Store.Repos)
	if err != nil {
		return err
	}
	exitCode, digest, err := a.write(st, repo, in)
	if err != nil {
		return err
	}
	return st.Record(ctx, store.RunInput{
		Repo:          repo,
		Ref:           validRef(in.Ref),
		Digest:        digest,
		ExitCode:      exitCode,
		Verdict:       validVerdict(in.Verdict),
		Assessment:    validAssessment(in.Assessment),
		NumComments:   &in.Comments,
		NumErrors:     &in.Errors,
		NumAdvisories: &in.Advisories,
		NumSkipped:    &in.Skipped,
		ToolVersion:   in.ToolVersion,
		SchemaVersion: in.SchemaVersion,
	})
}

// write places in.Source in the tree its exit code names — reviews/ for a
// passing run, rejected/ for a failing one — and returns the exit code
// alongside the digest the row records (docs/config.md §5, §4.4, §4.4.1).
// The precondition (docs/cli.md §2.3.1) fires before a document is
// examined, so exit 3 places nothing in either tree: it is not a rejected
// input, because rejected/ holds documents that were examined and did not
// hold up, and this one never reached that. store.Digest supplies the row's
// digest without writing a file for it — runs.digest is TEXT NOT NULL
// (docs/config.md §4.5.1).
func (a *storeAdapter) write(st *store.Store, repo string, in cli.StoreInput) (exitCode int, digest string, err error) {
	switch {
	case in.Precondition:
		return cli.ExitPrecondition, store.Digest(in.Source), nil
	case in.Valid:
		digest, _, err = st.WriteReview(repo, in.Ref, in.Source)
		return cli.ExitValid, digest, err
	default:
		digest, _, err = st.WriteRejected(repo, in.Source)
		return cli.ExitInvalid, digest, err
	}
}

// validRef reports ref only when it fits docs/config.md §4.3's shape, so a
// document whose ref field is present but malformed never reaches the
// database as if it were a real commit. A passing run's ref always fits —
// the structural ref-format check gates exit 0 — so this only ever discards
// something on a rejected run.
func validRef(ref string) string {
	if store.ValidateRef(ref) != nil {
		return ""
	}
	return ref
}

// validVerdict reports verdict only when it is one of store.Verdicts() —
// the runs table's CHECK constraint (docs/config.md §4.5.2) and
// review.schema.json's verdict enum both name the same three words, and
// store.Verdicts() is the single place this package spells them out rather
// than retyping them here too. Anything else — absent, ill-typed, or
// simply not one of those words — is left out rather than sent to a
// column that would reject it.
func validVerdict(verdict string) string {
	if slices.Contains(store.Verdicts(), verdict) {
		return verdict
	}
	return ""
}

// validAssessment reports assessment only when it is one of
// store.Assessments() — the runs table's CHECK constraint (docs/config.md
// §4.5.2) and review.schema.json's assessment enum both name the same four
// words, and store.Assessments() is the single place this package spells
// them out rather than retyping them here too. Anything else — absent,
// ill-typed, or simply not one of those words — is left out rather than
// sent to a column that would reject it, mirroring validVerdict exactly:
// assessment is optional, so an absent one is the ordinary case, not an
// error.
func validAssessment(assessment string) string {
	if slices.Contains(store.Assessments(), assessment) {
		return assessment
	}
	return ""
}

// validateRepoOverrides checks every store.repos value against
// docs/config.md §4.8 before anything about the store touches a filesystem.
// A name a person typed is never normalized — a bad one is a config error,
// exit 101, naming the offending entry.
func validateRepoOverrides(repos map[string]string) error {
	paths := make([]string, 0, len(repos))
	for path := range repos {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := store.ValidateName(repos[path]); err != nil {
			return fmt.Errorf("store.repos[%q]: %w", path, err)
		}
	}
	return nil
}
