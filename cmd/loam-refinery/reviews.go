package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/bobcob7/loam-refinery/internal/config"
	"github.com/bobcob7/loam-refinery/internal/store"
	"github.com/bobcob7/loam-refinery/internal/verify"
)

// reviewsAdapter is the concrete implementation of internal/cli's
// reviewStore interface (docs/config.md §6). Every method resolves the
// store from config the way storeAdapter does, but never creates what a
// read does not need: a store that has not been written to yet is answered
// as empty rather than materialized (docs/config.md §2.2).
type reviewsAdapter struct {
	git *store.Git
	log *slog.Logger
}

// newReviewsAdapter returns a reviewStore that resolves and opens the store
// fresh on every call, mirroring newStoreAdapter.
func newReviewsAdapter(log *slog.Logger) *reviewsAdapter {
	return &reviewsAdapter{git: store.NewGit(log), log: log}
}

// RepoName implements internal/cli's reviewStore: it walks up from dir the
// way verification finds a repository, and ok is false when dir is not
// inside one, where reviews has no default to offer (docs/config.md §6).
func (a *reviewsAdapter) RepoName(ctx context.Context, dir string) (string, bool, error) {
	if _, err := verify.Discover(ctx, dir); err != nil {
		if errors.Is(err, verify.ErrNoRepository) {
			return "", false, nil
		}
		return "", false, err
	}
	cfg, err := loadValidConfig()
	if err != nil {
		return "", false, err
	}
	name, err := store.RepoName(ctx, a.git, dir, cfg.Store.Repos)
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}

// Known implements internal/cli's reviewStore.
func (a *reviewsAdapter) Known(ctx context.Context, repo string) (bool, error) {
	st, ok, err := a.open(ctx)
	if err != nil || !ok {
		return false, err
	}
	defer st.Close()
	return st.Known(ctx, repo)
}

// ListReviews implements internal/cli's reviewStore.
func (a *reviewsAdapter) ListReviews(ctx context.Context, repo, ref string, limit int) ([]store.Review, int, error) {
	st, ok, err := a.open(ctx)
	if err != nil || !ok {
		return nil, 0, err
	}
	defer st.Close()
	return st.ListReviews(ctx, repo, ref, limit)
}

// ListFailedRuns implements internal/cli's reviewStore.
func (a *reviewsAdapter) ListFailedRuns(ctx context.Context, repo, ref string, limit int) ([]store.FailedRun, int, error) {
	st, ok, err := a.open(ctx)
	if err != nil || !ok {
		return nil, 0, err
	}
	defer st.Close()
	return st.ListFailedRuns(ctx, repo, ref, limit)
}

// ListRepos implements internal/cli's reviewStore.
func (a *reviewsAdapter) ListRepos(ctx context.Context) ([]store.RepoCount, error) {
	st, ok, err := a.open(ctx)
	if err != nil || !ok {
		return nil, err
	}
	defer st.Close()
	return st.ListRepos(ctx)
}

// ReadContent implements internal/cli's reviewStore. It is the only method
// here that ever opens a tree rather than store.db (docs/config.md §6.3).
func (a *reviewsAdapter) ReadContent(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// DistinctDigests implements internal/cli's reviewStore
// (docs/features/combined-reviews.md §5.3.1), the enumeration
// collect-reviews reads.
func (a *reviewsAdapter) DistinctDigests(ctx context.Context, repo, ref string) ([]store.DigestRow, error) {
	st, ok, err := a.open(ctx)
	if err != nil || !ok {
		return nil, err
	}
	defer st.Close()
	return st.DistinctDigests(ctx, repo, ref)
}

// ReviewPath implements internal/cli's reviewStore: the digest-to-path half
// of collect-reviews's own reader (refinery-uyb.11's notes: "digest to path
// is not on the reviewStore interface" until now). It calls
// store.ReviewPathAt rather than opening a *Store just to call
// Store.ReviewPath — a Store's root is always cfg.Store.Path, the same
// value store.New and store.NewReadOnly are constructed with, so nothing
// beyond the config file is needed to compute it. Both this method and
// Store.ReviewPath go through ReviewPathAt, so there is exactly one
// formula to keep in sync with itself.
func (a *reviewsAdapter) ReviewPath(ctx context.Context, repo, ref, digest string) (string, error) {
	cfg, err := loadValidConfig()
	if err != nil {
		return "", err
	}
	return store.ReviewPathAt(cfg.Store.Path, repo, ref, digest), nil
}

// StoreEnabled implements internal/cli's reviewStore: collect-reviews's own
// store.enabled field (docs/features/combined-reviews.md §8.1), read from
// the same config storeAdapter.Save already reads it from. store.enabled
// never gates a read (§9), so this is reported, never consulted, by every
// other method here.
func (a *reviewsAdapter) StoreEnabled(ctx context.Context) (bool, error) {
	cfg, err := loadValidConfig()
	if err != nil {
		return false, err
	}
	return cfg.Store.Enabled, nil
}

// open resolves the store directory from config and opens it read-only for
// one call. A store.db that does not exist yet is reported as absent
// (ok=false, err=nil) rather than created — store.New creates root and
// store.db when either is missing, which a read must never trigger on a
// machine that has no store at all (docs/config.md §2.2, §6.2). It opens
// through store.NewReadOnly rather than store.New: a read pays for nothing
// a writer needs — no MkdirAll, no WAL conversion, no migration, no clock.
func (a *reviewsAdapter) open(ctx context.Context) (*store.Store, bool, error) {
	cfg, err := loadValidConfig()
	if err != nil {
		return nil, false, err
	}
	exists, err := store.Exists(cfg.Store.Path)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	st, err := store.NewReadOnly(ctx, cfg.Store.Path)
	if err != nil {
		return nil, false, err
	}
	return st, true, nil
}

// loadValidConfig loads the config file and checks store.repos for shape
// before anything touches a filesystem (docs/config.md §4.8), the same
// check storeAdapter.Save makes before writing — a bad override entry must
// surface on a read the same way it does on a write.
func loadValidConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := validateRepoOverrides(cfg.Store.Repos); err != nil {
		return nil, err
	}
	return cfg, nil
}
