package timber

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type createWizardModel struct {
	repos     []registeredRepo
	worktrees []managedWorktree
	showTitle bool
	input     textinput.Model
	list      list.Model
	selection createWizardSelection
	err       error
	cancelled bool
}

type wizardWorktreeItem struct {
	worktree managedWorktree
}

func (x wizardWorktreeItem) FilterValue() string {
	return wizardWorktreeValue(x.worktree)
}

func (x wizardWorktreeItem) Title() string {
	return wizardWorktreeValue(x.worktree)
}

func (x wizardWorktreeItem) Description() string {
	return "open existing worktree"
}

type wizardCreateWorktreeItem struct {
	worktreeName string
	repoName     string
}

func (x wizardCreateWorktreeItem) FilterValue() string {
	return x.Title()
}

func (x wizardCreateWorktreeItem) Title() string {
	return x.worktreeName + "@" + x.repoName
}

func (x wizardCreateWorktreeItem) Description() string {
	return "create new worktree"
}

func newCreateWizardModel(repos []registeredRepo, worktrees []managedWorktree, showTitle bool) *createWizardModel {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "type worktree@repo"
	input.CharLimit = tuiWorktreeInputMaxLength
	input.Focus()

	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Bold(true)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Italic(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Italic(true)

	worktreeList := list.New(wizardItemsForTerm(repos, worktrees, ""), delegate, 0, 0)
	worktreeList.SetShowTitle(false)
	worktreeList.SetShowFilter(false)
	worktreeList.SetShowStatusBar(false)
	worktreeList.SetShowHelp(false)
	worktreeList.SetShowPagination(false)
	worktreeList.SetFilteringEnabled(false)

	return &createWizardModel{
		repos:     repos,
		worktrees: worktrees,
		showTitle: showTitle,
		input:     input,
		list:      worktreeList,
	}
}

func (m *createWizardModel) Init() tea.Cmd {
	return m.input.Focus()
}

func (m *createWizardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if windowSize, ok := message.(tea.WindowSizeMsg); ok {
		viewFrameWidth := tuiWorktreeViewStyle.GetHorizontalFrameSize()
		viewFrameHeight := tuiWorktreeViewStyle.GetVerticalFrameSize()
		boxFrameWidth := tuiWorktreeInputBoxStyle.GetHorizontalFrameSize()
		inputWidth := min(windowSize.Width-viewFrameWidth-boxFrameWidth, tuiWorktreeInputMaxLength-boxFrameWidth)
		m.input.Width = max(inputWidth, 1)
		m.list.SetSize(
			max(windowSize.Width-viewFrameWidth, 1),
			max(windowSize.Height-viewFrameHeight-8, 1),
		)
		return m, nil
	}

	if keyMsg, ok := message.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			selection, err := m.selectionForEnter()
			if err != nil {
				m.err = err
				return m, nil
			}
			m.selection = selection
			return m, tea.Quit
		case "up", "shift+tab":
			m.err = nil
			m.list.CursorUp()
			return m, nil
		case "down", "tab":
			m.err = nil
			m.list.CursorDown()
			return m, nil
		default:
			m.err = nil
		}
	}

	previousValue := m.input.Value()
	var command tea.Cmd
	m.input, command = m.input.Update(message)
	if m.input.Value() != previousValue {
		m.list.SetItems(wizardItemsForTerm(m.repos, m.worktrees, m.input.Value()))
		m.list.ResetSelected()
	}
	return m, command
}

func (m *createWizardModel) View() string {
	view := ""
	if m.showTitle {
		view += tuiWorktreeTitleStyle.Render(tuiWorktreeTitle) + "\n"
	}
	view += tuiWorktreeInputBoxStyle.Render(m.input.View()) + "\n" +
		m.list.View()
	if m.err != nil {
		view += "\n" + warningStyle.Render(m.err.Error())
	}
	return tuiWorktreeViewStyle.Render(view + "\n" + tuiWorktreeLegendStyle.Render(tuiWorktreeLegend))
}

func (m *createWizardModel) selectionForEnter() (createWizardSelection, error) {
	switch item := m.list.SelectedItem().(type) {
	case wizardWorktreeItem:
		return createWizardSelection{
			action:       wizardActionOpen,
			repoName:     item.worktree.Repo,
			worktreeName: item.worktree.Name,
		}, nil
	case wizardCreateWorktreeItem:
		return createWizardSelection{
			action:       wizardActionCreate,
			repoName:     item.repoName,
			worktreeName: item.worktreeName,
		}, nil
	}

	worktreeName, repoName, err := splitWizardWorktreeValue(m.input.Value())
	if err != nil {
		return createWizardSelection{}, err
	}
	if err := rejectAtInWorktreeName(worktreeName); err != nil {
		return createWizardSelection{}, err
	}
	if !wizardHasRepository(m.repos, repoName) {
		return createWizardSelection{}, fmt.Errorf("unknown repository %q", repoName)
	}
	return createWizardSelection{
		action:       wizardActionCreate,
		repoName:     repoName,
		worktreeName: worktreeName,
	}, nil
}

func wizardItemsForTerm(repos []registeredRepo, worktrees []managedWorktree, term string) []list.Item {
	worktreeValues := make([]string, len(worktrees))
	for index, worktree := range worktrees {
		worktreeValues[index] = wizardWorktreeValue(worktree)
	}

	ranks := filterWizardWorktrees(term, worktreeValues)
	items := make([]list.Item, 0, len(ranks)+len(repos))
	for _, rank := range ranks {
		items = append(items, wizardWorktreeItem{worktree: worktrees[rank.Index]})
	}

	worktreeName, repoTerm, qualified := strings.CutLast(term, "@")
	if !qualified || worktreeName == "" || strings.Contains(worktreeName, "@") {
		return items
	}

	repoNames := make([]string, len(repos))
	for index, repo := range repos {
		repoNames[index] = repo.Name
	}
	for _, rank := range filterWizardTerm(repoTerm, repoNames) {
		repo := repos[rank.Index]
		if wizardHasWorktree(worktrees, worktreeName, repo.Name) {
			continue
		}
		items = append(items, wizardCreateWorktreeItem{
			worktreeName: worktreeName,
			repoName:     repo.Name,
		})
	}
	return items
}

func wizardHasWorktree(worktrees []managedWorktree, worktreeName string, repoName string) bool {
	for _, worktree := range worktrees {
		if worktree.Name == worktreeName && worktree.Repo == repoName {
			return true
		}
	}
	return false
}

func wizardHasRepository(repos []registeredRepo, name string) bool {
	for _, repo := range repos {
		if repo.Name == name {
			return true
		}
	}
	return false
}

func filterWizardWorktrees(term string, targets []string) []list.Rank {
	worktreeTerm, repoTerm, qualified := strings.CutLast(term, "@")
	worktreeTargets := make([]string, len(targets))
	repoTargets := make([]string, len(targets))
	for index, target := range targets {
		worktreeTargets[index], repoTargets[index], _ = strings.CutLast(target, "@")
	}

	worktreeRanks := filterWizardTerm(worktreeTerm, worktreeTargets)
	if !qualified {
		return worktreeRanks
	}

	repoRanks := filterWizardTerm(repoTerm, repoTargets)
	repoMatches := make(map[int]list.Rank, len(repoRanks))
	for _, rank := range repoRanks {
		repoMatches[rank.Index] = rank
	}

	matches := make([]list.Rank, 0, len(worktreeRanks))
	for _, rank := range worktreeRanks {
		repoRank, ok := repoMatches[rank.Index]
		if !ok {
			continue
		}

		matchedIndexes := make([]int, 0, len(rank.MatchedIndexes)+len(repoRank.MatchedIndexes))
		matchedIndexes = append(matchedIndexes, rank.MatchedIndexes...)
		for _, index := range repoRank.MatchedIndexes {
			matchedIndexes = append(matchedIndexes, index+len(worktreeTargets[rank.Index])+1)
		}
		matches = append(matches, list.Rank{Index: rank.Index, MatchedIndexes: matchedIndexes})
	}
	return matches
}

func filterWizardTerm(term string, targets []string) []list.Rank {
	if term == "" {
		ranks := make([]list.Rank, len(targets))
		for index := range targets {
			ranks[index] = list.Rank{Index: index}
		}
		return ranks
	}
	return list.DefaultFilter(term, targets)
}

func wizardWorktreeValue(worktree managedWorktree) string {
	return worktree.Name + "@" + worktree.Repo
}

func splitWizardWorktreeValue(value string) (string, string, error) {
	worktreeName, repoName, found := strings.CutLast(value, "@")
	if !found || worktreeName == "" || repoName == "" {
		return "", "", fmt.Errorf("invalid worktree selection")
	}
	return worktreeName, repoName, nil
}
