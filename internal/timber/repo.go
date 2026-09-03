package timber

import (
	"errors"
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
	command.AddCommand(NewRepoListCommand(runtime))
	command.AddCommand(NewRepoRemoveCommand(runtime))
	command.AddCommand(NewRepoRenameCommand(runtime))
	return command
}

type repoAddCommandOptions struct {
	runtime Runtime
	name    string
}

func NewRepoAddCommand(runtime Runtime) *cobra.Command {
	options := &repoAddCommandOptions{runtime: runtime}

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

	targetPath := x.runtime.bareRepoPath(repoName)
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("repository %q already exists at %s", repoName, targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect repository path %q: %w", targetPath, err)
	}

	if err := ensureDirectory(filepath.Dir(targetPath)); err != nil {
		return err
	}

	if _, err := gitOutput(x.runtime, x.runtime.CurrentDirectory, "clone", "--bare", remoteURL, targetPath); err != nil {
		return err
	}

	if err := configureBareOriginTracking(x.runtime, targetPath); err != nil {
		return err
	}

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("added repository "+repoName+" at "+targetPath))
	return err
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

type repoListCommandOptions struct {
	runtime Runtime
	quiet   bool
}

func NewRepoListCommand(runtime Runtime) *cobra.Command {
	options := &repoListCommandOptions{runtime: runtime}
	command := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered repositories",
		Args:    cobra.NoArgs,
		RunE:    options.Execute,
	}
	command.Flags().BoolVarP(&options.quiet, "quiet", "q", false, "Print repository names only")

	return command
}

func (x *repoListCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := x.runtime.listRegisteredRepos()
	if err != nil {
		return err
	}
	if x.quiet {
		return writeRepoNames(command, repos)
	}

	tableView := newOutputTable("Name", "Path", "Origin")
	for _, repo := range repos {
		tableView.Row(repo.Name, x.runtime.displayHomePath(repo.BarePath), repo.originURL(x.runtime))
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}

func (x registeredRepo) originURL(runtime Runtime) string {
	result, err := gitOutput(runtime, x.BarePath, "remote", "get-url", remoteName)
	if err != nil {
		return ""
	}
	return result.stdout
}

func writeRepoNames(command *cobra.Command, repos []registeredRepo) error {
	for _, repo := range repos {
		if _, err := fmt.Fprintln(command.OutOrStdout(), repo.Name); err != nil {
			return err
		}
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

func (x Runtime) completeRegisteredRepoNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return x.completeRegisteredRepoFlagValues(nil, nil, toComplete)
}

func (x Runtime) completeRegisteredRepoFlagValues(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	repos, err := x.listRegisteredRepos()
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
