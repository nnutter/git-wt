package timber

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type repoRemoveCommandOptions struct {
	runtime Runtime
}

func NewRepoRemoveCommand(runtime Runtime) *cobra.Command {
	options := &repoRemoveCommandOptions{runtime: runtime}
	return &cobra.Command{
		Use:               "remove <name>",
		Aliases:           []string{"rm"},
		Short:             "Remove a registered repository with no worktrees",
		Args:              cobra.ExactArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeRegisteredRepoNames,
	}
}

func (x *repoRemoveCommandOptions) Execute(command *cobra.Command, args []string) error {
	repoName := args[0]
	repository, repo, err := x.runtime.openRegisteredRepository(repoName)
	if err != nil {
		return err
	}

	worktrees, err := x.runtime.managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return err
	}
	if len(worktrees) > 0 {
		names := make([]string, 0, len(worktrees))
		for _, worktree := range worktrees {
			names = append(names, worktree.Name)
		}
		return fmt.Errorf(
			"repository %q still has managed worktrees (%s); remove them first",
			repoName,
			strings.Join(names, ", "),
		)
	}

	// Also refuse if git still has any non-bare linked worktrees (unmanaged).
	porcelain, err := repository.listPorcelainWorktrees()
	if err != nil {
		return err
	}
	linked := 0
	for _, worktree := range porcelain {
		same, err := samePath(worktree.Path, repo.BarePath)
		if err != nil {
			return err
		}
		if same {
			continue
		}
		linked++
	}
	if linked > 0 {
		return fmt.Errorf("repository %q still has %d linked worktree(s); remove them first", repoName, linked)
	}

	if err := os.RemoveAll(repo.BarePath); err != nil {
		return fmt.Errorf("remove bare repository %q: %w", repo.BarePath, err)
	}

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("removed repository "+repoName))
	return err
}
