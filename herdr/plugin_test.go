package herdr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedPluginMatchesSourceFiles(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"herdr-plugin.toml", "bin/create", "bin/open"} {
		want, err := os.ReadFile(name)
		require.NoError(t, err)
		got, err := pluginFiles.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, string(want), string(got), "%s: embedded contents differ from source", name)
	}
}

func TestWritePluginReplacesDestinationAndPreservesExecutables(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "timber")
	stalePath := filepath.Join(destination, "stale")
	require.NoError(t, os.MkdirAll(destination, 0o755))
	require.NoError(t, os.WriteFile(stalePath, []byte("stale\n"), 0o644))

	require.NoError(t, WritePlugin(destination))

	_, err := os.Stat(stalePath)
	require.ErrorIs(t, err, os.ErrNotExist, "stale file still present")

	for _, name := range []string{"herdr-plugin.toml", "bin/create", "bin/open"} {
		got, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
		require.NoError(t, err)
		want, err := pluginFiles.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, string(want), string(got), "%s: installed contents differ from embed", name)
	}

	createInfo, err := os.Stat(filepath.Join(destination, "bin", "create"))
	require.NoError(t, err)
	require.NotZero(t, createInfo.Mode()&0o111, "bin/create is not executable")
}
