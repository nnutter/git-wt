package gitwt

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

const remoteName = "origin"

func PlainOpenWithOptions(path string) (*Repository, error) {
	gitRepository, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}

	workTreeResult, err := gitOutput(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}

	gitDirResult, err := gitOutput(path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}

	return &Repository{
		Repository: gitRepository,
		GitDir:     gitDirResult.stdout,
		WorkTree:   workTreeResult.stdout,
	}, nil
}

type Repository struct {
	*git.Repository

	GitDir   string
	WorkTree string
}

func (repository *Repository) addBranchConfig(branchName string, upstream string) error {
	upstreamRef := plumbing.ReferenceName(upstream)
	if !strings.HasPrefix(upstream, "refs/") {
		if strings.HasPrefix(upstream, remoteName+"/") {
			upstreamRef = plumbing.NewRemoteReferenceName(remoteName, strings.TrimPrefix(upstream, remoteName+"/"))
		} else {
			upstreamRef = plumbing.NewBranchReferenceName(upstream)
		}
	}

	branchConfig := &gitconfig.Branch{
		Name:   branchName,
		Remote: remoteName,
		Merge:  plumbing.NewBranchReferenceName(upstreamRef.Short()),
	}

	config, err := repository.Config()
	if err != nil {
		return fmt.Errorf("read repository config: %w", err)
	}

	if config.Branches == nil {
		config.Branches = map[string]*gitconfig.Branch{}
	}
	config.Branches[branchName] = branchConfig

	if err := repository.SetConfig(config); err != nil {
		return fmt.Errorf("write repository config: %w", err)
	}

	return nil
}

func (repository *Repository) branchExists(branchName string) (bool, error) {
	branchRef := plumbing.NewBranchReferenceName(branchName)
	return repository.branchStillExists(branchRef)
}

func (repository *Repository) branchMergedToUpstream(branchRef plumbing.ReferenceName, upstreamRef plumbing.ReferenceName) (bool, error) {
	_, err := repository.git("merge-base", "--is-ancestor", branchRef.String(), upstreamRef.String())
	if err == nil {
		return true, nil
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func (repository *Repository) branchStillExists(branchRef plumbing.ReferenceName) (bool, error) {
	_, err := repository.Reference(branchRef, true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (repository Repository) git(args ...string) (gitCommandResult, error) {
	allArgs := append([]string{"--git-dir", repository.GitDir, "--work-tree", repository.WorkTree}, args...)
	return gitOutput(repository.WorkTree, allArgs...)
}

type porcelainWorktree struct {
	Path       string
	BranchRef  string
	CommitHash string
	Detached   bool
	Prunable   string
}

func (repository *Repository) listPorcelainWorktrees() ([]porcelainWorktree, error) {
	result, err := repository.git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	blocks := strings.Split(strings.TrimSpace(result.stdout), "\n\n")
	worktrees := make([]porcelainWorktree, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		var worktree porcelainWorktree
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				worktree.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "branch "):
				worktree.BranchRef = strings.TrimPrefix(line, "branch ")
			case strings.HasPrefix(line, "HEAD "):
				worktree.CommitHash = strings.TrimPrefix(line, "HEAD ")
			case line == "detached":
				worktree.Detached = true
			case strings.HasPrefix(line, "prunable "):
				worktree.Prunable = strings.TrimPrefix(line, "prunable ")
			}
		}

		if worktree.Path != "" {
			worktrees = append(worktrees, worktree)
		}
	}

	return worktrees, nil
}

func (repository *Repository) mainWorktreePath() (string, error) {
	worktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return "", err
	}

	if len(worktrees) == 0 {
		return "", errors.New("no worktrees found")
	}

	return worktrees[0].Path, nil
}

func (repository *Repository) remoteHeadBranch() (string, plumbing.ReferenceName, error) {
	remoteHeadRef, err := repository.Reference(plumbing.NewRemoteHEADReferenceName(remoteName), false)
	if err == nil && remoteHeadRef.Type() == plumbing.SymbolicReference {
		return remoteHeadRef.Target().Short(), remoteHeadRef.Target(), nil
	}

	result, commandErr := repository.git("symbolic-ref", "refs/remotes/origin/HEAD")
	if commandErr != nil {
		return "", "", fmt.Errorf("resolve origin/HEAD: %w", err)
	}

	resolved := strings.TrimSpace(result.stdout)
	return plumbing.ReferenceName(resolved).Short(), plumbing.ReferenceName(resolved), nil
}
