package gitwt

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

const (
	remoteName      = "origin"
	branchRefPrefix = "refs/heads/"
	remoteRefPrefix = "refs/remotes/"
)

type referenceName string

func openRepository(path string) (*Repository, error) {
	workTreeResult, err := gitOutput(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}

	gitDirResult, err := gitOutput(path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve Git directory: %w", err)
	}

	return &Repository{GitDir: gitDirResult.stdout, WorkTree: workTreeResult.stdout}, nil
}

type Repository struct {
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
	result, err := x.git("for-each-ref", "--format=%(refname)", branchRefPrefix)
	if err != nil {
		return nil, fmt.Errorf("list local branches: %w", err)
	}

	branches := make([]string, 0)
	for branchRef := range strings.SplitSeq(result.stdout, "\n") {
		if branchName := shortReference(referenceName(branchRef)); branchName != "" {
			branches = append(branches, branchName)
		}
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
	result, err := x.git("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve origin/HEAD: %w", err)
	}

	return result.stdout, nil
}

func (x *Repository) upstreamReference(branchName string) (referenceName, error) {
	branchRef := branchReference(branchName)
	result, err := x.git("for-each-ref", "--format=%(refname)%00%(upstream)", string(branchRef))
	if err != nil {
		return "", fmt.Errorf("read branch config for %q: %w", branchName, err)
	}

	for line := range strings.SplitSeq(result.stdout, "\n") {
		refName, upstreamRef, found := strings.Cut(line, "\x00")
		if !found || refName != string(branchRef) {
			continue
		}
		if upstreamRef == "" {
			return "", fmt.Errorf("branch %q has no upstream branch", branchName)
		}

		return referenceName(upstreamRef), nil
	}

	return "", fmt.Errorf("branch %q does not exist", branchName)
}

func branchReference(branchName string) referenceName {
	return referenceName(branchRefPrefix + branchName)
}

func shortReference(ref referenceName) string {
	name := strings.TrimPrefix(string(ref), branchRefPrefix)
	return strings.TrimPrefix(name, remoteRefPrefix)
}
