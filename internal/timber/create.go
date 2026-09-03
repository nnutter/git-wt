package timber

import (
	"errors"
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

func NewCreateCommand(runtime Runtime) *cobra.Command {
	options := &createCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:               "create [name[@repo]]",
		Short:             "Create a managed Git worktree",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeCreateArgs,
	}

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
	return x.runtime.reportCreatedWorktreePath(command, worktreePath)
}

func (x *createCommandOptions) createWorktree(command *cobra.Command, args []string) (string, error) {
	var raw string
	if len(args) == 1 {
		raw = args[0]
	}
	qualified, err := x.runtime.parseQualifiedName(raw)
	if err != nil {
		return "", err
	}
	if err := rejectAtInWorktreeName(qualified.Name); err != nil {
		return "", err
	}
	if qualified.Repo != "" {
		x.RepoName = qualified.Repo
	}

	repo, repository, err := x.resolve(command.InOrStdin())
	if err != nil {
		return "", err
	}

	branchName, err := x.runtime.resolveCreateWorktreeName(repo.Name, qualified.Name)
	if err != nil {
		return "", err
	}

	worktreePath := x.runtime.managedWorktreePath(repo.Name, branchName)
	if _, err := os.Stat(worktreePath); err == nil {
		return "", fmt.Errorf("worktree directory %q already exists", worktreePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}

	if _, err := repository.git("fetch", remoteName); err != nil {
		return "", fmt.Errorf("fetch %s: %w", remoteName, err)
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
	if err := x.runtime.openHerdrSpace(command.Context(), worktree); err != nil {
		return "", err
	}
	if err := reportOpenedHerdrSpace(command, branchName); err != nil {
		return "", err
	}
	return worktreePath, nil
}

func (x Runtime) resolveCreateWorktreeName(repoName string, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	return x.unusedWorktreeName(repoName)
}

func (x *createCommandOptions) shouldCreateHerdrWorkspace() bool {
	return x.herdr || (!x.noHerdr && x.runtime.HerdrEnvironment)
}

const createPathFileEnvVarName = "TIMBER_CREATE_PATH_FILE"

func (x Runtime) reportCreatedWorktreePath(command *cobra.Command, worktreePath string) error {
	if pathFile := x.CreatePathFile; pathFile != "" {
		if err := x.writePathFile(pathFile, worktreePath); err != nil {
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
