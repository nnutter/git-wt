package timber

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

type qualifiedName struct {
	Name string
	Repo string
}

func (x Runtime) parseQualifiedName(raw string) (qualifiedName, error) {
	at := strings.LastIndex(raw, "@")
	if at < 0 {
		return qualifiedName{Name: raw}, nil
	}

	name := raw[:at]
	repo := raw[at+1:]
	if repo == "" {
		return qualifiedName{}, fmt.Errorf("missing repository name after @")
	}
	if _, err := x.registeredRepoByName(repo); err != nil {
		return qualifiedName{}, err
	}
	return qualifiedName{Name: name, Repo: repo}, nil
}

func (x Runtime) parseRepoOnlyArg(raw string) (string, error) {
	qualified, err := x.parseQualifiedName(raw)
	if err != nil {
		return "", err
	}
	if qualified.Name != "" || qualified.Repo == "" {
		return "", fmt.Errorf("expected @<repo>, got %q", raw)
	}
	return qualified.Repo, nil
}

func rejectAtInWorktreeName(name string) error {
	if strings.Contains(name, "@") {
		return fmt.Errorf("worktree name %q must not contain @", name)
	}
	return nil
}

func nameArgs(name string) []string {
	if name == "" {
		return nil
	}
	return []string{name}
}

func (x *repoSelection) resolveForWorktree(worktreeName string, input io.Reader) (registeredRepo, *Repository, error) {
	if x.RepoName != "" {
		return x.resolveNamed(x.RepoName)
	}
	if worktreeName != "" {
		repoName, err := x.runtime.inferUniqueRepoForWorktree(worktreeName)
		if err != nil {
			return registeredRepo{}, nil, err
		}
		return x.resolveNamed(repoName)
	}
	if repo, repository, err := x.tryResolveCurrent(); err == nil {
		return repo, repository, nil
	}
	return x.resolvePrompt(input)
}

func (x Runtime) inferUniqueRepoForWorktree(worktreeName string) (string, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return "", err
	}

	var matches []string
	for _, repo := range repos {
		worktreePath := x.managedWorktreePath(repo.Name, worktreeName)
		_, err := os.Stat(worktreePath)
		if err == nil {
			matches = append(matches, repo.Name)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("worktree %s not found", worktreeName)
	default:
		slices.Sort(matches)
		return "", fmt.Errorf(
			"worktree %q exists in multiple repositories; qualify as <worktree>@<repo> (%s)",
			worktreeName,
			strings.Join(matches, ", "),
		)
	}
}

func (x Runtime) completeQualifiedWorktreeNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if at := strings.LastIndex(toComplete, "@"); at >= 0 {
		name := toComplete[:at]
		if name == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return x.completeRepoSuffix(name, toComplete[at+1:], true)
	}

	return x.completeWorktreeNamesAcrossRepos(toComplete)
}

func (x Runtime) completeCreateArgs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if at := strings.LastIndex(toComplete, "@"); at >= 0 {
		return x.completeRepoSuffix(toComplete[:at], toComplete[at+1:], false)
	}
	return x.completeRepoQualifiers(nil, args, toComplete)
}

func (x Runtime) completeRepoQualifiers(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if toComplete != "" && !strings.HasPrefix(toComplete, "@") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return x.completeRepoSuffix("", strings.TrimPrefix(toComplete, "@"), false)
}

func (x Runtime) completeRepoSuffix(worktreeName string, repoPrefix string, requireWorktree bool) ([]string, cobra.ShellCompDirective) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	names := make([]string, 0)
	for _, repo := range repos {
		if !strings.HasPrefix(repo.Name, repoPrefix) {
			continue
		}
		if requireWorktree {
			if _, err := os.Stat(x.managedWorktreePath(repo.Name, worktreeName)); err != nil {
				continue
			}
		}
		names = append(names, worktreeName+"@"+repo.Name)
	}
	slices.Sort(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

func (x Runtime) completeWorktreeNamesAcrossRepos(toComplete string) ([]string, cobra.ShellCompDirective) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	reposForName := make(map[string][]string)
	var names []string
	for _, repo := range repos {
		for _, name := range x.managedWorktreeNamesOnDisk(repo.Name, toComplete) {
			if _, exists := reposForName[name]; !exists {
				names = append(names, name)
			}
			reposForName[name] = append(reposForName[name], repo.Name)
		}
	}

	completions := make([]string, 0, len(names))
	for _, name := range names {
		reposWithName := reposForName[name]
		if len(reposWithName) == 1 {
			completions = append(completions, name)
			continue
		}
		for _, repoName := range reposWithName {
			completions = append(completions, name+"@"+repoName)
		}
	}
	slices.Sort(completions)
	return completions, cobra.ShellCompDirectiveNoFileComp
}
