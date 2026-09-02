package timber

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

const (
	createWizardActionFieldKey   = "action"
	createWizardRepoFieldKey     = "repository"
	createWizardNameFieldKey     = "worktree-name"
	createWizardWorktreeFieldKey = "worktree"

	wizardActionCreate = "create"
	wizardActionOpen   = "open"
)

type createWizardSelection struct {
	action       string
	repoName     string
	worktreeName string
	cancelled    bool
}

type createWizardPrompter interface {
	Prompt(io.Reader, io.Writer, []registeredRepo, []managedWorktree) (createWizardSelection, error)
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
	form             *huh.Form
	action           string
	repoName         string
	worktreeName     string
	existingWorktree string
	navigatingBack   bool
	cancelled        bool
}

func NewTUICommand() *cobra.Command {
	options := new(tuiCreateCommandOptions)
	options.prompter = huhCreateWizardPrompter{}

	command := &cobra.Command{
		Use:   "tui",
		Short: "Interactively create a worktree or open an existing one",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	command.Flags().BoolVar(&options.herdr, "herdr", false, "Also create a Herdr workspace for a new worktree")
	command.Flags().BoolVar(&options.noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	return command
}

func (x *tuiCreateCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := listRegisteredReposForWizard()
	if err != nil {
		return err
	}

	worktrees, err := listWorktreesForWizard(repos)
	if err != nil {
		return err
	}

	selection, err := x.promptSelection(command, repos, worktrees)
	if err != nil || selection.cancelled {
		return err
	}

	if selection.action == wizardActionOpen {
		return x.openSelectedWorktree(command, selection)
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

func listWorktreesForWizard(repos []registeredRepo) ([]managedWorktree, error) {
	return collectWorktrees(repos, func(_ *Repository, worktree managedWorktree) (managedWorktree, error) {
		return worktree, nil
	})
}

func (x *tuiCreateCommandOptions) promptSelection(
	command *cobra.Command,
	repos []registeredRepo,
	worktrees []managedWorktree,
) (createWizardSelection, error) {
	selection, err := x.prompter.Prompt(command.InOrStdin(), command.ErrOrStderr(), repos, worktrees)
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

func (x *tuiCreateCommandOptions) openSelectedWorktree(
	command *cobra.Command,
	selection createWizardSelection,
) error {
	worktree := managedWorktree{
		Repo: selection.repoName,
		Name: selection.worktreeName,
		Path: managedWorktreePath(selection.repoName, selection.worktreeName),
	}
	if err := openHerdrSpace(command.Context(), worktree); err != nil {
		return err
	}
	return reportOpenedHerdrSpace(command, worktree.Name)
}

func (x huhCreateWizardPrompter) Prompt(
	input io.Reader,
	output io.Writer,
	repos []registeredRepo,
	worktrees []managedWorktree,
) (createWizardSelection, error) {
	if !x.terminalIsInteractive(input) {
		return createWizardSelection{}, errors.New("tui requires an interactive terminal")
	}

	programOptions := make([]tea.ProgramOption, 0, 2)
	if input != nil {
		programOptions = append(programOptions, tea.WithInput(input))
	}
	if output != nil {
		programOptions = append(programOptions, tea.WithOutput(output))
	}
	finalModel, err := tea.NewProgram(newCreateWizardModel(repos, worktrees), programOptions...).Run()
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
	return result.selection()
}

func (x huhCreateWizardPrompter) terminalIsInteractive(input io.Reader) bool {
	if x.interactive != nil {
		return x.interactive()
	}
	return isInteractiveTerminal(input)
}

func newCreateWizardModel(repos []registeredRepo, worktrees []managedWorktree) *createWizardModel {
	model := new(createWizardModel)
	model.form = newCreateWizardForm(model, repos, worktrees)
	return model
}

func (m *createWizardModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m *createWizardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := message.(tea.KeyMsg); ok {
		filtering := focusedFieldIsFiltering(m.form)
		if isCreateWizardEscKey(keyMsg) && !filtering && !m.canGoBack() {
			m.cancelled = true
			return m, tea.Quit
		}
		m.navigatingBack = !filtering && isCreateWizardBackKey(keyMsg)
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
	return m.form.GetFocusedField().GetKey() != createWizardActionFieldKey
}

func (m *createWizardModel) validateWorktreeName(value string) error {
	if m.navigatingBack {
		return nil
	}
	return requireWorktreeName(value)
}

func (m *createWizardModel) validateExistingWorktree(value string) error {
	if m.navigatingBack {
		return nil
	}
	return requireExistingWorktree(value)
}

func (m *createWizardModel) selection() (createWizardSelection, error) {
	if m.action != wizardActionOpen {
		return createWizardSelection{
			action:       wizardActionCreate,
			repoName:     m.repoName,
			worktreeName: m.worktreeName,
		}, nil
	}

	worktreeName, repoName, err := splitWizardWorktreeValue(m.existingWorktree)
	if err != nil {
		return createWizardSelection{}, err
	}
	return createWizardSelection{
		action:       wizardActionOpen,
		repoName:     repoName,
		worktreeName: worktreeName,
	}, nil
}

func newCreateWizardForm(
	model *createWizardModel,
	repos []registeredRepo,
	worktrees []managedWorktree,
) *huh.Form {
	repoOptions := lo.Map(repos, func(repo registeredRepo, _ int) huh.Option[string] {
		return huh.NewOption(repo.Name, repo.Name)
	})
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Action").
				Key(createWizardActionFieldKey).
				Options(
					huh.NewOption("Create worktree", wizardActionCreate),
					huh.NewOption("Open existing worktree", wizardActionOpen),
				).
				Value(&model.action),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Repository").
				Key(createWizardRepoFieldKey).
				Options(repoOptions...).
				Value(&model.repoName),
			huh.NewInput().
				Title("Worktree name").
				Key(createWizardNameFieldKey).
				Value(&model.worktreeName).
				Validate(model.validateWorktreeName),
		).WithHideFunc(func() bool {
			return model.action == wizardActionOpen
		}),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Worktree").
				Key(createWizardWorktreeFieldKey).
				Options(wizardWorktreeOptions(worktrees)...).
				Value(&model.existingWorktree).
				Validate(model.validateExistingWorktree),
		).WithHideFunc(func() bool {
			return model.action != wizardActionOpen
		}),
	).WithKeyMap(newCreateWizardKeyMap())
}

func wizardWorktreeOptions(worktrees []managedWorktree) []huh.Option[string] {
	if len(worktrees) == 0 {
		return []huh.Option[string]{huh.NewOption("No managed worktrees", "")}
	}
	return lo.Map(worktrees, func(worktree managedWorktree, _ int) huh.Option[string] {
		return huh.NewOption(worktree.Name+" ("+worktree.Repo+")", wizardWorktreeValue(worktree))
	})
}

func wizardWorktreeValue(worktree managedWorktree) string {
	return worktree.Name + "@" + worktree.Repo
}

func splitWizardWorktreeValue(value string) (string, string, error) {
	at := strings.LastIndex(value, "@")
	if at <= 0 || at == len(value)-1 {
		return "", "", fmt.Errorf("invalid worktree selection")
	}
	return value[:at], value[at+1:], nil
}

func newCreateWizardKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	previous := key.NewBinding(
		key.WithKeys("shift+tab", "esc"),
		key.WithHelp("esc", "back"),
	)
	keymap.Input.Prev = previous
	keymap.Select.Prev = previous
	return keymap
}

func focusedFieldIsFiltering(form *huh.Form) bool {
	type filteringField interface {
		GetFiltering() bool
	}
	field, ok := form.GetFocusedField().(filteringField)
	return ok && field.GetFiltering()
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

func requireExistingWorktree(value string) error {
	if value == "" {
		return fmt.Errorf("no managed worktrees")
	}
	return nil
}
