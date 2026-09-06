package timber

import (
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

const (
	wizardActionCreate = "create"
	wizardActionOpen   = "open"

	tuiWorktreeInputMaxLength = 72
	tuiWorktreeTitle          = "Open or Create Worktree"
	tuiWorktreeLegend         = "tab/↓ next • shift+tab/↑ prev • enter accept • esc/^c quit • worktree@repo creates"
)

var (
	tuiWorktreeTitleStyle    = lipgloss.NewStyle().Bold(true)
	tuiWorktreeLegendStyle   = lipgloss.NewStyle().Faint(true)
	tuiWorktreeInputBoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	tuiWorktreeViewStyle     = lipgloss.NewStyle().Padding(1, 2)
)

type createWizardSelection struct {
	action       string
	repoName     string
	worktreeName string
	cancelled    bool
}

type createWizardPrompter interface {
	Prompt(io.Reader, io.Writer, []registeredRepo, []managedWorktree, bool) (createWizardSelection, error)
}

type bubbleteaCreateWizardPrompter struct {
	interactive func() bool
}

type tuiCreateCommandOptions struct {
	runtime  Runtime
	herdr    bool
	noHerdr  bool
	noTitle  bool
	prompter createWizardPrompter
}

func NewTUICommand(runtime Runtime) *cobra.Command {
	options := &tuiCreateCommandOptions{runtime: runtime}
	options.prompter = bubbleteaCreateWizardPrompter{}

	command := &cobra.Command{
		Use:   "tui",
		Short: "Interactively create a worktree or open an existing one",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	command.Flags().BoolVar(&options.herdr, "herdr", false, "Also create a Herdr workspace for a new worktree")
	command.Flags().BoolVar(&options.noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.Flags().BoolVar(&options.noTitle, "no-title", false, "Hide the title header")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	return command
}

func (x *tuiCreateCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := x.runtime.listRegisteredReposForWizard()
	if err != nil {
		return err
	}

	worktrees, err := x.runtime.listWorktreesForWizard(repos)
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

func (x *tuiCreateCommandOptions) promptSelection(
	command *cobra.Command,
	repos []registeredRepo,
	worktrees []managedWorktree,
) (createWizardSelection, error) {
	return x.prompter.Prompt(command.InOrStdin(), command.ErrOrStderr(), repos, worktrees, !x.noTitle)
}

func (x *tuiCreateCommandOptions) createSelectedWorktree(
	command *cobra.Command,
	selection createWizardSelection,
) error {
	createOptions := &createCommandOptions{runtime: x.runtime}
	createOptions.RepoName = selection.repoName
	createOptions.herdr = x.herdr
	createOptions.noHerdr = x.noHerdr
	worktreePath, err := createOptions.createWorktree(command, []string{selection.worktreeName})
	if err != nil {
		return err
	}
	return x.runtime.reportCreatedWorktreePath(command, worktreePath)
}

func (x *tuiCreateCommandOptions) openSelectedWorktree(
	command *cobra.Command,
	selection createWizardSelection,
) error {
	worktree := managedWorktree{
		Repo: selection.repoName,
		Name: selection.worktreeName,
		Path: x.runtime.managedWorktreePath(selection.repoName, selection.worktreeName),
	}
	if err := x.runtime.openHerdrSpace(command.Context(), worktree); err != nil {
		return err
	}
	return reportOpenedHerdrSpace(command, worktree.Name)
}

func (x bubbleteaCreateWizardPrompter) Prompt(
	input io.Reader,
	output io.Writer,
	repos []registeredRepo,
	worktrees []managedWorktree,
	showTitle bool,
) (createWizardSelection, error) {
	if !x.terminalIsInteractive(input) {
		return createWizardSelection{}, errors.New("tui requires an interactive terminal")
	}

	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if input != nil {
		programOptions = append(programOptions, tea.WithInput(input))
	}
	if output != nil {
		programOptions = append(programOptions, tea.WithOutput(output))
	}
	finalModel, err := tea.NewProgram(newCreateWizardModel(repos, worktrees, showTitle), programOptions...).Run()
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
	return result.selection, nil
}

func (x bubbleteaCreateWizardPrompter) terminalIsInteractive(input io.Reader) bool {
	if x.interactive != nil {
		return x.interactive()
	}
	return isInteractiveTerminal(input)
}
