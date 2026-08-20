package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A real profile file on disk resolves through the real adapter end to end:
// Load reads it by name and List finds it enumerating the directory, both
// through config.ProfilesDir rather than a mock. Mutating Load's body to
// "return profile.Profile{}, false, nil" (refinery-emv.21) fails this.
func TestProfilesAdapter_LoadAndListResolveARealProfile(t *testing.T) {
	home := homeFor(t)
	writeProfile(t, home, "backend", "Go services; concurrency, error wrapping, context handling", "Look hard at goroutine leaks.")
	adapter := newProfilesAdapter()
	p, ok, err := adapter.Load("backend")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "backend", p.Name)
	assert.Equal(t, "Go services; concurrency, error wrapping, context handling", p.Description)
	assert.Equal(t, "Look hard at goroutine leaks.", p.Body)
	profiles, broken, err := adapter.List()
	require.NoError(t, err)
	assert.Empty(t, broken)
	require.Len(t, profiles, 1)
	assert.Equal(t, "backend", profiles[0].Name)
}

// docs/cli.md §2.1.5 (refinery-emv.18): a file that fails to parse is
// omitted from the index and named in broken instead - never folded into
// profiles, which is what would happen if a broken file's zero-value
// profile were appended alongside the ones that did parse.
func TestProfilesAdapter_ListOmitsBrokenProfilesAndNamesThem(t *testing.T) {
	home := homeFor(t)
	writeProfile(t, home, "backend", "d", "b")
	dir := filepath.Join(home, "profiles")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "wip.md"), []byte("not a profile file"), 0o600))
	adapter := newProfilesAdapter()
	profiles, broken, err := adapter.List()
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "backend", profiles[0].Name)
	assert.Equal(t, []string{"wip.md"}, broken)
}

// The directory is resolved with config.ProfilesDir, never config.Load
// (cmd/loam-refinery/profiles.go's own comment): an unparseable config.json
// must not stop a profile from loading, since profiles have no config-file
// counterpart (docs/config.md §2). Mutating the adapter to resolve via
// config.Load instead breaks this, because config.Load fails outright on a
// config.json that will not parse.
func TestProfilesAdapter_UnparseableConfigDoesNotBlockALoad(t *testing.T) {
	home := homeFor(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.json"), []byte("not json"), 0o600))
	writeProfile(t, home, "backend", "d", "b")
	adapter := newProfilesAdapter()
	p, ok, err := adapter.Load("backend")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "backend", p.Name)
	profiles, broken, err := adapter.List()
	require.NoError(t, err)
	assert.Empty(t, broken)
	require.Len(t, profiles, 1)
}

// A directory that cannot be resolved at all - here, no LOAM_REFINERY_HOME,
// no XDG_CONFIG_HOME, and no HOME to fall back to - is the tool's own
// state, and the error has to reach ExitTool through the real wiring
// run() builds, not just through a mock configured to return one.
func TestProfilesAdapter_UnresolvableDirectoryReachesExitTool(t *testing.T) {
	t.Setenv("LOAM_REFINERY_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(t.Context(), []string{"prime", "--profile=backend"}, bytes.NewReader(nil), stdout, stderr)
	assert.Equal(t, cli.ExitTool, code)
	assert.Empty(t, stdout.String(), "a tool-error exit must print nothing on stdout")
}

// Bare prime never calls the profile source at all (docs/cli.md §6.1), and
// this proves it through the real adapter rather than a mock that could
// simply never be wired to panic: home is made completely inaccessible
// after the run's other setup happens, so any attempt to enumerate or read
// inside it - including a ReadDir on home/profiles - fails loudly. Bare
// prime still has to exit clean.
func TestBarePrime_TouchesNoFilesystemThroughTheRealAdapter(t *testing.T) {
	home := homeFor(t)
	require.NoError(t, os.Chmod(home, 0o000))
	t.Cleanup(func() { require.NoError(t, os.Chmod(home, 0o700)) })
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(t.Context(), []string{"prime"}, bytes.NewReader(nil), stdout, stderr)
	assert.Equal(t, cli.ExitValid, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "loam-refinery")
}

// writeProfile writes name.md under home/profiles, creating the directory
// if needed, in the frontmatter shape internal/profile.Reader parses.
func writeProfile(t *testing.T, home, name, description, body string) {
	t.Helper()
	dir := filepath.Join(home, "profiles")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	content := "---\ndescription: " + description + "\n---\n\n" + body + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o600))
}
