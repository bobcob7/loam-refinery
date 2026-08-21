package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProfile writes one profile file under dir, failing the test on error.
func writeProfile(t *testing.T, dir, filename, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600))
}

// validFrontMatter builds a minimal, well-formed frontmatter block with the
// given description, ready to have a body appended after it.
func validFrontMatter(description string) string {
	return "---\ndescription: " + description + "\n---\n"
}

func TestReader_Load_Valid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProfile(t, dir, "backend.md", "---\ndescription: Go services; concurrency, error wrapping, context handling\n---\n\nAnchor concurrency findings at the goroutine that leaks.\n")
	r := New(dir)
	p, ok, err := r.Load("backend")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "backend", p.Name)
	assert.Equal(t, "Go services; concurrency, error wrapping, context handling", p.Description)
	assert.Equal(t, "Anchor concurrency findings at the goroutine that leaks.", p.Body)
}

func TestReader_Load_Outcomes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProfile(t, dir, "good.md", validFrontMatter("A valid profile")+"Body text.\n")
	writeProfile(t, dir, "broken.md", "---\ndescription: ok\n---\n")
	r := New(dir)
	t.Run("valid name and file", func(t *testing.T) {
		t.Parallel()
		p, ok, err := r.Load("good")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "good", p.Name)
	})
	t.Run("name matches no file", func(t *testing.T) {
		t.Parallel()
		p, ok, err := r.Load("missing")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Zero(t, p)
	})
	t.Run("name is not a valid name", func(t *testing.T) {
		t.Parallel()
		p, ok, err := r.Load("UPPERCASE")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Zero(t, p)
	})
	t.Run("file exists but is malformed", func(t *testing.T) {
		t.Parallel()
		p, ok, err := r.Load("broken")
		require.Error(t, err)
		assert.False(t, ok)
		assert.Zero(t, p)
	})
}

func TestReader_Load_PathTraversalRejectedAsBadName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	secretPath := filepath.Join(parent, "secret.md")
	require.NoError(t, os.WriteFile(secretPath, []byte(validFrontMatter("leaked secret")+"leaked body\n"), 0o600))
	t.Cleanup(func() { os.Remove(secretPath) })
	r := New(dir)
	cases := []struct {
		label string
		name  string
	}{
		{"parent traversal", "../secret"},
		{"embedded separator", "a/b"},
		{"absolute path", "/etc/passwd"},
		{"leading dot", ".hidden"},
		{"uppercase", "UPPER"},
		{"underscore", "has_underscore"},
		{"empty", ""},
		{"space", "has space"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()
			p, ok, err := r.Load(tc.name)
			require.NoError(t, err)
			assert.False(t, ok)
			assert.Zero(t, p)
		})
	}
}

// TestReader_Load_IdentityNotCaseFold pins refinery-emv.7's acceptance
// criterion directly: on a case-insensitive filesystem, resolving
// "readme.md" by name would let the kernel fold it onto README.md even
// though "README" is not a valid profile name. Load must refuse it rather
// than opening the file the identity check hides.
func TestReader_Load_IdentityNotCaseFold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProfile(t, dir, "README.md", "# Not a profile\n")
	r := New(dir)
	p, ok, err := r.Load("readme")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Zero(t, p)
}

func TestReader_Load_FrontMatterErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		label           string
		content         string
		wantErrContains string
	}{
		{"missing frontmatter", "no frontmatter here\n", "missing frontmatter"},
		{"unterminated frontmatter", "---\ndescription: ok\nbody without closing fence\n", "unterminated frontmatter"},
		// The file's only key is unknown - no description line comes
		// first to trip the duplicate-key branch instead, which is what
		// let a mutant that deletes the unknown-key check stay green
		// (refinery-emv.10a): under that mutant "author" is read as the
		// description value and this file parses successfully.
		{"unknown key only", "---\nauthor: someone\n---\nbody\n", `unknown frontmatter key "author"`},
		{"unknown key after description", "---\ndescription: ok\nauthor: someone\n---\nbody\n", `unknown frontmatter key "author"`},
		{"duplicate description key", "---\ndescription: one\ndescription: two\n---\nbody\n", `duplicate frontmatter key "description"`},
		{"missing description key", "---\n---\nbody\n", `missing required frontmatter key "description"`},
		{"empty description value", "---\ndescription: \n---\nbody\n", "description must not be empty"},
		{"malformed line with no colon", "---\ndescription value\n---\nbody\n", "malformed frontmatter line"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeProfile(t, dir, "profile.md", tc.content)
			r := New(dir)
			p, ok, err := r.Load("profile")
			require.Error(t, err)
			assert.False(t, ok)
			assert.Zero(t, p)
			assert.ErrorContains(t, err, tc.wantErrContains)
		})
	}
}

func TestReader_Load_DescriptionRuneLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := New(dir)
	desc120 := strings.Repeat("a", 120)
	desc121 := strings.Repeat("a", 121)
	multiByte120 := strings.Repeat("é", 120)
	writeProfile(t, dir, "at-limit.md", validFrontMatter(desc120)+"Body.\n")
	writeProfile(t, dir, "over-limit.md", validFrontMatter(desc121)+"Body.\n")
	writeProfile(t, dir, "multibyte.md", validFrontMatter(multiByte120)+"Body.\n")
	t.Run("120 runes passes", func(t *testing.T) {
		t.Parallel()
		p, ok, err := r.Load("at-limit")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, desc120, p.Description)
	})
	t.Run("121 runes fails", func(t *testing.T) {
		t.Parallel()
		_, ok, err := r.Load("over-limit")
		require.Error(t, err)
		assert.False(t, ok)
	})
	t.Run("120 multi-byte runes passes despite being well over 120 bytes", func(t *testing.T) {
		t.Parallel()
		p, ok, err := r.Load("multibyte")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, multiByte120, p.Description)
		assert.Equal(t, 120, utf8.RuneCountInString(p.Description))
	})
}

func TestReader_Load_EmptyBody(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := New(dir)
	writeProfile(t, dir, "empty.md", "---\ndescription: ok\n---\n")
	writeProfile(t, dir, "whitespace.md", "---\ndescription: ok\n---\n   \n\t\n")
	t.Run("no body at all", func(t *testing.T) {
		t.Parallel()
		_, ok, err := r.Load("empty")
		require.Error(t, err)
		assert.False(t, ok)
	})
	t.Run("whitespace-only body", func(t *testing.T) {
		t.Parallel()
		_, ok, err := r.Load("whitespace")
		require.Error(t, err)
		assert.False(t, ok)
	})
}

func TestReader_Load_BodyTrimming(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := New(dir)
	content := "---\ndescription: ok\n---\n\nFirst line.\n\n  Indented second line.\n\n\n"
	writeProfile(t, dir, "trim.md", content)
	p, ok, err := r.Load("trim")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "First line.\n\n  Indented second line.", p.Body)
}

// TestReader_Load_CRLFParsesSameAsLF is refinery-emv.8's first acceptance
// criterion: a profile hand-edited on a core.autocrlf=true checkout must
// parse identically to its LF twin, not fail on lines[0] == "---\r".
func TestReader_Load_CRLFParsesSameAsLF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lf := "---\ndescription: CRLF twin\n---\n\nBody line one.\n\nBody line two.\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	writeProfile(t, dir, "lf.md", lf)
	writeProfile(t, dir, "crlf.md", crlf)
	r := New(dir)
	lfProfile, ok, err := r.Load("lf")
	require.NoError(t, err)
	require.True(t, ok)
	crlfProfile, ok, err := r.Load("crlf")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, lfProfile.Description, crlfProfile.Description)
	assert.Equal(t, lfProfile.Body, crlfProfile.Body)
}

// TestReader_Load_WhitespaceOnlyFrontMatterLineIsBlank is refinery-emv.8's
// second acceptance criterion: a whitespace-only frontmatter line - CRLF's
// empty line becomes exactly this once the "\r" survives a naive split -
// must be treated as blank rather than as a malformed key: value line.
func TestReader_Load_WhitespaceOnlyFrontMatterLineIsBlank(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProfile(t, dir, "profile.md", "---\ndescription: ok\n   \n\t\n---\nBody.\n")
	r := New(dir)
	p, ok, err := r.Load("profile")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "ok", p.Description)
}
