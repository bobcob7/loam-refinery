package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bobcob7/loam-refinery/internal/config"
	"github.com/bobcob7/loam-refinery/internal/profile"
)

// profilesAdapter is the concrete implementation of internal/cli's
// profileSource interface (docs/cli.md §2.1.1). main.go constructs one
// unconditionally on every invocation, so what keeps bare prime cheap is
// not deferred construction - the type is an empty struct - but lazy
// resolution: config.ProfilesDir is only called inside Load and List
// themselves, so a bare prime, which calls neither, never touches the
// filesystem (docs/cli.md §6.1).
//
// The directory is resolved with config.ProfilesDir, never config.Load: per
// docs/config.md §2, profiles have no config-file counterpart, and an
// unparseable config.json must not stop prime --profile from working.
type profilesAdapter struct{}

// newProfilesAdapter returns a profileSource that resolves the profile
// directory fresh on every call.
func newProfilesAdapter() *profilesAdapter {
	return &profilesAdapter{}
}

// Load implements internal/cli's profileSource.
func (a *profilesAdapter) Load(name string) (profile.Profile, bool, error) {
	dir, err := config.ProfilesDir()
	if err != nil {
		return profile.Profile{}, false, err
	}
	return profile.New(dir).Load(name)
}

// List implements internal/cli's profileSource. It resolves and enumerates
// the directory itself rather than delegating to profile.Reader.List:
// Reader.List's contract is deliberately unchanged (docs/cli.md §2.1.5) and
// fails the whole call on any one unparseable file, while prime --list
// needs the opposite - the profiles that do parse, plus the name of every
// file that does not, never failing for a parse error alone. Reader.Load
// already treats an invalid or malformed name as "not found" rather than
// an error, so calling it once per candidate name reproduces Reader.List's
// identity-matched read without duplicating any of its unexported logic.
func (a *profilesAdapter) List() ([]profile.Profile, []string, error) {
	dir, err := config.ProfilesDir()
	if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []profile.Profile{}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	reader := profile.New(dir)
	profiles := make([]profile.Profile, 0, len(entries))
	var broken []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok {
			continue
		}
		p, found, err := reader.Load(name)
		if err != nil {
			broken = append(broken, e.Name())
			continue
		}
		if !found {
			continue
		}
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, broken, nil
}
