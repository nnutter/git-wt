package timber

import (
	"errors"
	"fmt"
	"io"
	"os"
)

type repoSelection struct {
	runtime      Runtime
	RepoName     string
	repoPrompter repoPrompter
}

func (x *repoSelection) resolve(input io.Reader) (registeredRepo, *Repository, error) {
	if x.RepoName != "" {
		return x.resolveNamed(x.RepoName)
	}
	if repo, repository, err := x.tryResolveCurrent(); err == nil {
		return repo, repository, nil
	}
	return x.resolvePrompt(input)
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

// reposToConsider returns the qualifier repo if set, else every registered
// repository. It does not show the repository picker.
func (x *repoSelection) reposToConsider() ([]registeredRepo, error) {
	if x.RepoName != "" {
		repo, err := x.runtime.registeredRepoByName(x.RepoName)
		if err != nil {
			return nil, err
		}
		return []registeredRepo{repo}, nil
	}
	return x.runtime.listRegisteredRepos()
}

func (x *repoSelection) resolveNamed(name string) (registeredRepo, *Repository, error) {
	repository, repo, err := x.runtime.openRegisteredRepository(name)
	return repo, repository, err
}

func (x *repoSelection) tryResolveCurrent() (registeredRepo, *Repository, error) {
	currentDirectory := x.runtime.CurrentDirectory
	worktreeRepository, err := openRepository(x.runtime, currentDirectory)
	if err != nil {
		return registeredRepo{}, nil, fmt.Errorf("current directory is not inside a Git worktree: %w", err)
	}

	commonDir, err := worktreeRepository.commonGitDir()
	if err != nil {
		return registeredRepo{}, nil, err
	}

	repos, err := x.runtime.listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, nil, err
	}

	for _, repo := range repos {
		same, err := samePath(repo.BarePath, commonDir)
		if err != nil {
			return registeredRepo{}, nil, err
		}
		if same {
			repository, err := openBareRepository(x.runtime, repo.BarePath)
			if err != nil {
				return registeredRepo{}, nil, err
			}
			return repo, repository, nil
		}
	}

	return registeredRepo{}, nil, fmt.Errorf(
		"current worktree is not part of a registered repository (common git dir %s)",
		commonDir,
	)
}

func (x *repoSelection) resolvePrompt(input io.Reader) (registeredRepo, *Repository, error) {
	repos, err := x.runtime.listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, nil, err
	}
	if len(repos) == 0 {
		return registeredRepo{}, nil, errors.New("no registered repositories; run timber repo add first")
	}

	if !isInteractiveTerminal(input) {
		return registeredRepo{}, nil, errors.New("repository selection requires @<repo>, a managed worktree cwd, or an interactive terminal")
	}

	prompter := x.repoPrompter
	if prompter == nil {
		prompter = bubbleteaRepoPrompter{
			input: input,
		}
	}

	selected, err := prompter.Prompt(repos)
	if err != nil {
		return registeredRepo{}, nil, err
	}

	repository, err := openBareRepository(x.runtime, selected.BarePath)
	if err != nil {
		return registeredRepo{}, nil, err
	}
	return selected, repository, nil
}

func isInteractiveTerminal(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}
	if fileInfo.Mode()&os.ModeCharDevice == 0 {
		return false
	}

	// bubbletea needs /dev/tty; treat its absence as non-interactive.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	if err := tty.Close(); err != nil {
		return false
	}
	return true
}

func samePath(left string, right string) (bool, error) {
	return canonicalPath(left) == canonicalPath(right), nil
}
