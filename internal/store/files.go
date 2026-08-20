// Storing and locating review files (config.md sections 4.4, 4.4.1, 4.6).
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// maxRejectedSize is the cap on a kept rejected input (config.md section
// 4.4.1). An input over this is not written; its run is still recorded.
const maxRejectedSize = 1 << 20

// sha256Hex returns the lowercase hex SHA-256 of data — the filename a
// stored file is addressed by (config.md section 4.4).
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ReviewPath returns the path a passing review of ref in repo, with the
// given digest, occupies under the store — computed, never stored
// (config.md section 4.5.1).
func (s *Store) ReviewPath(repo, ref, digest string) string {
	return filepath.Join(s.root, "reviews", filepath.FromSlash(repo), ref, digest+".json")
}

// RejectedPath returns the path a rejected input in repo, with the given
// digest, occupies under the store — computed, never stored.
func (s *Store) RejectedPath(repo, digest string) string {
	return filepath.Join(s.root, "rejected", filepath.FromSlash(repo), digest+".json")
}

// WriteReview writes data — the submitted bytes, verbatim — to
// reviews/<repo>/<ref>/<digest>.json, creating the directories above it at
// mode 0700. It returns the digest data was addressed by and the path it
// now occupies, whether this call created the file or found it already
// there: storing a review that is already stored is a no-op, no directory
// scan, and content addressing is what makes that safe — the bytes already
// at that path are these bytes, by construction (config.md section 4.4).
// repo and ref are validated before either reaches the filesystem
// (config.md sections 4.3, 4.8): a value that does not fit never becomes a
// path component.
func (s *Store) WriteReview(repo, ref string, data []byte) (digest, path string, err error) {
	if err := ValidateName(repo); err != nil {
		return "", "", fmt.Errorf("invalid repository name: %w", err)
	}
	if err := ValidateRef(ref); err != nil {
		return "", "", fmt.Errorf("invalid ref: %w", err)
	}
	digest = sha256Hex(data)
	path = s.ReviewPath(repo, ref, digest)
	if err := createAtomic(path, data); err != nil {
		return digest, "", err
	}
	return digest, path, nil
}

// WriteRejected writes data to rejected/<repo>/<digest>.json, unless it is
// over the 1 MiB cap, in which case nothing is written (config.md section
// 4.4.1). The digest is always returned — the run's row records it either
// way — and path is empty exactly when the file was not kept, which is the
// signal a caller uses to omit it from output. repo is validated before it
// reaches the filesystem (config.md section 4.8).
func (s *Store) WriteRejected(repo string, data []byte) (digest, path string, err error) {
	if err := ValidateName(repo); err != nil {
		return "", "", fmt.Errorf("invalid repository name: %w", err)
	}
	digest = sha256Hex(data)
	if len(data) > maxRejectedSize {
		return digest, "", nil
	}
	path = s.RejectedPath(repo, digest)
	if err := createAtomic(path, data); err != nil {
		return digest, "", err
	}
	return digest, path, nil
}

// createAtomic writes data to path by writing a temporary file in path's
// directory and renaming it into place, after creating that directory at
// mode 0700. A file already at path is left untouched and reported as
// success without writing anything — content addressing guarantees the
// bytes there already are these bytes (config.md section 4.4) — so the
// common case, storing a review that is already stored, never allocates a
// temp file at all.
//
// The rename is what makes a killed run or a second writer arriving
// mid-write harmless: os.Rename onto an existing name is atomic on every
// filesystem this tool runs on, so a reader of path never observes
// anything but a complete file or no file — never a file whose write was
// interrupted partway. An O_EXCL create followed by a separate write
// cannot make that promise: a crash or a second writer between the two
// steps leaves a zero-byte file at path that content addressing can never
// reclaim, because "reclaimed by the next store of the same bytes" is
// exactly the write this function skips once path exists.
func createAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}
	return nil
}
