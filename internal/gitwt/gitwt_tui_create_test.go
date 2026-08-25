package gitwt

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWizardEmptyNameShowsRequiredError(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	assert.Contains(t, model.View(), "name is required")
	assert.Equal(t, createWizardNameFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, model.cancelled)
}

func TestCreateWizardShiftTabBackDoesNotShowRequiredError(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyShiftTab},
	)

	assert.NotContains(t, model.View(), "name is required")
	assert.NotEqual(t, createWizardNameFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, model.cancelled)
}

func TestCreateWizardEscBackDoesNotShowRequiredError(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEsc},
	)

	assert.NotContains(t, model.View(), "name is required")
	assert.NotEqual(t, createWizardNameFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, model.cancelled)
}

func TestCreateWizardEscOnRepositoryCancels(t *testing.T) {
	model := newCreateWizardModel([]registeredRepo{{Name: testRepoName}})
	_ = model.Init()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(*createWizardModel)

	assert.True(t, model.cancelled)
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd())
}

func TestCreateWizardShiftTabOnRepositoryDoesNotCancel(t *testing.T) {
	model := driveCreateWizard(t, tea.KeyMsg{Type: tea.KeyShiftTab})

	assert.False(t, model.cancelled)
	assert.NotEqual(t, createWizardNameFieldKey, model.form.GetFocusedField().GetKey())
	assert.NotContains(t, model.View(), "name is required")
}

func TestCreateWizardRequiredErrorStillShowsAfterGoingBack(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	assert.Contains(t, model.View(), "name is required")
	assert.False(t, model.cancelled)
}

func TestRequireWorktreeName(t *testing.T) {
	require.EqualError(t, requireWorktreeName(""), "name is required")
	assert.NoError(t, requireWorktreeName("feature/login"))
}

func driveCreateWizard(t *testing.T, messages ...tea.Msg) *createWizardModel {
	t.Helper()

	model := newCreateWizardModel([]registeredRepo{{Name: testRepoName}})
	_ = model.Init()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(*createWizardModel)
	for _, message := range messages {
		next, cmd := model.Update(message)
		model = applyCreateWizardCmd(t, next, cmd)
	}
	return model
}

func applyCreateWizardCmd(t *testing.T, model tea.Model, cmd tea.Cmd) *createWizardModel {
	t.Helper()

	result, ok := model.(*createWizardModel)
	require.True(t, ok)
	if cmd == nil {
		return result
	}
	message := cmd()
	if message == nil {
		return result
	}
	if _, ok := message.(tea.BatchMsg); ok {
		return result
	}
	updated, _ := result.Update(message)
	result, ok = updated.(*createWizardModel)
	require.True(t, ok)
	return result
}
