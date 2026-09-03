package timber

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFromProcessCapturesProcessState(t *testing.T) {
	t.Parallel()

	runtime, err := RuntimeFromProcess()
	require.NoError(t, err)

	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	homeDirectory, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, currentDirectory, runtime.CurrentDirectory)
	assert.Equal(t, homeDirectory, runtime.HomeDirectory)
	assert.NotEmpty(t, runtime.DataHome)
	assert.NotEmpty(t, runtime.ConfigHome)
	assert.NotEmpty(t, runtime.WorktreeRoot)
	assert.Equal(t, os.TempDir(), runtime.TemporaryDirectory)
	assert.NotNil(t, runtime.Environment)
}

func TestValueOrDefault(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "value", valueOrDefault("value", "fallback"))
	assert.Equal(t, "fallback", valueOrDefault("", "fallback"))
}
