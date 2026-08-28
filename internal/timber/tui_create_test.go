package timber

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWizardEmptyNameShowsRequiredError(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyEnter},
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
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEsc},
	)

	assert.NotContains(t, model.View(), "name is required")
	assert.NotEqual(t, createWizardNameFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, model.cancelled)
}

func TestCreateWizardEscOnActionCancels(t *testing.T) {
	model := newCreateWizardModel([]registeredRepo{{Name: testRepoName}}, nil)
	_ = model.Init()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(*createWizardModel)

	assert.True(t, model.cancelled)
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd())
}

func TestCreateWizardEscOnRepositoryGoesBack(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEsc},
	)

	assert.False(t, model.cancelled)
	assert.Equal(t, createWizardActionFieldKey, model.form.GetFocusedField().GetKey())
}

func TestCreateWizardShiftTabOnActionDoesNotCancel(t *testing.T) {
	model := driveCreateWizard(t, tea.KeyMsg{Type: tea.KeyShiftTab})

	assert.False(t, model.cancelled)
	assert.Equal(t, createWizardActionFieldKey, model.form.GetFocusedField().GetKey())
	assert.NotContains(t, model.View(), "name is required")
}

func TestCreateWizardRequiredErrorStillShowsAfterGoingBack(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	assert.Contains(t, model.View(), "name is required")
	assert.False(t, model.cancelled)
}

func TestCreateWizardJMovesDownOnActionAndRepository(t *testing.T) {
	model := driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: "alpha"}, {Name: "beta"}},
		nil,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
	)

	assert.Equal(t, wizardActionOpen, model.action)
	assert.Equal(t, createWizardActionFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, focusedFieldIsFiltering(model.form))

	model = driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: "alpha"}, {Name: "beta"}},
		nil,
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
	)

	assert.Equal(t, wizardActionCreate, model.action)
	assert.Equal(t, "beta", model.repoName)
	assert.Equal(t, createWizardRepoFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, focusedFieldIsFiltering(model.form))
}

func TestCreateWizardJMovesDownOnExistingWorktrees(t *testing.T) {
	worktrees := []managedWorktree{
		{Repo: testRepoName, Name: "feature/one"},
		{Repo: testRepoName, Name: "feature/two"},
	}
	model := driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: testRepoName}},
		worktrees,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
	)

	assert.Equal(t, wizardActionOpen, model.action)
	assert.Equal(t, "feature/two@"+testRepoName, model.existingWorktree)
	assert.Equal(t, createWizardWorktreeFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, focusedFieldIsFiltering(model.form))
}

func TestCreateWizardRequiresSlashToFilter(t *testing.T) {
	model := driveCreateWizard(t, tea.KeyMsg{Type: tea.KeyEnter})

	assert.Contains(t, model.View(), "Repository")
	assert.False(t, focusedFieldIsFiltering(model.form))

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = applyCreateWizardCmd(t, updated, cmd)
	assert.False(t, focusedFieldIsFiltering(model.form))
	assert.Contains(t, model.View(), "Repository")
	assert.Equal(t, testRepoName, model.repoName)

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = applyCreateWizardCmd(t, updated, cmd)
	assert.True(t, focusedFieldIsFiltering(model.form))

	selectField, ok := model.form.GetFocusedField().(*huh.Select[string])
	require.True(t, ok)
	assert.True(t, selectField.GetFiltering())
}

func TestCreateWizardOpenWithoutWorktreesShowsError(t *testing.T) {
	model := driveCreateWizard(t,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	assert.Contains(t, model.View(), "no managed worktrees")
	assert.Equal(t, createWizardWorktreeFieldKey, model.form.GetFocusedField().GetKey())
	assert.False(t, model.cancelled)
}

func TestRequireWorktreeName(t *testing.T) {
	require.EqualError(t, requireWorktreeName(""), "name is required")
	assert.NoError(t, requireWorktreeName("feature/login"))
}

func TestRequireExistingWorktree(t *testing.T) {
	require.EqualError(t, requireExistingWorktree(""), "no managed worktrees")
	assert.NoError(t, requireExistingWorktree("feature/login@"+testRepoName))
}

func TestSplitWizardWorktreeValue(t *testing.T) {
	name, repo, err := splitWizardWorktreeValue("feature/login@timber")
	require.NoError(t, err)
	assert.Equal(t, "feature/login", name)
	assert.Equal(t, "timber", repo)

	_, _, err = splitWizardWorktreeValue("")
	require.EqualError(t, err, "invalid worktree selection")
}

func driveCreateWizard(t *testing.T, messages ...tea.Msg) *createWizardModel {
	t.Helper()
	return driveCreateWizardWith(t, []registeredRepo{{Name: testRepoName}}, nil, messages...)
}

func driveCreateWizardWith(
	t *testing.T,
	repos []registeredRepo,
	worktrees []managedWorktree,
	messages ...tea.Msg,
) *createWizardModel {
	t.Helper()

	model := newCreateWizardModel(repos, worktrees)
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
	return applyCreateWizardCmdBudget(t, model, cmd, 32)
}

func applyCreateWizardCmdBudget(t *testing.T, model tea.Model, cmd tea.Cmd, budget int) *createWizardModel {
	t.Helper()

	result, ok := model.(*createWizardModel)
	require.True(t, ok)
	if budget <= 0 || cmd == nil {
		return result
	}

	for _, message := range flattenImmediateCmds(cmd) {
		if message == nil {
			continue
		}
		if _, ok := message.(tea.QuitMsg); ok {
			continue
		}
		updated, next := result.Update(message)
		result = applyCreateWizardCmdBudget(t, updated, next, budget-1)
	}
	return result
}

func flattenImmediateCmds(cmd tea.Cmd) []tea.Msg {
	message, ok := runCmdNonBlocking(cmd)
	if !ok || message == nil {
		return nil
	}
	batch, isBatch := message.(tea.BatchMsg)
	if !isBatch {
		return []tea.Msg{message}
	}

	messages := make([]tea.Msg, 0, len(batch))
	for _, nested := range batch {
		if nested == nil {
			continue
		}
		messages = append(messages, flattenImmediateCmds(nested)...)
	}
	return messages
}

func runCmdNonBlocking(cmd tea.Cmd) (tea.Msg, bool) {
	if cmd == nil {
		return nil, false
	}

	done := make(chan tea.Msg, 1)
	go func() {
		done <- cmd()
	}()
	select {
	case message := <-done:
		return message, true
	case <-time.After(20 * time.Millisecond):
		return nil, false
	}
}
