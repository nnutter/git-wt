package timber

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func NewRepoCommand(runtime Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "repo",
		Short: "Manage registered bare repositories",
	}
	command.AddCommand(NewRepoAddCommand(runtime))
	command.AddCommand(NewRepoImportCommand(runtime))
	command.AddCommand(NewRepoListCommand(runtime))
	command.AddCommand(NewRepoRemoveCommand(runtime))
	command.AddCommand(NewRepoRenameCommand(runtime))
	return command
}

// configureBareOriginTracking makes a bare clone usable like a normal remote-tracking
// repository. `git clone --bare` omits remote.origin.fetch, so refs/remotes/origin/*
// (including origin/HEAD) are never populated without this setup.
func configureBareOriginTracking(runtime Runtime, barePath string) error {
	repository, err := openBareRepository(runtime, barePath)
	if err != nil {
		return err
	}

	if _, err := repository.git(
		"config",
		"remote."+remoteName+".fetch",
		"+refs/heads/*:refs/remotes/"+remoteName+"/*",
	); err != nil {
		return err
	}
	if _, err := repository.git("fetch", remoteName); err != nil {
		return err
	}
	if _, err := repository.git("remote", "set-head", remoteName, "--auto"); err != nil {
		// Non-fatal when the remote has no HEAD; local fallbacks still apply later.
		return nil
	}
	return nil
}

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

func validateRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("repository name is required")
	}
	if strings.HasSuffix(name, bareRepoSuffix) {
		return fmt.Errorf("repository name %q must not end with %s", name, bareRepoSuffix)
	}
	if strings.Contains(name, "/") || strings.Contains(name, string(filepath.Separator)) {
		return fmt.Errorf("repository name %q must not contain path separators", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("repository name %q is invalid", name)
	}
	return nil
}
