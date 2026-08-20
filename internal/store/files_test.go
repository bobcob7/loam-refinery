package store

import (
	"crypto/sha256"
	"encoding/hex"
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

// TestWriteRejected_OverCapTruncatesToOneMiB proves config.md section
// 4.4.1: an input over 1 MiB is stored truncated to exactly its first 1
// MiB, not dropped, and the digest returned is the SHA-256 of the FULL
// input rather than the truncated bytes — the name still identifies what
// was actually submitted. The payload size is the literal 2*1024*1024
// rather than a multiple of maxRejectedSize (refinery-a96.35): a payload
// derived from the constant shrinks along with it, so a cap quietly changed
// from 1 MiB to 1 KiB would still pass this test at whatever the new cap
// is.
func TestWriteRejected_OverCapTruncatesToOneMiB(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := make([]byte, 2*1024*1024)
	for i := range data {
		data[i] = byte(i)
	}
	digest, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	require.NotEmpty(t, path, "an oversized input is still written, truncated")
	fullSum := sha256.Sum256(data)
	assert.Equal(t, hex.EncodeToString(fullSum[:]), digest, "the digest covers the full submitted input, not the truncated bytes")
	on, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Len(t, on, 1<<20, "the stored file is truncated to exactly 1 MiB")
	assert.Equal(t, data[:1<<20], on, "the stored bytes are the input's first 1 MiB")
}

// TestWriteRejected_TruncatedFileDoesNotHashToItsOwnName proves the
// truncation signal config.md section 4.4.1 documents: because the digest
// names the full input but a truncated file holds only its first 1 MiB,
// hashing that stored file never reproduces the filename that addresses
// it. That is deliberate — no `stored` column, no truncated flag — hashing
// a rejected file and comparing it to its own name is enough to tell a
// truncated file from a whole one.
func TestWriteRejected_TruncatedFileDoesNotHashToItsOwnName(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := make([]byte, 2*1024*1024)
	for i := range data {
		data[i] = byte(i)
	}
	_, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	on, err := os.ReadFile(path)
	require.NoError(t, err)
	onSum := sha256.Sum256(on)
	assert.NotEqual(t, filepath.Base(path), hex.EncodeToString(onSum[:])+".json",
		"a truncated file's own hash must not match the filename it lives under")
}

// TestWriteRejected_ExactlyAtCapIsKeptWhole proves the 1 MiB cap is
// exclusive: "over 1 MiB is truncated" means exactly 1 MiB is kept whole.
// An at-cap file is also self-verifying — its own hash matches its name —
// the opposite of TestWriteRejected_TruncatedFileDoesNotHashToItsOwnName.
// The payload size is the literal 1 << 20 rather than maxRejectedSize
// itself (refinery-a96.35): a payload derived from the constant shrinks
// along with it, so a cap quietly changed from 1 MiB to 1 KiB would still
// pass this test at whatever the new cap is — a literal is what makes a
// shrunk cap reject a payload this test still expects to be kept whole.
func TestWriteRejected_ExactlyAtCapIsKeptWhole(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i)
	}
	digest, path, err := s.WriteRejected("github.com/example/example", data)
	require.NoError(t, err)
	require.NotEmpty(t, path)
	on, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, on, "an at-cap input is stored whole, not truncated")
	onSum := sha256.Sum256(on)
	assert.Equal(t, digest, hex.EncodeToString(onSum[:]), "an at-cap file's own hash matches its filename")
}

// TestWriteReview_DuplicateIsIdempotent proves config.md section 4.4:
// storing a review that is already stored succeeds without error, and the
// file holds exactly one copy afterward.
func TestWriteReview_DuplicateIsIdempotent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := []byte(`{"ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"}`)
	digest1, path1, err := s.WriteReview("github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", data)
	require.NoError(t, err)
	digest2, path2, err := s.WriteReview("github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", data)
	require.NoError(t, err, "storing the same bytes twice must not fail the caller")
	assert.Equal(t, digest1, digest2)
	assert.Equal(t, path1, path2)
	on, err := os.ReadFile(path1)
	require.NoError(t, err)
	assert.Equal(t, data, on)
}

// TestWriteReview_DuplicateLeavesExistingFileUntouched proves the stronger
// claim createAtomic's doc comment makes and TestWriteReview_
// DuplicateIsIdempotent cannot: a second store of the same bytes leaves the
// file already at that path alone rather than rewriting it. Content
// addressing means the two are indistinguishable in ordinary operation —
// the bytes a rewrite would produce are identical to the bytes already
// there — so this plants a sentinel that is NOT what WriteReview would
// produce at that exact path, to make a rewrite (correct bytes) and a skip
// (sentinel survives) tell different stories. A createAtomic that dropped
// its "already there" check — the equivalent of O_EXCL silently becoming
// O_TRUNC — would overwrite the sentinel with data and fail this test; it
// passes TestWriteReview_DuplicateIsIdempotent either way, because that
// test can only see the bytes it wrote itself.
func TestWriteReview_DuplicateLeavesExistingFileUntouched(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	data := []byte(`{"ref":"4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"}`)
	digest, path, err := s.WriteReview("github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", data)
	require.NoError(t, err)
	sentinel := []byte("not what WriteReview would write here")
	require.NoError(t, os.WriteFile(path, sentinel, 0o600))
	digest2, path2, err := s.WriteReview("github.com/example/example", "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f", data)
	require.NoError(t, err)
	assert.Equal(t, digest, digest2)
	assert.Equal(t, path, path2)
	on, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, sentinel, on, "a file already at path must be left alone, not rewritten with the same bytes")
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

func TestCreateAtomic_CreatesDirectoriesMode0700(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c.json")
	require.NoError(t, createAtomic(path, []byte("x")))
	info, err := os.Stat(filepath.Join(dir, "a"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

// TestCreateAtomic_NoTempFileLeftBehind proves createAtomic's rename leaves
// nothing but the target: no ".tmp-*" survives a successful write, which
// would otherwise litter every tree this function writes into.
func TestCreateAtomic_NoTempFileLeftBehind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	require.NoError(t, createAtomic(path, []byte("x")))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "c.json", entries[0].Name())
}
