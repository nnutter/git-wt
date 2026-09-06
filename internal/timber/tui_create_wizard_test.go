package timber

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWizardUsesQualifiedWorktreeLabels(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: "timber", Name: "feature/one", DisplayPath: "/home/user/worktrees/feature/one"},
		{Repo: "other", Name: "feature/one"},
	}

	model := driveCreateWizardWith(t, []registeredRepo{{Name: "other"}, {Name: "timber"}}, worktrees)

	assert.Equal(t, []string{
		"feature/one@timber",
		"feature/one@other",
	}, listWorktreeNames(model.list.VisibleItems()))
	view := model.View()
	assert.Equal(t, 80-tuiWorktreeViewStyle.GetHorizontalFrameSize(), model.list.Width())
	assert.Less(t, strings.Index(view, "Open or Create Worktree"), strings.Index(view, "type worktree@repo"))
	assert.Contains(t, view, "╭")
	assert.Contains(t, view, "╯")
	assert.LessOrEqual(t, model.input.Width+tuiWorktreeInputBoxStyle.GetHorizontalFrameSize(), tuiWorktreeInputMaxLength)
	assert.Contains(t, view, "feature/one@timber")
	assert.NotContains(t, view, "/home/user/worktrees/feature/one")
	assert.Contains(t, view, "type worktree@repo")
	assert.Contains(t, view, "tab/↓ next")
	assert.Contains(t, view, "shift+tab/↑ prev")
	assert.Contains(t, view, "enter accept")
	assert.Contains(t, view, "esc/^c quit")
	assert.Contains(t, view, "worktree@repo")
	assert.NotContains(t, view, "q quit")
	assert.False(t, model.list.ShowHelp())
	assert.Equal(t, tuiWorktreeInputMaxLength, model.input.CharLimit)
	assert.Empty(t, model.input.Prompt)
}

func TestCreateWizardNoTitleSuppressesHeader(t *testing.T) {
	t.Parallel()
	model := newCreateWizardModel([]registeredRepo{{Name: testRepoName}}, make([]managedWorktree, 0), false)
	_ = model.Init()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(*createWizardModel)

	view := model.View()

	assert.NotContains(t, view, tuiWorktreeTitle)
	assert.Contains(t, view, "type worktree@repo")
	assert.Contains(t, view, "tab/↓ next")
	assert.Contains(t, view, "esc/^c quit")
}

func TestFilterWizardWorktreesByWorktreeName(t *testing.T) {
	t.Parallel()
	targets := []string{
		"feature/login@timber",
		"feature/login@other",
		"bugfix/login@timber",
	}

	ranks := filterWizardWorktrees("feature", targets)
	assert.Equal(t, []int{0, 1}, rankIndexes(ranks))
}

func TestFilterWizardWorktreesByRepositoryAfterAt(t *testing.T) {
	t.Parallel()
	targets := []string{
		"feature/login@timber",
		"feature/login@other",
		"bugfix/login@timber",
	}

	ranks := filterWizardWorktrees("feature@other", targets)
	assert.Equal(t, []int{1}, rankIndexes(ranks))

	ranks = filterWizardWorktrees("@timber", targets)
	assert.Equal(t, []int{0, 2}, rankIndexes(ranks))
}

func TestCreateWizardFiltersWorktreesAndRepositories(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: "timber", Name: "feature/login"},
		{Repo: "other", Name: "feature/login"},
		{Repo: "timber", Name: "bugfix/login"},
	}
	model := driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: "other"}, {Name: "timber"}},
		worktrees,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature@other")},
	)

	assert.Equal(t, []string{"feature/login@other", "feature@other"}, wizardItemNames(model.list.VisibleItems()))
	assert.Equal(t, "open existing worktree", model.list.VisibleItems()[0].(wizardWorktreeItem).Description())
	assert.Equal(t, "create new worktree", model.list.VisibleItems()[1].(wizardCreateWorktreeItem).Description())
	assert.Equal(t, "feature@other", model.input.Value())
}

func TestCreateWizardOffersNewWorktreesForMatchingRepositories(t *testing.T) {
	t.Parallel()
	repos := []registeredRepo{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}
	worktrees := []managedWorktree{{Repo: "alpha", Name: "feature"}}

	model := driveCreateWizardWith(
		t,
		repos,
		worktrees,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature@")},
	)

	assert.Equal(t, []string{"feature@alpha", "feature@beta", "feature@gamma"}, wizardItemNames(model.list.VisibleItems()))
	assert.Equal(t, "open existing worktree", model.list.VisibleItems()[0].(wizardWorktreeItem).Description())
	assert.Equal(t, "create new worktree", model.list.VisibleItems()[1].(wizardCreateWorktreeItem).Description())
	assert.Equal(t, "create new worktree", model.list.VisibleItems()[2].(wizardCreateWorktreeItem).Description())
}

func TestCreateWizardOffersNewWorktreeForRepoWithoutExistingWorktrees(t *testing.T) {
	t.Parallel()
	model := driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: "alpha"}, {Name: "beta"}},
		make([]managedWorktree, 0),
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature@be")},
	)

	assert.Equal(t, []string{"feature@beta"}, wizardItemNames(model.list.VisibleItems()))
	assert.Equal(t, "create new worktree", model.list.VisibleItems()[0].(wizardCreateWorktreeItem).Description())
}

func TestCreateWizardEnterOpensSelectedWorktree(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: "timber", Name: "feature/one"},
		{Repo: "other", Name: "feature/two"},
	}
	model := driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: "other"}, {Name: "timber"}},
		worktrees,
		tea.KeyMsg{Type: tea.KeyDown},
	)

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*createWizardModel)

	assert.Equal(t, createWizardSelection{
		action:       wizardActionOpen,
		repoName:     "other",
		worktreeName: "feature/two",
	}, model.selection)
	require.NotNil(t, command)
	assert.Equal(t, tea.Quit(), command())
}

func TestCreateWizardEnterCreatesSelectedNewWorktree(t *testing.T) {
	t.Parallel()
	model := driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: "other"}},
		[]managedWorktree{{Repo: "other", Name: "feature/login"}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature@other")},
		tea.KeyMsg{Type: tea.KeyDown},
	)

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*createWizardModel)

	assert.Equal(t, createWizardSelection{
		action:       wizardActionCreate,
		repoName:     "other",
		worktreeName: "feature",
	}, model.selection)
	require.NotNil(t, command)
	assert.Equal(t, tea.Quit(), command())
}

func TestCreateWizardEnterCreatesUnmatchedQualifiedWorktree(t *testing.T) {
	t.Parallel()
	model := driveCreateWizardWith(
		t,
		[]registeredRepo{{Name: testRepoName}},
		[]managedWorktree{{Repo: testRepoName, Name: "feature/existing"}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature/new@" + testRepoName)},
	)

	require.Len(t, model.list.VisibleItems(), 1)
	assert.Equal(t, "create new worktree", model.list.VisibleItems()[0].(wizardCreateWorktreeItem).Description())
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*createWizardModel)

	assert.Equal(t, createWizardSelection{
		action:       wizardActionCreate,
		repoName:     testRepoName,
		worktreeName: "feature/new",
	}, model.selection)
	require.NotNil(t, command)
	assert.Equal(t, tea.Quit(), command())
}

func TestCreateWizardRejectsUnmatchedUnqualifiedWorktree(t *testing.T) {
	t.Parallel()
	model := driveCreateWizard(t, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature/new")})

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*createWizardModel)

	require.Nil(t, command)
	require.EqualError(t, model.err, "invalid worktree selection")
	assert.False(t, model.cancelled)
}

func TestCreateWizardRejectsUnregisteredRepository(t *testing.T) {
	t.Parallel()
	model := driveCreateWizard(t, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature/new@missing")})

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*createWizardModel)

	require.Nil(t, command)
	require.EqualError(t, model.err, `unknown repository "missing"`)
	assert.False(t, model.cancelled)
}

func TestCreateWizardEscCancels(t *testing.T) {
	t.Parallel()
	model := newCreateWizardModel([]registeredRepo{{Name: testRepoName}}, make([]managedWorktree, 0), true)
	_ = model.Init()

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(*createWizardModel)

	assert.True(t, model.cancelled)
	require.NotNil(t, command)
	assert.Equal(t, tea.Quit(), command())
}

func TestCreateWizardDownAndUpNavigateMatches(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: testRepoName, Name: "feature/one"},
		{Repo: testRepoName, Name: "feature/two"},
	}
	model := driveCreateWizardWith(t, []registeredRepo{{Name: testRepoName}}, worktrees)

	assert.Equal(t, "feature/one@"+testRepoName, selectedWizardWorktree(model))

	model = driveCreateWizardWith(t, []registeredRepo{{Name: testRepoName}}, worktrees, tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, "feature/two@"+testRepoName, selectedWizardWorktree(model))

	model = driveCreateWizardWith(t, []registeredRepo{{Name: testRepoName}}, worktrees, tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, "feature/two@"+testRepoName, selectedWizardWorktree(model))

	model = driveCreateWizardWith(t, []registeredRepo{{Name: testRepoName}}, worktrees, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, "feature/one@"+testRepoName, selectedWizardWorktree(model))

	model = driveCreateWizardWith(t, []registeredRepo{{Name: testRepoName}}, worktrees, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, "feature/one@"+testRepoName, selectedWizardWorktree(model))
}

func TestSplitWizardWorktreeValue(t *testing.T) {
	t.Parallel()
	name, repo, err := splitWizardWorktreeValue("feature/login@timber")
	require.NoError(t, err)
	assert.Equal(t, "feature/login", name)
	assert.Equal(t, "timber", repo)

	name, repo, err = splitWizardWorktreeValue("feature@nested@timber")
	require.NoError(t, err)
	assert.Equal(t, "feature@nested", name)
	assert.Equal(t, "timber", repo)

	_, _, err = splitWizardWorktreeValue("")
	require.EqualError(t, err, "invalid worktree selection")
}

func driveCreateWizard(t *testing.T, messages ...tea.Msg) *createWizardModel {
	t.Helper()
	return driveCreateWizardWith(t, []registeredRepo{{Name: testRepoName}}, make([]managedWorktree, 0), messages...)
}

func driveCreateWizardWith(
	t *testing.T,
	repos []registeredRepo,
	worktrees []managedWorktree,
	messages ...tea.Msg,
) *createWizardModel {
	t.Helper()

	model := newCreateWizardModel(repos, worktrees, true)
	_ = model.Init()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(*createWizardModel)
	for _, message := range messages {
		next, command := model.Update(message)
		model = applyCreateWizardCmd(t, next, command)
	}
	return model
}

func applyCreateWizardCmd(t *testing.T, model tea.Model, command tea.Cmd) *createWizardModel {
	t.Helper()
	return applyCreateWizardCmdBudget(t, model, command, 32)
}

func applyCreateWizardCmdBudget(t *testing.T, model tea.Model, command tea.Cmd, budget int) *createWizardModel {
	t.Helper()

	result, ok := model.(*createWizardModel)
	require.True(t, ok)
	if budget <= 0 || command == nil {
		return result
	}

	for _, message := range flattenImmediateCmds(command) {
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

func flattenImmediateCmds(command tea.Cmd) []tea.Msg {
	message, ok := runCmdNonBlocking(command)
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

func runCmdNonBlocking(command tea.Cmd) (tea.Msg, bool) {
	if command == nil {
		return nil, false
	}

	done := make(chan tea.Msg, 1)
	go func() {
		done <- command()
	}()
	select {
	case message := <-done:
		return message, true
	case <-time.After(20 * time.Millisecond):
		return nil, false
	}
}

func wizardItemNames(items []list.Item) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		titledItem, ok := item.(interface{ Title() string })
		if !ok {
			continue
		}
		result = append(result, titledItem.Title())
	}
	return result
}

func listWorktreeNames(items []list.Item) []string {
	return wizardItemNames(items)
}

func rankIndexes(ranks []list.Rank) []int {
	result := make([]int, 0, len(ranks))
	for _, rank := range ranks {
		result = append(result, rank.Index)
	}
	return result
}

func selectedWizardWorktree(model *createWizardModel) string {
	item, ok := model.list.SelectedItem().(wizardWorktreeItem)
	if !ok {
		return ""
	}
	return item.Title()
}
