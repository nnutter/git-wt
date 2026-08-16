package gitwt

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

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
		return nil, errors.New("no registered repositories; run git-wt repo add first")
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
	createOptions.RepoFlag = selection.repoName
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

	var repoName string
	var worktreeName string
	form := newCreateWizardForm(&repoName, &worktreeName, repos).
		WithInput(input).
		WithOutput(output)
	if err := form.Run(); err != nil {
		return createWizardSelection{}, err
	}

	return createWizardSelection{repoName: repoName, worktreeName: worktreeName}, nil
}

func (x huhCreateWizardPrompter) terminalIsInteractive() bool {
	if x.interactive != nil {
		return x.interactive()
	}
	return isInteractiveTerminal()
}

func newCreateWizardForm(repoName *string, worktreeName *string, repos []registeredRepo) *huh.Form {
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
				Value(worktreeName).
				Validate(requireWorktreeName),
		),
	)
}

func requireWorktreeName(value string) error {
	if value == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}
