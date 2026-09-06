package timber

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICreateNoTitlePassesShowTitleFalse(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	prompter := &stubCreateWizardPrompter{selection: createWizardSelection{cancelled: true}}
	options := &tuiCreateCommandOptions{runtime: testRepository.runtime, noTitle: true}
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	assert.False(t, prompter.showTitle)
}

func TestTUICommandRegistersNoTitleFlag(t *testing.T) {
	t.Parallel()
	command := NewTUICommand(testRuntime(t))

	flag := command.Flags().Lookup("no-title")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Hide the title header", flag.Usage)
}

func TestCreateWizardRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()
	prompter := bubbleteaCreateWizardPrompter{interactive: func() bool { return false }}
	_, err := prompter.Prompt(bytes.NewBuffer(nil), io.Discard, []registeredRepo{{Name: testRepoName}}, make([]managedWorktree, 0), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interactive terminal")
}
