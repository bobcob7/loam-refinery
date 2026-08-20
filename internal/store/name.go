// Path safety and repository naming (config.md sections 4.2 and 4.8).
package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bobcob7/loam-refinery/internal/verify"
)

// noRepo is the reserved name for a document with no repository at all, and
// the name every derivation chain falls back to when it cannot produce
// something that fits (config.md section 4.2).
const noRepo = "no-repo"

// segmentPattern is the shape config.md section 4.8 requires of every path
// segment. Its leading-character anchor is also why "." and ".." can never
// survive normalization: both start with a character outside [a-z0-9].
var segmentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// unsafeChar matches anything a normalized segment may not contain.
var unsafeChar = regexp.MustCompile(`[^a-z0-9._-]`)

// dashRun matches two or more consecutive '-', collapsed to one.
var dashRun = regexp.MustCompile(`-{2,}`)

// scpLike matches git's [user@]host:path remote syntax, which has no
// scheme and is therefore not a valid net/url URL.
var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)

// ValidateName reports whether name is a valid repository name per
// config.md section 4.8: 1 to 3 segments joined by "/", each matching
// ^[a-z0-9][a-z0-9._-]*$, at most 64 characters, and never "." or "..". It
// performs no normalization — a name that comes from a person is checked
// as written, not corrected, which is what lets a traversal attempt be
// rejected on its shape rather than discovered by its effect.
//
// config.md section 4.8 also states a name is at most 200 characters
// total, and that bound holds here without a check of its own: at most 3
// segments of at most 64 characters, plus 2 separators, is 194 — always
// under 200 given the segment-count and segment-length limits above — so
// no string this function accepts can ever be longer, and no string it
// rejects for length needs a second rule to reject it for. An earlier
// version carried an explicit len(name) > 200 check; refinery-a96.35 found
// it unreachable — deleting it never changed which strings ValidateName
// accepted or rejected, only which of two error messages an already-too-
// long string got — and it was removed rather than kept for a case that
// cannot exist under the current segment-count and segment-length limits.
func ValidateName(name string) error {
	if len(name) == 0 {
		return errors.New("repository name must not be empty")
	}
	segments := strings.Split(name, "/")
	if len(segments) > 3 {
		return fmt.Errorf("repository name %q has %d segments, more than the 3 allowed", name, len(segments))
	}
	for _, seg := range segments {
		if err := validateSegment(seg); err != nil {
			return fmt.Errorf("repository name %q: %w", name, err)
		}
	}
	return nil
}

// validateSegment checks one path segment against config.md section 4.8.
func validateSegment(seg string) error {
	if seg == "." || seg == ".." {
		return fmt.Errorf("segment %q is not allowed", seg)
	}
	if len(seg) == 0 {
		return errors.New("segment must not be empty")
	}
	if len(seg) > 64 {
		return fmt.Errorf("segment %q is %d characters, more than the 64 allowed", seg, len(seg))
	}
	if !segmentPattern.MatchString(seg) {
		return fmt.Errorf("segment %q does not match ^[a-z0-9][a-z0-9._-]*$", seg)
	}
	return nil
}

// ValidateRef reports whether ref is a valid commit ref per config.md
// section 4.3: exactly 40 lowercase hex characters.
func ValidateRef(ref string) error {
	if len(ref) != 40 {
		return fmt.Errorf("ref %q must be exactly 40 characters, has %d", ref, len(ref))
	}
	for _, r := range ref {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return fmt.Errorf("ref %q must be lowercase hex", ref)
		}
	}
	return nil
}

// normalizeSegment applies config.md section 4.8's order to one candidate
// segment: lowercase, replace every character outside [a-z0-9._-] with
// '-', collapse runs of '-', trim any leading or trailing character that is
// not [a-z0-9], then truncate to 64. The order matters — lowercasing first
// is what keeps "My_Repo" from becoming "-y-repo": replacing before
// lowercasing would treat the uppercase M and R as outside the
// (case-sensitive) safe set and drop them, where lowercasing first keeps
// every letter and leaves only the case changed. Trimming before
// truncating is what keeps a 64-character result still matching the
// leading-character anchor. An empty result signals the caller to fall
// back.
func normalizeSegment(s string) string {
	s = strings.ToLower(s)
	s = unsafeChar.ReplaceAllString(s, "-")
	s = dashRun.ReplaceAllString(s, "-")
	s = strings.TrimFunc(s, func(r rune) bool { return !isAlnum(r) })
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// isAlnum reports whether r is in [a-z0-9], the set normalizeSegment trims
// down to at each edge.
func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// normalizeName runs every element of segments through normalizeSegment and
// joins the result with "/". It fails — telling the caller to try the next
// derivation, or fall back to no-repo — when any segment normalizes empty,
// when the joined name is empty, when it has more than 3 segments, when it
// is over 200 characters, or when it equals the reserved no-repo (config.md
// sections 4.2 and 4.8). None of those is possible for a name accepted by
// ValidateName, but a derived name has not been validated by a person and
// must be rejected on its shape rather than trusted.
func normalizeName(segments []string) (string, bool) {
	if len(segments) == 0 || len(segments) > 3 {
		return "", false
	}
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		norm := normalizeSegment(seg)
		if norm == "" {
			return "", false
		}
		out = append(out, norm)
	}
	name := strings.Join(out, "/")
	if name == "" || name == noRepo || len(name) > 200 {
		return "", false
	}
	return name, true
}

// RepoName resolves the repository name for workingDir per config.md
// section 4.2: store.repos overrides everything, keyed by the worktree
// root when there is one or by workingDir when there is not; failing that,
// the normalized origin remote; failing that, local/<basename of the
// worktree root>; failing that, the reserved no-repo. overrides is
// consulted as given — a name that comes from a person is never
// normalized or validated here (config.md section 4.8); that is the config
// loader's and --repo's job.
func RepoName(ctx context.Context, git gitRunner, workingDir string, overrides map[string]string) (string, error) {
	root, err := git.root(ctx, workingDir)
	if err != nil && !errors.Is(err, verify.ErrNoRepository) {
		return "", err
	}
	key := root
	if key == "" {
		key = workingDir
	}
	if name, ok := overrides[key]; ok {
		return name, nil
	}
	if root == "" {
		return noRepo, nil
	}
	if name, ok := remoteName(ctx, git, root); ok {
		return name, nil
	}
	if name, ok := localName(root); ok {
		return name, nil
	}
	return noRepo, nil
}

// remoteName derives a repository name from root's origin remote, per
// config.md section 4.2's normalization (lowercase the host, drop
// userinfo, port, a trailing .git, and leading or trailing /) followed by
// section 4.8's general path-safety normalization. It reports false when
// there is no origin, or when nothing usable survives normalization.
func remoteName(ctx context.Context, git gitRunner, root string) (string, bool) {
	raw, err := git.originURL(ctx, root)
	if err != nil || raw == "" {
		return "", false
	}
	host, path, ok := parseOrigin(raw)
	if !ok {
		return "", false
	}
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	segments := []string{host}
	if path != "" {
		segments = append(segments, strings.Split(path, "/")...)
	}
	return normalizeName(segments)
}

// parseOrigin splits a git remote URL into its host and path, ahead of
// config.md section 4.2's normalization. It handles the scp-like
// [user@]host:path syntax git accepts alongside ordinary URLs, and reports
// false for a remote with no host at all — a local filesystem path, which
// has no host component to derive a name from and falls through to
// local/<basename> the same as having no origin.
func parseOrigin(raw string) (host, path string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	if !strings.Contains(raw, "://") {
		if m := scpLike.FindStringSubmatch(raw); m != nil {
			return strings.ToLower(m[1]), m[2], true
		}
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", "", false
	}
	return strings.ToLower(u.Hostname()), strings.TrimPrefix(u.Path, "/"), true
}

// localName derives a repository name from root's basename, per config.md
// section 4.2's local/<basename> fallback. It reports false when the
// basename normalizes to nothing usable.
func localName(root string) (string, bool) {
	seg := normalizeSegment(filepath.Base(root))
	if seg == "" {
		return "", false
	}
	name := "local/" + seg
	if name == noRepo || len(name) > 200 {
		return "", false
	}
	return name, true
}
