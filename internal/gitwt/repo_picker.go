package gitwt

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type repoPrompter interface {
	Prompt(repos []registeredRepo) (registeredRepo, error)
}

type bubbleteaRepoPrompter struct {
	input  io.Reader
	output io.Writer
}

func (x bubbleteaRepoPrompter) Prompt(repos []registeredRepo) (registeredRepo, error) {
	items := make([]list.Item, 0, len(repos))
	for _, repo := range repos {
		items = append(items, repoListItem{repo: repo})
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false

	repoList := list.New(items, delegate, 0, 0)
	repoList.Title = "Select repository"
	repoList.SetShowStatusBar(false)
	repoList.SetFilteringEnabled(true)
	repoList.Styles.Title = lipgloss.NewStyle().Bold(true)

	model := repoPickerModel{list: repoList}
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if x.input != nil {
		programOptions = append(programOptions, tea.WithInput(x.input))
	}
	if x.output != nil {
		programOptions = append(programOptions, tea.WithOutput(x.output))
	}

	program := tea.NewProgram(model, programOptions...)
	finalModel, err := program.Run()
	if err != nil {
		return registeredRepo{}, fmt.Errorf("repository picker: %w", err)
	}

	result, ok := finalModel.(repoPickerModel)
	if !ok {
		return registeredRepo{}, errors.New("repository picker returned unexpected model")
	}
	if result.cancelled || result.choice.Name == "" {
		return registeredRepo{}, errors.New("repository selection cancelled")
	}
	return result.choice, nil
}

type repoListItem struct {
	repo registeredRepo
}

func (x repoListItem) FilterValue() string { return x.repo.Name }
func (x repoListItem) Title() string       { return x.repo.Name }
func (x repoListItem) Description() string { return x.repo.BarePath }

type repoPickerModel struct {
	list      list.Model
	choice    registeredRepo
	cancelled bool
}

func (x repoPickerModel) Init() tea.Cmd {
	return nil
}

func (x repoPickerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		x.list.SetSize(message.Width, message.Height)
		return x, nil
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "esc", "q":
			if x.list.FilterState() != list.Filtering {
				x.cancelled = true
				return x, tea.Quit
			}
		case "enter":
			if x.list.FilterState() != list.Filtering {
				item, ok := x.list.SelectedItem().(repoListItem)
				if ok {
					x.choice = item.repo
				}
				return x, tea.Quit
			}
		}
	}

	var command tea.Cmd
	x.list, command = x.list.Update(message)
	return x, command
}

func (x repoPickerModel) View() string {
	return x.list.View()
}
