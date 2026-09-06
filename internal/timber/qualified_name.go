package timber

import (
	"fmt"
	"io"
	"strings"
)

type qualifiedName struct {
	Name string
	Repo string
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
