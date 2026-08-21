package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveLocations_Defaults, and its siblings below, exercise the
// environment directly with t.Setenv, which panics if called under a
// parallel ancestor — so this whole tree does not call t.Parallel().
func TestResolveLocations_Defaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", "")
	loc, err := resolveLocations()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "loam-refinery", "config.json"), loc.configPath)
	assert.Equal(t, filepath.Join(home, ".local", "share", "loam-refinery"), loc.defaultStorePath)
	assert.False(t, loc.homeCollapsed)
}

func TestResolveLocations_HomeVarCollapses(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "x")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", root)
	loc, err := resolveLocations()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "config.json"), loc.configPath)
	assert.Equal(t, root, loc.defaultStorePath)
	assert.True(t, loc.homeCollapsed)
}

func TestResolveLocations_HomeVarWinsOverXDG(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "x")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))
	t.Setenv("LOAM_REFINERY_HOME", root)
	loc, err := resolveLocations()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "config.json"), loc.configPath)
	assert.Equal(t, root, loc.defaultStorePath)
}

func TestProfilesDir_Defaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", "")
	dir, err := ProfilesDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "loam-refinery", "profiles"), dir)
	_, statErr := os.Stat(dir)
	assert.True(t, os.IsNotExist(statErr), "ProfilesDir must not create the directory")
}

func TestProfilesDir_XDGConfigHome(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "xdg-config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", "")
	dir, err := ProfilesDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configHome, "loam-refinery", "profiles"), dir)
}

// TestProfilesDir_HomeVarCollapses sets XDG_CONFIG_HOME to a real,
// different directory rather than clearing it: with XDG_CONFIG_HOME empty,
// LOAM_REFINERY_HOME losing precedence would fall through to the same HOME
// default XDG resolves to anyway, and the test would pass for the wrong
// reason. A genuinely competing XDG_CONFIG_HOME makes the assertion
// actually pin precedence (refinery-emv.11).
func TestProfilesDir_HomeVarCollapses(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "x")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", root)
	dir, err := ProfilesDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "profiles"), dir)
}

// TestProfilesDir_ErrorPropagatesFromResolveLocations exercises
// ProfilesDir's error return (refinery-emv.11): with no LOAM_REFINERY_HOME,
// no XDG_CONFIG_HOME, and no HOME, resolveLocations cannot find a home
// directory to fall back to, and that error must reach the caller.
func TestProfilesDir_ErrorPropagatesFromResolveLocations(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", "")
	_, err := ProfilesDir()
	assert.Error(t, err)
}

func TestProfilesDir_MalformedConfigFileDoesNotFail(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, "cfg")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "loam-refinery"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "loam-refinery", "config.json"), []byte("not json"), 0o600))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", "")
	dir, err := ProfilesDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "loam-refinery", "profiles"), dir)
}

func TestLoad_MissingFileYieldsDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", "")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "1", cfg.Version)
	assert.True(t, cfg.Store.Enabled)
	assert.Empty(t, cfg.Store.Repos)
	assert.Equal(t, filepath.Join(home, ".local", "share", "loam-refinery"), cfg.Store.Path)
	assert.Equal(t, filepath.Join(home, ".config", "loam-refinery", "config.json"), cfg.ConfigPath)
}

func TestLoad_HomeVarOverridesStorePathInFile(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "x")
	require.NoError(t, os.MkdirAll(root, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"version":"1","store":{"enabled":true,"path":"/somewhere/else"}}`), 0o600))
	t.Setenv("HOME", home)
	t.Setenv("LOAM_REFINERY_HOME", root)
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, root, cfg.Store.Path)
}

func TestLoad_StorePathFromFileUsedWithoutHomeVar(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, "cfg")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "loam-refinery"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "loam-refinery", "config.json"), []byte(`{"version":"1","store":{"enabled":true,"path":"/custom/store"}}`), 0o600))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOAM_REFINERY_HOME", "")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/custom/store", cfg.Store.Path)
}

func TestLoadFile_MissingFileIsNotAnError(t *testing.T) {
	t.Parallel()
	cfg, err := loadFile(filepath.Join(t.TempDir(), "config.json"))
	require.NoError(t, err)
	assert.Equal(t, "1", cfg.Version)
	assert.True(t, cfg.Store.Enabled)
	assert.NotNil(t, cfg.Store.Repos)
	assert.Empty(t, cfg.Store.Repos)
	assert.Empty(t, cfg.Store.Path)
}

// TestLoadFile_ValidConfig sets HOME to expand store.path's tilde
// predictably, so — like the resolveLocations tests above — it cannot run
// in parallel.
func TestLoadFile_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"version":"1","store":{"enabled":false,"path":"~/reviews","repos":{"/Users/me/src/refinery":"github.com/bobcob7/loam-refinery"}}}`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	t.Setenv("HOME", dir)
	cfg, err := loadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "1", cfg.Version)
	assert.False(t, cfg.Store.Enabled)
	assert.Equal(t, filepath.Join(dir, "reviews"), cfg.Store.Path)
	assert.Equal(t, map[string]string{"/Users/me/src/refinery": "github.com/bobcob7/loam-refinery"}, cfg.Store.Repos)
}

func TestLoadFile_UnreadableFile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced the same way on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores unreadable permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1"}`), 0o000))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_UnparseableJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{not json`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_UnknownKeyNamesTheKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","store":{"enable":true}}`), 0o600))
	_, err := loadFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store.enable")
}

func TestLoadFile_TopLevelUnknownKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","bogus":true}`), 0o600))
	_, err := loadFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"bogus"`)
	assert.Contains(t, err.Error(), "unknown key")
}

func TestLoadFile_FlagOnlyKeysNameTheFlagNotUnknownKey(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"strict"} {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","`+key+`":true}`), 0o600))
			_, err := loadFile(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
			assert.NotContains(t, err.Error(), "unknown key")
		})
	}
}

// disable, warn_only, and require_verification used to be flag-only keys too
// (--disable, --warn-only, --require-verification), but refinery-uyb.5
// dropped all three flags entirely. A config file naming one of them now
// falls through to the ordinary "unknown key" message, distinct from
// strict's continued "is a flag" message above (docs/config.md §3.1).
func TestLoadFile_RemovedFlagKeysAreUnknownNotFlags(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"disable", "warn_only", "require_verification"} {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","`+key+`":true}`), 0o600))
			_, err := loadFile(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
			assert.Contains(t, err.Error(), "unknown key")
			assert.NotContains(t, err.Error(), "is a flag")
		})
	}
}

func TestLoadFile_MissingVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"store":{"enabled":true}}`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_WrongVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"2"}`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_VersionWrongType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1}`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_StorePathRelativeRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","store":{"path":"relative/path"}}`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_StoreEnabledWrongType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","store":{"enabled":"yes"}}`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_StoreNotAnObject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","store":true}`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_ReposWrongShape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"1","store":{"repos":{"a":1}}}`), 0o600))
	_, err := loadFile(path)
	assert.Error(t, err)
}

func TestMaterialize_FirstRunWritesExactDefaults(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced the same way on windows")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nested", "loam-refinery", "config.json")
	require.NoError(t, Materialize(configPath))
	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, `{"version":"1","store":{"enabled":true}}`, string(body))
	dirInfo, err := os.Stat(filepath.Dir(configPath))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
	cfg, err := loadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "1", cfg.Version)
	assert.True(t, cfg.Store.Enabled)
	assert.Empty(t, cfg.Store.Path)
	assert.Empty(t, cfg.Store.Repos)
}

func TestMaterialize_NeverOverwritesExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	custom := `{"version":"1","store":{"enabled":false}}`
	require.NoError(t, os.WriteFile(configPath, []byte(custom), 0o600))
	require.NoError(t, Materialize(configPath))
	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, custom, string(body))
}

func TestMaterialize_IdempotentWhenDirectoryAlreadyExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	require.NoError(t, Materialize(configPath))
	require.NoError(t, Materialize(configPath))
	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, `{"version":"1","store":{"enabled":true}}`, string(body))
}

func TestConfig_RepoOverride(t *testing.T) {
	t.Parallel()
	cfg := &Config{Store: Store{Repos: map[string]string{
		"/some/cwd":      "cwd-repo",
		"/some/worktree": "worktree-repo",
	}}}
	t.Run("no worktree matches the working directory", func(t *testing.T) {
		t.Parallel()
		name, ok := cfg.RepoOverride("", "/some/cwd")
		require.True(t, ok)
		assert.Equal(t, "cwd-repo", name)
	})
	t.Run("a worktree root takes precedence over the working directory", func(t *testing.T) {
		t.Parallel()
		name, ok := cfg.RepoOverride("/some/worktree", "/some/cwd")
		require.True(t, ok)
		assert.Equal(t, "worktree-repo", name)
	})
	t.Run("no match", func(t *testing.T) {
		t.Parallel()
		_, ok := cfg.RepoOverride("/nope", "/also-nope")
		assert.False(t, ok)
	})
}
