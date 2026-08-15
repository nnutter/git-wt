package gitwt

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type createCommandOptions struct {
	repoSelection
	upstream string
	herdr    bool
	noHerdr  bool
}

func NewCreateCommand() *cobra.Command {
	options := new(createCommandOptions)

	command := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a managed Git worktree",
		Args:  cobra.MaximumNArgs(1),
		RunE:  options.Execute,
	}

	options.addRepoFlag(command)
	command.Flags().StringVarP(&options.upstream, "upstream", "u", "", "Upstream branch")
	command.Flags().BoolVar(&options.herdr, "herdr", false, "Also create a Herdr workspace for the new worktree")
	command.Flags().BoolVar(&options.noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")

	return command
}

func (x *createCommandOptions) Execute(command *cobra.Command, args []string) error {
	worktreePath, err := x.createWorktree(command, args)
	if err != nil {
		return err
	}
	return reportCreatedWorktreePath(command, worktreePath)
}

func (x *createCommandOptions) createWorktree(command *cobra.Command, args []string) (string, error) {
	repo, repository, err := x.resolve()
	if err != nil {
		return "", err
	}

	branchName, err := resolveCreateWorktreeName(repo.Name, args)
	if err != nil {
		return "", err
	}

	worktreePath := managedWorktreePath(repo.Name, branchName)
	if _, err := os.Stat(worktreePath); err == nil {
		return "", fmt.Errorf("worktree directory %q already exists", worktreePath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}

	upstreamBranch := x.upstream
	if upstreamBranch == "" {
		resolvedUpstream, err := repository.remoteHeadBranch()
		if err != nil {
			return "", err
		}
		upstreamBranch = resolvedUpstream
	}

	branchExists, err := repository.branchExists(branchName)
	if err != nil {
		return "", err
	}
	if err := ensureWorktreeDirectory(worktreePath); err != nil {
		return "", err
	}
	if branchExists {
		if _, err := repository.git("worktree", "add", worktreePath, branchName); err != nil {
			return "", err
		}
	} else {
		if _, err := repository.git("worktree", "add", "-b", branchName, worktreePath, upstreamBranch); err != nil {
			return "", err
		}
	}

	if err := setBranchUpstream(repository, branchName, upstreamBranch); err != nil {
		return "", err
	}

	if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("created "+worktreePath)); err != nil {
		return "", err
	}

	if !x.shouldCreateHerdrWorkspace() {
		return worktreePath, nil
	}

	worktree := managedWorktree{
		Repo: repo.Name,
		Name: branchName,
		Path: worktreePath,
	}
	if err := openHerdrSpace(command.Context(), worktree); err != nil {
		return "", err
	}
	if err := reportOpenedHerdrSpace(command, branchName); err != nil {
		return "", err
	}
	return worktreePath, nil
}

func resolveCreateWorktreeName(repoName string, args []string) (string, error) {
	if len(args) == 1 && args[0] != "" {
		return args[0], nil
	}
	return unusedWorktreeName(repoName)
}

func (x *createCommandOptions) shouldCreateHerdrWorkspace() bool {
	return x.herdr || (!x.noHerdr && runningInHerdr())
}

const createPathFileEnvVarName = "GIT_WT_CREATE_PATH_FILE"

func reportCreatedWorktreePath(command *cobra.Command, worktreePath string) error {
	if pathFile := os.Getenv(createPathFileEnvVarName); pathFile != "" {
		if err := os.WriteFile(pathFile, []byte(worktreePath+"\n"), 0o600); err != nil {
			return fmt.Errorf("write created worktree path file: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(command.OutOrStdout(), worktreePath)
	return err
}

func setBranchUpstream(repository *Repository, branchName string, upstreamBranch string) error {
	// Local start points (e.g. bare-repo fallback to "main") are not valid --set-upstream-to targets.
	if !strings.Contains(upstreamBranch, "/") {
		return nil
	}
	_, err := repository.git("branch", "--set-upstream-to", upstreamBranch, branchName)
	return err
}
