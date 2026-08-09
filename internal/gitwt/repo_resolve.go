package gitwt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type repoSelection struct {
	RepoFlag          string
	CurrentFlag       bool
	autoDetectCurrent bool
	repoPrompter      repoPrompter
}

// addRepoFlag registers --repo only (for list/prune/remove with auto-detect).
func (x *repoSelection) addRepoFlag(command *cobra.Command) {
	x.autoDetectCurrent = true
	command.Flags().StringVar(&x.RepoFlag, "repo", "", "Registered repository name")
}

// addFlags registers --repo and --current (for create, which keeps explicit --current).
func (x *repoSelection) addFlags(command *cobra.Command) {
	x.autoDetectCurrent = false
	command.Flags().StringVar(&x.RepoFlag, "repo", "", "Registered repository name")
	command.Flags().BoolVar(&x.CurrentFlag, "current", false, "Use the repository for the current worktree")
	command.MarkFlagsMutuallyExclusive("repo", "current")
}

func (x *repoSelection) resolve() (registeredRepo, *Repository, error) {
	if x.RepoFlag != "" {
		return x.resolveNamed(x.RepoFlag)
	}
	if x.CurrentFlag {
		return x.resolveCurrent()
	}
	if x.autoDetectCurrent {
		if repo, repository, err := x.tryResolveCurrent(); err == nil {
			return repo, repository, nil
		}
	}
	return x.resolvePrompt()
}

func (x *repoSelection) resolveNamed(name string) (registeredRepo, *Repository, error) {
	repository, repo, err := openRegisteredRepository(name)
	return repo, repository, err
}

func (x *repoSelection) resolveCurrent() (registeredRepo, *Repository, error) {
	repo, repository, err := x.tryResolveCurrent()
	if err != nil {
		return registeredRepo{}, nil, err
	}
	return repo, repository, nil
}

func (x *repoSelection) tryResolveCurrent() (registeredRepo, *Repository, error) {
	currentDirectory, err := os.Getwd()
	if err != nil {
		return registeredRepo{}, nil, fmt.Errorf("get current directory: %w", err)
	}

	worktreeRepository, err := openRepository(currentDirectory)
	if err != nil {
		return registeredRepo{}, nil, fmt.Errorf("current directory is not inside a Git worktree: %w", err)
	}

	commonDir, err := worktreeRepository.commonGitDir()
	if err != nil {
		return registeredRepo{}, nil, err
	}

	repos, err := listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, nil, err
	}

	for _, repo := range repos {
		same, err := samePath(repo.BarePath, commonDir)
		if err != nil {
			return registeredRepo{}, nil, err
		}
		if same {
			repository, err := openBareRepository(repo.BarePath)
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

func (x *repoSelection) resolvePrompt() (registeredRepo, *Repository, error) {
	repos, err := listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, nil, err
	}
	if len(repos) == 0 {
		return registeredRepo{}, nil, errors.New("no registered repositories; run git-wt repo add first")
	}

	if !isInteractiveTerminal() {
		if x.autoDetectCurrent {
			return registeredRepo{}, nil, errors.New("repository selection requires --repo, a managed worktree cwd, or an interactive terminal")
		}
		return registeredRepo{}, nil, errors.New("repository selection requires --repo, --current, or an interactive terminal")
	}

	prompter := x.repoPrompter
	if prompter == nil {
		prompter = bubbleteaRepoPrompter{}
	}

	selected, err := prompter.Prompt(repos)
	if err != nil {
		return registeredRepo{}, nil, err
	}

	repository, err := openBareRepository(selected.BarePath)
	if err != nil {
		return registeredRepo{}, nil, err
	}
	return selected, repository, nil
}

func (x *Repository) commonGitDir() (string, error) {
	result, err := x.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("resolve common git dir: %w", err)
	}
	return filepath.Clean(result.stdout), nil
}

func isInteractiveTerminal() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if fileInfo.Mode()&os.ModeCharDevice == 0 {
		return false
	}

	// bubbletea needs /dev/tty; treat its absence as non-interactive (CI/tests).
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = tty.Close()
	return true
}

func samePath(left string, right string) (bool, error) {
	leftResolved, err := filepath.EvalSymlinks(left)
	if err != nil {
		leftResolved = filepath.Clean(left)
	}
	rightResolved, err := filepath.EvalSymlinks(right)
	if err != nil {
		rightResolved = filepath.Clean(right)
	}
	return leftResolved == rightResolved, nil
}
