package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.Context(), t.TempDir(), NewClock())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// TestWriteReview_ByteIdentical proves config.md section 4.4: a stored
// review file holds the submitted bytes verbatim, and its filename is the
// full lowercase hex SHA-256 of those exact bytes in both trees.
func TestWriteReview_ByteIdentical(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := []byte(`{"ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f","verdict":"approve"}`)
	digest, path, err := s.WriteReview("github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", data)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	assert.Equal(t, hex.EncodeToString(sum[:]), digest)
	assert.Equal(t, digest+".json", filepath.Base(path))
	on, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, on, "stored bytes must be byte-identical to what was submitted")
}

// TestWriteRejected_ByteIdenticalEvenWhenNotJSON proves config.md section
// 4.4.1: the rejected tree keeps the input verbatim even when it is not
// valid JSON at all.
func TestWriteRejected_ByteIdenticalEvenWhenNotJSON(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := []byte("this is not json at all {{{")
	digest, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	require.NotEmpty(t, path)
	on, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, on)
	sum := sha256.Sum256(data)
	assert.Equal(t, hex.EncodeToString(sum[:]), digest)
	assert.Equal(t, digest+".json", filepath.Base(path))
}

// TestWriteRejected_OverCapWritesNoFile proves config.md section 4.4.1: an
// input over 1 MiB is not kept, though its digest is still computed so the
// run can still be recorded against it.
func TestWriteRejected_OverCapWritesNoFile(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := make([]byte, 2*1024*1024)
	for i := range data {
		data[i] = byte(i)
	}
	digest, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	assert.Empty(t, path, "an oversized input must not be written")
	assert.NotEmpty(t, digest, "the digest is still computed for the run row")
	entries, err := os.ReadDir(filepath.Join(s.Root(), "rejected"))
	if err == nil {
		assert.Empty(t, entries, "nothing should have been written under rejected/")
	} else {
		assert.True(t, os.IsNotExist(err), "rejected/ should not even have been created")
	}
}

// TestWriteRejected_ExactlyAtCapIsKept proves the 1 MiB cap is exclusive:
// "over 1 MiB is not kept" means exactly 1 MiB is.
func TestWriteRejected_ExactlyAtCapIsKept(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := make([]byte, maxRejectedSize)
	_, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

// TestWriteReview_DuplicateIsEEXISTAndIdempotent proves config.md section
// 4.4: storing a review that is already stored is an O_EXCL create that
// fails EEXIST, and that failure is treated as success rather than
// propagated — both calls succeed and the file holds exactly one copy.
func TestWriteReview_DuplicateIsEEXISTAndIdempotent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := []byte(`{"ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"}`)
	digest1, path1, err := s.WriteReview("github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", data)
	require.NoError(t, err)
	_, err = os.OpenFile(path1, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	require.True(t, errors.Is(err, os.ErrExist), "the file created by the first write must already exist, proven by O_EXCL now failing EEXIST")
	digest2, path2, err := s.WriteReview("github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", data)
	require.NoError(t, err, "storing the same bytes twice must not fail the caller")
	assert.Equal(t, digest1, digest2)
	assert.Equal(t, path1, path2)
	on, err := os.ReadFile(path1)
	require.NoError(t, err)
	assert.Equal(t, data, on)
}

func TestReviewPath_LayoutMatchesSpec(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	path := s.ReviewPath("github.com/bobcob7/loam-refinery", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", "3f9a")
	want := filepath.Join(s.Root(), "reviews", "github.com", "bobcob7", "loam-refinery", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", "3f9a.json")
	assert.Equal(t, want, path)
}

func TestRejectedPath_LayoutMatchesSpec(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	path := s.RejectedPath("github.com/bobcob7/loam-refinery", "44136fa3")
	want := filepath.Join(s.Root(), "rejected", "github.com", "bobcob7", "loam-refinery", "44136fa3.json")
	assert.Equal(t, want, path)
}

// TestWriteReview_InvalidRefNeverTouchesFilesystem proves config.md section
// 4.3: a ref that is not 40 hex characters never reaches the filesystem.
func TestWriteReview_InvalidRefNeverTouchesFilesystem(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	_, _, err := s.WriteReview("github.com/example/example", "not-a-ref", []byte("{}"))
	assert.Error(t, err)
	entries, statErr := os.ReadDir(filepath.Join(s.Root(), "reviews"))
	if statErr == nil {
		assert.Empty(t, entries)
	} else {
		assert.True(t, os.IsNotExist(statErr))
	}
}

// TestWriteReview_InvalidRepoNeverTouchesFilesystem proves the same
// guarantee for the repository name (config.md section 4.8).
func TestWriteReview_InvalidRepoNeverTouchesFilesystem(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	_, _, err := s.WriteReview("../../etc", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", []byte("{}"))
	assert.Error(t, err)
	entries, statErr := os.ReadDir(filepath.Join(s.Root(), "reviews"))
	if statErr == nil {
		assert.Empty(t, entries)
	} else {
		assert.True(t, os.IsNotExist(statErr))
	}
}

func TestWriteRejected_InvalidRepoNeverTouchesFilesystem(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	_, _, err := s.WriteRejected("../../etc", []byte("junk"))
	assert.Error(t, err)
	entries, statErr := os.ReadDir(filepath.Join(s.Root(), "rejected"))
	if statErr == nil {
		assert.Empty(t, entries)
	} else {
		assert.True(t, os.IsNotExist(statErr))
	}
}

func TestCreateExclusive_CreatesDirectoriesMode0700(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c.json")
	require.NoError(t, createExclusive(path, []byte("x")))
	info, err := os.Stat(filepath.Join(dir, "a"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}
