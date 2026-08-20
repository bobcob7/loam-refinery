package store

import (
	"context"
	"errors"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateName_Valid(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"no-repo",
		"github.com",
		"github.com/bobcob7/loam-refinery",
		"local/scratch",
		"a",
		"a.b_c-d",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, ValidateName(name))
		})
	}
}

func TestValidateName_Invalid(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"empty":                "",
		"traversal segment":    "github.com/../etc",
		"dot segment":          "github.com/./x",
		"five segments":        "a/b/c/d/e",
		"uppercase":            "GitHub.com/x",
		"leading dash":         "-github.com",
		"leading dot":          ".github",
		"disallowed character": "github.com/x y",
		"over 200 chars total": "a/" + repeat("b", 199),
		"segment over 64":      repeat("a", 65),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, ValidateName(value))
		})
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

func TestValidateRef_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ValidateRef("4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f"))
}

func TestValidateRef_Invalid(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"too short":  "4f2c1a9",
		"uppercase":  "4F2C1A9E3B7D5F0C8A1E2D4B6C8F0A2E4D6B8C0F",
		"non-hex":    "g" + repeat("0", 39),
		"empty":      "",
		"41 chars":   repeat("0", 41),
		"whitespace": " " + repeat("0", 39),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, ValidateRef(value))
		})
	}
}

// TestNormalizeSegment_LowercasesBeforeReplacing proves the order config.md
// section 4.8 requires: lowercasing first is what keeps a mixed-case
// segment from having its uppercase letters treated as unsafe characters
// and replaced with '-'. Replacing before lowercasing would turn "My" into
// "-y" (M falls outside the case-sensitive [a-z0-9._-] class, y does not);
// lowercasing first keeps both letters intact.
func TestNormalizeSegment_LowercasesBeforeReplacing(t *testing.T) {
	t.Parallel()
	got := normalizeSegment("My_Repo")
	assert.NotEqual(t, "-y-repo", got, "wrong order would drop the leading M and R as unsafe characters")
	assert.Equal(t, "my_repo", got, "underscore is inside [a-z0-9._-] and survives replacement; only case changes")
}

// TestNormalizeSegment_UnsafeCharactersBecomeDashes exercises the same
// ordering guarantee with a character that is unambiguously outside
// [a-z0-9._-] regardless of case, so the result is unambiguous too.
func TestNormalizeSegment_UnsafeCharactersBecomeDashes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "my-repo", normalizeSegment("My Repo"))
}

func TestNormalizeSegment_CollapsesRunsAndTrimsEdges(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a-b", normalizeSegment("a---b"))
	assert.Equal(t, "ab", normalizeSegment("-ab-"))
	assert.Equal(t, "ab", normalizeSegment(".ab."))
}

func TestNormalizeSegment_TruncatesTo64(t *testing.T) {
	t.Parallel()
	got := normalizeSegment(repeat("a", 100))
	assert.Len(t, got, 64)
}

func TestNormalizeSegment_EmptyResultSignalsFallback(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", normalizeSegment("---"))
	assert.Equal(t, "", normalizeSegment("..."))
}

func TestRepoName_NoRepositoryFallsBackToNoRepo(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{
		rootFunc: func(ctx context.Context, dir string) (string, error) {
			return "", verify.ErrNoRepository
		},
	}
	name, err := RepoName(t.Context(), git, "/tmp/scratch", nil)
	require.NoError(t, err)
	assert.Equal(t, "no-repo", name)
}

func TestRepoName_OverrideWinsOverEverything(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{
		rootFunc: func(ctx context.Context, dir string) (string, error) {
			return "/repo/root", nil
		},
		originURLFunc: func(ctx context.Context, root string) (string, error) {
			return "https://github.com/example/example", nil
		},
	}
	overrides := map[string]string{"/repo/root": "upstream/example"}
	name, err := RepoName(t.Context(), git, "/repo/root/sub", overrides)
	require.NoError(t, err)
	assert.Equal(t, "upstream/example", name)
}

func TestRepoName_OverrideKeyedByWorkingDirWhenNoRepository(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{
		rootFunc: func(ctx context.Context, dir string) (string, error) {
			return "", verify.ErrNoRepository
		},
	}
	overrides := map[string]string{"/scratch": "team/scratch"}
	name, err := RepoName(t.Context(), git, "/scratch", overrides)
	require.NoError(t, err)
	assert.Equal(t, "team/scratch", name)
}

func TestRepoName_NormalizedOriginRemote(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"git@github.com:bobcob7/loam-refinery.git": "github.com/bobcob7/loam-refinery",
		"https://github.com/bobcob7/loam-refinery": "github.com/bobcob7/loam-refinery",
		"ssh://git@example.com:2222/team/svc.git":  "example.com/team/svc",
		"https://EXAMPLE.com/Team/Svc":             "example.com/team/svc",
	}
	for origin, want := range cases {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			git := &gitRunnerMock{
				rootFunc: func(ctx context.Context, dir string) (string, error) {
					return "/repo/root", nil
				},
				originURLFunc: func(ctx context.Context, root string) (string, error) {
					return origin, nil
				},
			}
			name, err := RepoName(t.Context(), git, "/repo/root", nil)
			require.NoError(t, err)
			assert.Equal(t, want, name)
		})
	}
}

func TestRepoName_NoOriginFallsBackToLocalBasename(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{
		rootFunc: func(ctx context.Context, dir string) (string, error) {
			return "/home/me/scratch", nil
		},
		originURLFunc: func(ctx context.Context, root string) (string, error) {
			return "", nil
		},
	}
	name, err := RepoName(t.Context(), git, "/home/me/scratch", nil)
	require.NoError(t, err)
	assert.Equal(t, "local/scratch", name)
}

// TestRepoName_RemoteNormalizingToNoRepoFallsBack proves config.md section
// 4.2's rule that a derived name may never equal the reserved no-repo: an
// origin with no path and a host that itself normalizes to "no-repo"
// derives exactly that single reserved segment, which must be rejected in
// favor of the next candidate rather than accepted.
func TestRepoName_RemoteNormalizingToNoRepoFallsBack(t *testing.T) {
	t.Parallel()
	git := &gitRunnerMock{
		rootFunc: func(ctx context.Context, dir string) (string, error) {
			return "/home/me/scratch", nil
		},
		originURLFunc: func(ctx context.Context, root string) (string, error) {
			return "https://no-repo", nil
		},
	}
	name, err := RepoName(t.Context(), git, "/home/me/scratch", nil)
	require.NoError(t, err)
	assert.Equal(t, "local/scratch", name, "the colliding remote-derived name is rejected and falls back to local/<basename>")
}

func TestRepoName_GitFailurePropagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	git := &gitRunnerMock{
		rootFunc: func(ctx context.Context, dir string) (string, error) {
			return "", boom
		},
	}
	_, err := RepoName(t.Context(), git, "/tmp", nil)
	assert.ErrorIs(t, err, boom)
}
