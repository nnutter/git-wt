package gitwt

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

const (
	remoteName      = "origin"
	branchRefPrefix = "refs/heads/"
	remoteRefPrefix = "refs/remotes/"
)

type referenceName string

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

func (x *Repository) branchExists(branchName string) (bool, error) {
	return x.branchStillExists(branchReference(branchName))
}

func (x *Repository) branchMergedToUpstream(branchRef referenceName, upstreamRef referenceName) (bool, error) {
	upstreamExists, err := x.branchStillExists(upstreamRef)
	if err != nil {
		return false, err
	}
	if !upstreamExists {
		return false, nil
	}

	_, err = x.git("merge-base", "--is-ancestor", string(branchRef), string(upstreamRef))
	if err == nil {
		return true, nil
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func (x *Repository) branchStillExists(branchRef referenceName) (bool, error) {
	_, err := x.git("show-ref", "--verify", "--quiet", string(branchRef))
	if err == nil {
		return true, nil
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func (x Repository) git(args ...string) (gitCommandResult, error) {
	allArgs := append([]string{"--git-dir", x.GitDir, "--work-tree", x.WorkTree}, args...)
	return gitOutput(x.WorkTree, allArgs...)
}

func (x Repository) isClean() (bool, error) {
	result, err := x.git("status", "--porcelain")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(result.stdout) == "", nil
}

func (x Repository) status() (string, error) {
	result, err := x.git("status", "-sb")
	if err != nil {
		return "", err
	}

	statusLine, _, _ := strings.Cut(result.stdout, "\n")
	return strings.TrimPrefix(statusLine, "## "), nil
}

type porcelainWorktree struct {
	Path       string
	BranchRef  string
	CommitHash string
	Detached   bool
	Prunable   string
}

func (x porcelainWorktree) branchName() string {
	if !strings.HasPrefix(x.BranchRef, "refs/heads/") {
		return ""
	}

	return shortReference(referenceName(x.BranchRef))
}

func (x *Repository) listPorcelainWorktrees() ([]porcelainWorktree, error) {
	result, err := x.git("worktree", "list", "--porcelain")
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
		for line := range strings.SplitSeq(block, "\n") {
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

func (x *Repository) localBranches() ([]string, error) {
	branchIter, err := x.Branches()
	if err != nil {
		return nil, fmt.Errorf("list local branches: %w", err)
	}

	branches := make([]string, 0)
	err = branchIter.ForEach(func(branchRef *plumbing.Reference) error {
		branches = append(branches, shortReference(referenceName(branchRef.Name())))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate local branches: %w", err)
	}

	slices.Sort(branches)
	return branches, nil
}

func (x *Repository) mainWorktreePath() (string, error) {
	worktrees, err := x.listPorcelainWorktrees()
	if err != nil {
		return "", err
	}

	if len(worktrees) == 0 {
		return "", errors.New("no worktrees found")
	}

	return worktrees[0].Path, nil
}

func (x *Repository) mainWorktreeBranch() (string, error) {
	worktrees, err := x.listPorcelainWorktrees()
	if err != nil {
		return "", err
	}

	if len(worktrees) == 0 {
		return "", errors.New("no worktrees found")
	}

	branchName := worktrees[0].branchName()
	if branchName == "" {
		return "", errors.New("main worktree is not on a branch")
	}

	return branchName, nil
}

func (x *Repository) remoteHeadBranch() (string, error) {
	remoteHeadRef, err := x.Reference(plumbing.NewRemoteHEADReferenceName(remoteName), false)
	if err == nil && remoteHeadRef.Type() == plumbing.SymbolicReference {
		return shortReference(referenceName(remoteHeadRef.Target())), nil
	}

	result, commandErr := x.git("symbolic-ref", "refs/remotes/origin/HEAD")
	if commandErr != nil {
		return "", fmt.Errorf("resolve origin/HEAD: %w", err)
	}

	resolved := strings.TrimSpace(result.stdout)
	return shortReference(referenceName(resolved)), nil
}

func (x *Repository) upstreamReference(branchName string) (referenceName, error) {
	branchConfig, err := x.Branch(branchName)
	if err != nil {
		return "", fmt.Errorf("read branch config for %q: %w", branchName, err)
	}

	if branchConfig.Merge == "" {
		return "", fmt.Errorf("branch %q has no upstream branch", branchName)
	}

	if branchConfig.Remote == "" || branchConfig.Remote == "." {
		return referenceName(branchConfig.Merge), nil
	}

	return remoteReference(branchConfig.Remote, referenceName(branchConfig.Merge)), nil
}

func branchReference(branchName string) referenceName {
	return referenceName(branchRefPrefix + branchName)
}

func remoteReference(remote string, branchRef referenceName) referenceName {
	return referenceName(remoteRefPrefix + remote + "/" + shortReference(branchRef))
}

func shortReference(ref referenceName) string {
	name := strings.TrimPrefix(string(ref), branchRefPrefix)
	return strings.TrimPrefix(name, remoteRefPrefix)
}
