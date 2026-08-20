// Package profile reads reviewer profiles, per docs/cli.md §2.1.1-§2.1.6:
// one Markdown file per profile, named <name>.md, opening with frontmatter
// that carries exactly one key (description) and closing with a body that
// prime appends verbatim.
//
// Profiles are config, not data (docs/config.md §2), and this package treats
// the directory the same way internal/config treats config.json: read-only,
// never created, and a caller supplies the directory rather than this
// package resolving it - see config.ProfilesDir for that half.
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// maxDescriptionRunes is docs/cli.md §2.1.2's limit on description, counted
// in runes rather than bytes - this repository has been bitten before by a
// byte-offset assumption over multi-byte UTF-8.
const maxDescriptionRunes = 120

// frontMatterFence is the line, alone, that opens and closes frontmatter.
const frontMatterFence = "---"

// Profile is one reviewer profile: Description is what --list shows, and
// Body is the file's content after the closing fence, trimmed and never
// otherwise reflowed - callers frame it (docs/cli.md §2.1.3).
type Profile struct {
	Name        string
	Description string
	Body        string
}

// Reader reads profiles from one directory. It holds no other state, the
// same way internal/store's Store holds only the paths it was constructed
// with.
type Reader struct {
	dir string
}

// New constructs a Reader over dir. dir is not required to exist: a missing
// directory is List's empty result, not a construction error.
func New(dir string) *Reader {
	return &Reader{dir: dir}
}

// List returns every valid profile in the directory, sorted by name. A
// non-.md file, and a .md file whose stem is not a valid name, is ignored
// rather than rejected (docs/cli.md §2.1.2) - so a README.md can sit beside
// the profiles it documents. A missing directory returns a non-nil empty
// result and a nil error, so a JSON caller renders "[]" rather than "null";
// a path that exists but is a regular file is an error. Every malformed
// profile in the directory is reported, not just the first (refinery-
// emv.12): the listing is still all-or-nothing on failure, but the single
// error names every broken file in one pass.
func (r *Reader) List() ([]Profile, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", r.dir, err)
	}
	profiles := make([]Profile, 0, len(entries))
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(entry.Name(), ".md")
		if !ok || !validName(name) {
			continue
		}
		p, ok, err := r.loadEntry(name, entry.Name())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !ok {
			continue
		}
		profiles = append(profiles, p)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	slices.SortFunc(profiles, func(a, b Profile) int { return strings.Compare(a.Name, b.Name) })
	return profiles, nil
}

// Load reads one profile by name. ok is false when name is not a valid
// profile name, or names no file in the directory - the CLI routes both to
// "unknown profile" (exit 2, docs/cli.md §2.1.4). A non-nil err means the
// file exists but is unreadable or malformed - the tool's own state, routed
// to exit 101 - and is never returned alongside ok=true.
//
// name is untrusted: it arrives from an orchestrator (docs/cli.md §2.1.2),
// and resolving it never touches the filesystem until it has been checked
// against the name charset, so a name carrying a separator, "..", a leading
// dot, or an absolute path can never reach a path outside dir.
//
// A path is never opened by name+".md" directly: on a case-insensitive
// filesystem the kernel would fold that open onto whatever entry
// case-folds to it, so Load("readme") could silently open README.md even
// though README.md is not a valid profile name and List would never show
// it (refinery-emv.7). Instead Load enumerates the directory, the same way
// List does, and only opens an entry whose name matches name+".md" byte for
// byte - an identity check, not a containment one, so it agrees with List
// over every entry.
func (r *Reader) Load(name string) (Profile, bool, error) {
	if !validName(name) {
		return Profile{}, false, nil
	}
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, fmt.Errorf("reading %s: %w", r.dir, err)
	}
	want := name + ".md"
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() == want {
			return r.loadEntry(name, want)
		}
	}
	return Profile{}, false, nil
}

// loadEntry reads and parses one directory entry already known to be
// filename byte for byte - the caller has either matched it by identity
// (Load) or read it straight off a ReadDir result (List), so this never
// re-enumerates the directory itself. That split keeps List's one pass over
// its own entries at O(n) instead of a ReadDir per entry.
func (r *Reader) loadEntry(name, filename string) (Profile, bool, error) {
	path := filepath.Join(r.dir, filename)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	p, err := parse(name, raw)
	if err != nil {
		return Profile{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	return p, true, nil
}

// validName is docs/cli.md §2.1.2's name charset: lowercase letters, digits,
// and hyphens only. The charset alone is what makes a separator, "..", a
// leading dot, and an absolute path all invalid - none of "/", ".", or an
// uppercase letter is in it, so no case has to be checked separately.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' {
			continue
		}
		return false
	}
	return true
}

// parse decodes one profile file: frontmatter fenced by two lines that read
// exactly "---", a closed key set of {description}, and a body that is
// everything after the closing fence, trimmed.
func parse(name string, raw []byte) (Profile, error) {
	// CRLF normalizes to LF before splitting: a profile hand-edited on a
	// core.autocrlf=true checkout must parse the same as its LF twin, not
	// fail with lines[0] == "---\r" and a message that says nothing about
	// line endings (refinery-emv.8).
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if lines[0] != frontMatterFence {
		return Profile{}, errors.New("missing frontmatter: file must open with \"---\"")
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == frontMatterFence {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return Profile{}, errors.New("unterminated frontmatter: no closing \"---\"")
	}
	description, err := parseDescription(lines[1:closeIdx])
	if err != nil {
		return Profile{}, err
	}
	body := strings.TrimSpace(strings.Join(lines[closeIdx+1:], "\n"))
	if body == "" {
		return Profile{}, errors.New("empty body")
	}
	return Profile{Name: name, Description: description, Body: body}, nil
}

// parseDescription decodes the frontmatter lines between the two fences: the
// closed key set of {description}, required, non-empty, and at most
// maxDescriptionRunes runes.
func parseDescription(lines []string) (string, error) {
	found := false
	description := ""
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return "", fmt.Errorf("malformed frontmatter line %q", line)
		}
		key = strings.TrimSpace(key)
		if key != "description" {
			return "", fmt.Errorf("unknown frontmatter key %q", key)
		}
		if found {
			return "", errors.New("duplicate frontmatter key \"description\"")
		}
		found = true
		description = strings.TrimSpace(value)
	}
	if !found {
		return "", errors.New("missing required frontmatter key \"description\"")
	}
	if description == "" {
		return "", errors.New("description must not be empty")
	}
	if n := utf8.RuneCountInString(description); n > maxDescriptionRunes {
		return "", fmt.Errorf("description is %d characters, over the %d limit", n, maxDescriptionRunes)
	}
	return description, nil
}
