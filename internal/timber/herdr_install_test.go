package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHerdrInstallWritesPluginAndLinks(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := runTimberCommand(t, "herdr", "install")
	require.NoError(t, result.err, result.stderr)

	destination := filepath.Join(configHome, "herdr", "plugins", "timber")
	assert.Contains(t, result.stderr, "installed herdr plugin to "+destination)
	assert.Contains(t, result.stderr, "linked herdr plugin nnutter.timber")
	assert.Contains(t, result.stdout, filepath.Join(configHome, "herdr", "config.toml"))
	assert.Contains(t, result.stdout, herdrKeybindingTOML)
	assert.Equal(t, []string{
		fakeHerdrLogLine("plugin", "link", destination, "--enabled"),
	}, readFakeHerdrLog(t, logPath))

	for _, name := range []string{"herdr-plugin.toml", "bin/create", "bin/open"} {
		got, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
		require.NoError(t, err)
		want, err := os.ReadFile(filepath.Join("..", "..", "herdr", filepath.FromSlash(name)))
		require.NoError(t, err)
		assert.Equal(t, want, got, name)
	}

	createInfo, err := os.Stat(filepath.Join(destination, "bin", "create"))
	require.NoError(t, err)
	assert.NotEqual(t, 0, createInfo.Mode()&0o111)
}

func TestHerdrInstallFailsWhenPluginLinkFails(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)
	t.Setenv("FAKE_HERDR_FAIL", "plugin link")

	result := runTimberCommand(t, "herdr", "install")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr plugin link")
	assert.FileExists(t, filepath.Join(configHome, "herdr", "plugins", "timber", "herdr-plugin.toml"))
	assert.NotContains(t, result.stdout, herdrKeybindingTOML)
}
