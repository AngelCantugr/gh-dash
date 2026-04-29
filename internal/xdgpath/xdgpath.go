package xdgpath

import (
	"os"
	"path/filepath"
)

const (
	appDir      = "gh-dash"
	cacheSubDir = "v1"
	stateSubDir = "state/v1"
)

// CacheDir returns the gh-dash cache directory:
//
//	Linux:   $XDG_CACHE_HOME/gh-dash/v1/  or  $HOME/.cache/gh-dash/v1/
//	macOS:   $HOME/Library/Caches/gh-dash/v1/
//	Windows: %LocalAppData%/gh-dash/v1/
//
// Creates the directory if missing. Returns the absolute path.
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appDir, cacheSubDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ConfigDir returns the gh-dash config directory (state lives here too):
//
//	Linux:   $XDG_CONFIG_HOME/gh-dash/  or  $HOME/.config/gh-dash/
//	macOS:   $HOME/Library/Application Support/gh-dash/
//	Windows: %AppData%/gh-dash/
//
// Creates the directory if missing. Returns the absolute path.
func ConfigDir() (string, error) {
	base, err := configBase()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// StateDir returns the per-user session state directory:
//
//	ConfigDir() + "/state/v1/"
//
// Creates the directory if missing.
func StateDir() (string, error) {
	cfgDir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfgDir, stateSubDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// configBase returns the platform base config directory, honoring
// XDG_CONFIG_HOME on Linux (matching parser.go behavior).
func configBase() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}
	return os.UserConfigDir()
}
