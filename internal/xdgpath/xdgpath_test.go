package xdgpath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/xdgpath"
)

// setenv sets an environment variable for the duration of a test, restoring
// the original value (or unsetting it) when the test finishes.
func setenv(t *testing.T, key, value string) {
	t.Helper()
	prev, ok := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if ok {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestCacheDir(t *testing.T) {
	tmp := t.TempDir()

	// Override every env var that os.UserCacheDir / XDG machinery consult so
	// the test is hermetic on Linux, macOS, and Windows alike.
	setenv(t, "XDG_CACHE_HOME", tmp)
	setenv(t, "HOME", tmp)
	setenv(t, "USERPROFILE", tmp)  // Windows fallback
	setenv(t, "LOCALAPPDATA", tmp) // Windows %LocalAppData%

	dir, err := xdgpath.CacheDir()
	require.NoError(t, err)

	// Must end with the versioned sub-path.
	require.True(t, filepath.IsAbs(dir), "CacheDir must return an absolute path")
	require.DirExists(t, dir, "CacheDir must create the directory")
	require.Contains(t, dir, "gh-dash", "path must contain app name")
	require.Contains(t, dir, "v1", "path must contain version component")

	// Calling again must be idempotent.
	dir2, err := xdgpath.CacheDir()
	require.NoError(t, err)
	require.Equal(t, dir, dir2)
}

func TestConfigDir(t *testing.T) {
	tmp := t.TempDir()

	setenv(t, "XDG_CONFIG_HOME", tmp)
	setenv(t, "HOME", tmp)
	setenv(t, "USERPROFILE", tmp)
	setenv(t, "APPDATA", tmp) // Windows %AppData%

	dir, err := xdgpath.ConfigDir()
	require.NoError(t, err)

	require.True(t, filepath.IsAbs(dir))
	require.DirExists(t, dir)
	require.Contains(t, dir, "gh-dash")

	// XDG_CONFIG_HOME must be honoured.
	require.Contains(t, dir, tmp, "ConfigDir must be rooted at XDG_CONFIG_HOME")

	// Idempotent.
	dir2, err := xdgpath.ConfigDir()
	require.NoError(t, err)
	require.Equal(t, dir, dir2)
}

func TestConfigDir_WithoutXDG(t *testing.T) {
	tmp := t.TempDir()

	// Remove XDG override to exercise the os.UserConfigDir fallback path.
	setenv(t, "XDG_CONFIG_HOME", "")
	setenv(t, "HOME", tmp)
	setenv(t, "USERPROFILE", tmp)
	setenv(t, "APPDATA", tmp)

	dir, err := xdgpath.ConfigDir()
	require.NoError(t, err)

	require.True(t, filepath.IsAbs(dir))
	require.DirExists(t, dir)
	require.Contains(t, dir, "gh-dash")
}

func TestStateDir(t *testing.T) {
	tmp := t.TempDir()

	setenv(t, "XDG_CONFIG_HOME", tmp)
	setenv(t, "HOME", tmp)
	setenv(t, "USERPROFILE", tmp)
	setenv(t, "APPDATA", tmp)

	state, err := xdgpath.StateDir()
	require.NoError(t, err)

	require.True(t, filepath.IsAbs(state))
	require.DirExists(t, state)
	require.Contains(t, state, "gh-dash")
	require.Contains(t, state, "state")
	require.Contains(t, state, "v1")

	// StateDir must be a sub-directory of ConfigDir.
	cfg, err := xdgpath.ConfigDir()
	require.NoError(t, err)
	rel, err := filepath.Rel(cfg, state)
	require.NoError(t, err)
	require.False(t, filepath.IsAbs(rel), "StateDir must be inside ConfigDir")

	// Idempotent.
	state2, err := xdgpath.StateDir()
	require.NoError(t, err)
	require.Equal(t, state, state2)
}
