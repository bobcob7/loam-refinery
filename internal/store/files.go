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
// there: storing a review that is already stored is an O_EXCL create that
// fails EEXIST, no directory scan, and this call treats that as success
// rather than failure (config.md section 4.4). repo and ref are validated
// before either reaches the filesystem (config.md sections 4.3, 4.8): a
// value that does not fit never becomes a path component.
func (s *Store) WriteReview(repo, ref string, data []byte) (digest, path string, err error) {
	if err := ValidateName(repo); err != nil {
		return "", "", fmt.Errorf("invalid repository name: %w", err)
	}
	if err := ValidateRef(ref); err != nil {
		return "", "", fmt.Errorf("invalid ref: %w", err)
	}
	digest = sha256Hex(data)
	path = s.ReviewPath(repo, ref, digest)
	if err := createExclusive(path, data); err != nil {
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
	if err := createExclusive(path, data); err != nil {
		return digest, "", err
	}
	return digest, path, nil
}

// createExclusive writes data to path with O_EXCL after creating path's
// parent directories at mode 0700. A file already there is left untouched
// and reported as success: content addressing means the bytes it holds are
// these bytes, by construction (config.md section 4.4).
func createExclusive(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
