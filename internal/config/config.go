// Package config resolves where settings and the review store live, and
// reads the config file that names them, per docs/config.md §2 and §3.
//
// Every error an exported function in this package returns is a config
// problem in the tool-error band (docs/cli.md §4, exit 101): a location that
// could not be determined, a file that could not be read, or one that could
// be read but not trusted. A missing config file is not an error — it is
// defaults, and Load returns those silently. A caller therefore never needs
// to inspect an error from this package to decide how to route it; err !=
// nil is already the exit-101 signal.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store is the store.* settings from the config file, fully resolved: Path
// is expanded and defaulted, Repos is copied as-is (empty rather than nil
// when absent).
type Store struct {
	Enabled bool
	Path    string
	Repos   map[string]string
}

// Config is the fully resolved configuration for one process: environment,
// config file, and built-in defaults collapsed per the precedence in
// docs/config.md §2.1.
type Config struct {
	Version    string
	Store      Store
	ConfigPath string
}

// flagOnlyKeys are validate's flags. They read as config keys, but a value
// that changes whether a document is valid may not come from a file two
// machines can disagree about (docs/config.md §3.1), so these are rejected
// with a message naming the flag rather than "unknown key".
var flagOnlyKeys = []string{"strict", "disable", "warn_only", "require_verification"}

// Load resolves the config file location from the environment, reads it if
// present, and returns the fully resolved Config. See the package doc for
// how a caller should treat a non-nil error.
func Load() (*Config, error) {
	loc, err := resolveLocations()
	if err != nil {
		return nil, err
	}
	cfg, err := loadFile(loc.configPath)
	if err != nil {
		return nil, err
	}
	switch {
	case loc.homeCollapsed:
		cfg.Store.Path = loc.defaultStorePath
	case cfg.Store.Path == "":
		cfg.Store.Path = loc.defaultStorePath
	}
	cfg.ConfigPath = loc.configPath
	return cfg, nil
}

// defaultsFile is the exact contents written on first use (docs/config.md
// §2.2): version and store.enabled, spelled out, and nothing else.
// store.path and store.repos are omitted deliberately — writing the
// resolved store path would freeze a default that should keep following the
// environment.
const defaultsFile = `{"version":"1","store":{"enabled":true}}`

// Materialize creates the config directory (mode 0700) and, if configPath
// does not already exist, writes the defaults-only file (mode 0600). It
// never overwrites an existing file.
//
// Call this explicitly where docs/config.md §2.2 calls for it — the first
// validate run with something to keep. It must not run from an init
// function or from package-level state; this package has none.
func Materialize(configPath string) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	f, err := os.OpenFile(configPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("creating %s: %w", configPath, err)
	}
	defer f.Close()
	if _, err := f.WriteString(defaultsFile); err != nil {
		return fmt.Errorf("writing %s: %w", configPath, err)
	}
	return nil
}

// RepoOverride looks up store.repos for one repository, matched against
// worktreeRoot when it is not empty, or workingDir when there is no
// worktree (docs/config.md §4.2).
func (c *Config) RepoOverride(worktreeRoot, workingDir string) (string, bool) {
	key := worktreeRoot
	if key == "" {
		key = workingDir
	}
	name, ok := c.Store.Repos[key]
	return name, ok
}

// locations is where the config file and the default store directory are,
// resolved from the environment alone — before the config file is read, so
// that Load knows where to read it from.
type locations struct {
	configPath       string
	homeCollapsed    bool
	defaultStorePath string
}

// resolveLocations implements docs/config.md §2: LOAM_REFINERY_HOME, when
// set, collapses config and store into one directory and the XDG variables
// are not consulted; otherwise config and store resolve independently under
// XDG_CONFIG_HOME/XDG_DATA_HOME, defaulting to ~/.config and
// ~/.local/share.
func resolveLocations() (locations, error) {
	if home := lookupEnv("LOAM_REFINERY_HOME"); home != "" {
		return locations{configPath: filepath.Join(home, "config.json"), homeCollapsed: true, defaultStorePath: home}, nil
	}
	configDir, err := xdgDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return locations{}, err
	}
	storeDir, err := xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if err != nil {
		return locations{}, err
	}
	return locations{configPath: filepath.Join(configDir, "loam-refinery", "config.json"), defaultStorePath: filepath.Join(storeDir, "loam-refinery")}, nil
}

// xdgDir resolves one XDG base directory: envVar if set, else
// $HOME/fallback.
func xdgDir(envVar, fallback string) (string, error) {
	if dir := lookupEnv(envVar); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, fallback), nil
}

// lookupEnv reads an environment variable, treating an empty value the same
// as an unset one — an exported XDG_CONFIG_HOME="" should not pin every
// path to the filesystem root.
func lookupEnv(name string) string {
	return os.Getenv(name)
}

// loadFile reads and parses the config file at path. A missing file yields
// silent defaults, per docs/config.md §3.
func loadFile(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Version: "1", Store: Store{Enabled: true, Repos: map[string]string{}}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return parse(raw, path)
}

// parse decodes the config file's root object. Unknown keys are rejected
// outright rather than ignored, and the four flag-only keys are named as
// such rather than as unknown keys (docs/config.md §3, §3.1).
func parse(raw []byte, path string) (*Config, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	for _, key := range flagOnlyKeys {
		if _, ok := root[key]; ok {
			return nil, fmt.Errorf("%s: %q is a flag (--%s), not a config setting", path, key, strings.ReplaceAll(key, "_", "-"))
		}
	}
	for key := range root {
		if key != "version" && key != "store" {
			return nil, fmt.Errorf("%s: unknown key %q", path, key)
		}
	}
	versionRaw, ok := root["version"]
	if !ok {
		return nil, fmt.Errorf("%s: missing required key %q", path, "version")
	}
	var version string
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		return nil, fmt.Errorf("%s: %q must be a string", path, "version")
	}
	if version != "1" {
		return nil, fmt.Errorf("%s: unsupported version %q", path, version)
	}
	cfg := &Config{Version: version, Store: Store{Enabled: true, Repos: map[string]string{}}}
	if storeRaw, ok := root["store"]; ok {
		if err := parseStore(storeRaw, path, &cfg.Store); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// parseStore decodes the store object: enabled, path, and repos, rejecting
// any other key.
func parseStore(raw json.RawMessage, path string, store *Store) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("%s: %q must be an object", path, "store")
	}
	for key := range fields {
		if key != "enabled" && key != "path" && key != "repos" {
			return fmt.Errorf("%s: unknown key %q", path, "store."+key)
		}
	}
	if v, ok := fields["enabled"]; ok {
		if err := json.Unmarshal(v, &store.Enabled); err != nil {
			return fmt.Errorf("%s: %q must be a boolean", path, "store.enabled")
		}
	}
	if v, ok := fields["path"]; ok {
		if err := parseStorePath(v, path, store); err != nil {
			return err
		}
	}
	if v, ok := fields["repos"]; ok {
		var repos map[string]string
		if err := json.Unmarshal(v, &repos); err != nil {
			return fmt.Errorf("%s: %q must be an object of strings", path, "store.repos")
		}
		store.Repos = repos
	}
	return nil
}

// parseStorePath decodes store.path: it must be a string, it expands a
// leading ~, and the result must be absolute (docs/config.md §3).
func parseStorePath(v json.RawMessage, path string, store *Store) error {
	var raw string
	if err := json.Unmarshal(v, &raw); err != nil {
		return fmt.Errorf("%s: %q must be a string", path, "store.path")
	}
	expanded, err := expandHome(raw)
	if err != nil {
		return fmt.Errorf("%s: %q: %w", path, "store.path", err)
	}
	if !filepath.IsAbs(expanded) {
		return fmt.Errorf("%s: %q must be an absolute path: %q", path, "store.path", raw)
	}
	store.Path = expanded
	return nil
}

// expandHome expands a leading ~ to the user's home directory. Only "~" and
// "~/..." are recognised; a "~user/..." form is left untouched, since
// nothing in docs/config.md asks for it.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}
