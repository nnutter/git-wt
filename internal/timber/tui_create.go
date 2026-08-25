package timber

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

const createWizardNameFieldKey = "worktree-name"

type createWizardSelection struct {
	repoName     string
	worktreeName string
	cancelled    bool
}

type createWizardPrompter interface {
	Prompt(io.Reader, io.Writer, []registeredRepo) (createWizardSelection, error)
}

type huhCreateWizardPrompter struct {
	interactive func() bool
}

type tuiCreateCommandOptions struct {
	herdr    bool
	noHerdr  bool
	prompter createWizardPrompter
}

type createWizardModel struct {
	form           *huh.Form
	repoName       string
	worktreeName   string
	navigatingBack bool
	cancelled      bool
}

func NewTUICreateCommand() *cobra.Command {
	options := new(tuiCreateCommandOptions)
	options.prompter = huhCreateWizardPrompter{}

	command := &cobra.Command{
		Use:   "create",
		Short: "Interactively create a managed Git worktree",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	command.Flags().BoolVar(&options.herdr, "herdr", false, "Also create a Herdr workspace for the new worktree")
	command.Flags().BoolVar(&options.noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	return command
}

func (x *tuiCreateCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := listRegisteredReposForWizard()
	if err != nil {
		return err
	}

	selection, err := x.promptSelection(command, repos)
	if err != nil || selection.cancelled {
		return err
	}

	return x.createSelectedWorktree(command, selection)
}

func listRegisteredReposForWizard() ([]registeredRepo, error) {
	repos, err := listRegisteredRepos()
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, errors.New("no registered repositories; run timber repo add first")
	}
	return repos, nil
}

func (x *tuiCreateCommandOptions) promptSelection(
	command *cobra.Command,
	repos []registeredRepo,
) (createWizardSelection, error) {
	selection, err := x.prompter.Prompt(command.InOrStdin(), command.ErrOrStderr(), repos)
	if err == nil {
		return selection, nil
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return createWizardSelection{cancelled: true}, nil
	}
	return createWizardSelection{}, err
}

func (x *tuiCreateCommandOptions) createSelectedWorktree(
	command *cobra.Command,
	selection createWizardSelection,
) error {
	createOptions := new(createCommandOptions)
	createOptions.RepoName = selection.repoName
	createOptions.herdr = x.herdr
	createOptions.noHerdr = x.noHerdr
	worktreePath, err := createOptions.createWorktree(command, []string{selection.worktreeName})
	if err != nil {
		return err
	}
	return reportCreatedWorktreePath(command, worktreePath)
}

func (x huhCreateWizardPrompter) Prompt(
	input io.Reader,
	output io.Writer,
	repos []registeredRepo,
) (createWizardSelection, error) {
	if !x.terminalIsInteractive() {
		return createWizardSelection{}, errors.New("tui create requires an interactive terminal")
	}

	programOptions := make([]tea.ProgramOption, 0, 2)
	if input != nil {
		programOptions = append(programOptions, tea.WithInput(input))
	}
	if output != nil {
		programOptions = append(programOptions, tea.WithOutput(output))
	}
	finalModel, err := tea.NewProgram(newCreateWizardModel(repos), programOptions...).Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return createWizardSelection{cancelled: true}, nil
		}
		return createWizardSelection{}, err
	}

	result, ok := finalModel.(*createWizardModel)
	if !ok {
		return createWizardSelection{}, errors.New("create wizard returned unexpected model")
	}
	if result.cancelled {
		return createWizardSelection{cancelled: true}, nil
	}
	return createWizardSelection{repoName: result.repoName, worktreeName: result.worktreeName}, nil
}

func (x huhCreateWizardPrompter) terminalIsInteractive() bool {
	if x.interactive != nil {
		return x.interactive()
	}
	return isInteractiveTerminal()
}

func newCreateWizardModel(repos []registeredRepo) *createWizardModel {
	model := new(createWizardModel)
	model.form = newCreateWizardForm(&model.repoName, &model.worktreeName, repos, model.validateWorktreeName)
	return model
}

func (m *createWizardModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m *createWizardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := message.(tea.KeyMsg); ok {
		if isCreateWizardEscKey(keyMsg) && !m.canGoBack() {
			m.cancelled = true
			return m, tea.Quit
		}
		m.navigatingBack = isCreateWizardBackKey(keyMsg)
	}

	updated, cmd := m.form.Update(message)
	m.form = updated.(*huh.Form)
	switch m.form.State {
	case huh.StateCompleted:
		return m, tea.Quit
	case huh.StateAborted:
		m.cancelled = true
		return m, tea.Quit
	}
	return m, cmd
}

func (m *createWizardModel) View() string {
	return m.form.View()
}

func (m *createWizardModel) canGoBack() bool {
	return m.form.GetFocusedField().GetKey() == createWizardNameFieldKey
}

func (m *createWizardModel) validateWorktreeName(value string) error {
	if m.navigatingBack {
		return nil
	}
	return requireWorktreeName(value)
}

func newCreateWizardForm(
	repoName *string,
	worktreeName *string,
	repos []registeredRepo,
	validateName func(string) error,
) *huh.Form {
	options := lo.Map(repos, func(repo registeredRepo, _ int) huh.Option[string] {
		return huh.NewOption(repo.Name, repo.Name)
	})
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Repository").
				Options(options...).
				Filtering(true).
				Value(repoName),
			huh.NewInput().
				Title("Worktree name").
				Key(createWizardNameFieldKey).
				Value(worktreeName).
				Validate(validateName),
		),
	).WithKeyMap(newCreateWizardKeyMap())
}

func newCreateWizardKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Input.Prev = key.NewBinding(
		key.WithKeys("shift+tab", "esc"),
		key.WithHelp("esc", "back"),
	)
	keymap.Select.Prev = key.NewBinding(
		key.WithKeys("shift+tab", "esc"),
		key.WithHelp("esc", "back"),
	)
	return keymap
}

func isCreateWizardEscKey(message tea.KeyMsg) bool {
	return message.String() == "esc"
}

func isCreateWizardBackKey(message tea.KeyMsg) bool {
	switch message.String() {
	case "esc", "shift+tab":
		return true
	default:
		return false
	}
}

func requireWorktreeName(value string) error {
	if value == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}
