package gitwt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func NewRepoCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "repo",
		Short: "Manage registered bare repositories",
	}
	command.AddCommand(NewRepoAddCommand())
	command.AddCommand(NewRepoListCommand())
	command.AddCommand(NewRepoRemoveCommand())
	command.AddCommand(NewRepoRenameCommand())
	return command
}

type repoAddCommandOptions struct {
	name string
}

func NewRepoAddCommand() *cobra.Command {
	options := new(repoAddCommandOptions)

	command := &cobra.Command{
		Use:   "add <url-or-path>",
		Short: "Register a bare repository from a remote URL or path",
		Args:  cobra.ExactArgs(1),
		RunE:  options.Execute,
	}
	command.Flags().StringVar(&options.name, "name", "", "Repository name (default: derived from URL)")

	return command
}

func (x *repoAddCommandOptions) Execute(command *cobra.Command, args []string) error {
	remoteURL, err := resolveRemoteURL(args[0])
	if err != nil {
		return err
	}

	repoName := normalizeRepoName(x.name)
	if repoName == "" {
		repoName, err = defaultRepoNameFromRemote(remoteURL)
		if err != nil {
			return err
		}
	}
	if err := validateRepoName(repoName); err != nil {
		return err
	}

	targetPath := bareRepoPath(repoName)
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("repository %q already exists at %s", repoName, targetPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect repository path %q: %w", targetPath, err)
	}

	if err := ensureDirectory(filepath.Dir(targetPath)); err != nil {
		return err
	}

	if _, err := gitOutput(".", "clone", "--bare", remoteURL, targetPath); err != nil {
		return err
	}

	if err := configureBareOriginTracking(targetPath); err != nil {
		return err
	}

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("added repository "+repoName+" at "+targetPath))
	return err
}

// configureBareOriginTracking makes a bare clone usable like a normal remote-tracking
// repository. `git clone --bare` omits remote.origin.fetch, so refs/remotes/origin/*
// (including origin/HEAD) are never populated without this setup.
func configureBareOriginTracking(barePath string) error {
	repository, err := openBareRepository(barePath)
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

type repoListCommandOptions struct{}

func NewRepoListCommand() *cobra.Command {
	options := new(repoListCommandOptions)
	return &cobra.Command{
		Use:   "list",
		Short: "List registered repositories",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
}

func (x *repoListCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := listRegisteredRepos()
	if err != nil {
		return err
	}

	tableView := newOutputTable("Name", "Path")
	for _, repo := range repos {
		tableView.Row(repo.Name, displayHomePath(repo.BarePath))
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}

type repoRemoveCommandOptions struct{}

func NewRepoRemoveCommand() *cobra.Command {
	options := new(repoRemoveCommandOptions)
	return &cobra.Command{
		Use:               "remove <name>",
		Short:             "Remove a registered repository with no worktrees",
		Args:              cobra.ExactArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: completeRegisteredRepoNames,
	}
}

func (x *repoRemoveCommandOptions) Execute(command *cobra.Command, args []string) error {
	repoName := args[0]
	repository, repo, err := openRegisteredRepository(repoName)
	if err != nil {
		return err
	}

	worktrees, err := managedWorktreesFromRepository(repository, repo.Name)
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

func completeRegisteredRepoNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeRegisteredRepoFlagValues(nil, nil, toComplete)
}

func completeRegisteredRepoFlagValues(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	repos, err := listRegisteredRepos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		if strings.HasPrefix(repo.Name, toComplete) {
			names = append(names, repo.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
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
