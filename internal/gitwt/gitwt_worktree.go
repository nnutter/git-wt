package gitwt

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

// ManagedWorktree represents a Git worktree created and managed by git-wt.
// It is a sibling directory of the main worktree using the dot-normalized naming scheme.
type ManagedWorktree struct {
	// Branch is the full branch name stored in the worktree (e.g. "nn/feat-1").
	Branch string
	// Path is the absolute filesystem path of the worktree workdir.
	Path string
}

// normalizeName converts a branch name to a path-safe suffix by replacing '/' with '.'.
func normalizeName(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

// siblingWorktreePath returns the absolute path that git-wt would use for a worktree
// with the given branch name, given the main worktree's absolute path.
func siblingWorktreePath(mainWorktreePath, branchName string) string {
	parent := filepath.Dir(mainWorktreePath)
	base := filepath.Base(mainWorktreePath)
	return filepath.Join(parent, base+"."+normalizeName(branchName))
}

// resolveMainWorktreePath shells out to git to find the main worktree path.
// The main worktree is the first entry in `git worktree list --porcelain`.
func resolveMainWorktreePath(repoPath string) (string, error) {
	worktrees, err := parseWorktrees(repoPath)
	if err != nil {
		return "", err
	}
	if len(worktrees) == 0 {
		return "", fmt.Errorf("no worktrees found")
	}
	return worktrees[0].path, nil
}

// rawWorktree is an internal type used only during porcelain parsing.
type rawWorktree struct {
	path   string
	branch string // full ref, e.g. "refs/heads/nn/feat-1"
	isBare bool
}

// parseWorktrees runs `git worktree list --porcelain` from repoPath and returns
// all worktree entries including the main worktree as the first element.
func parseWorktrees(repoPath string) ([]rawWorktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	return parsePorcelainWorktrees(string(out)), nil
}

// parsePorcelainWorktrees parses the output of `git worktree list --porcelain`.
func parsePorcelainWorktrees(output string) []rawWorktree {
	var result []rawWorktree
	var current rawWorktree
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = rawWorktree{path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			current.branch = strings.TrimPrefix(line, "branch ")
		case line == "bare":
			current.isBare = true
		case line == "":
			if current.path != "" {
				result = append(result, current)
				current = rawWorktree{}
			}
		}
	}
	// Final entry if file doesn't end with blank line.
	if current.path != "" {
		result = append(result, current)
	}
	return result
}

// listManagedWorktrees returns all worktrees that appear to have been created by
// git-wt: they are siblings of the main worktree and follow the dot-suffix naming scheme.
func listManagedWorktrees(repoPath string) ([]ManagedWorktree, error) {
	all, err := parseWorktrees(repoPath)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}

	mainPath := all[0].path
	mainParent := filepath.Dir(mainPath)
	mainBase := filepath.Base(mainPath)
	prefix := mainBase + "."

	slog.Debug("listManagedWorktrees", "mainPath", mainPath, "prefix", prefix)

	var managed []ManagedWorktree
	for _, wt := range all[1:] {
		dir := filepath.Dir(wt.path)
		base := filepath.Base(wt.path)
		if dir != mainParent || !strings.HasPrefix(base, prefix) {
			continue
		}
		branchName := strings.TrimPrefix(wt.branch, "refs/heads/")
		managed = append(managed, ManagedWorktree{
			Branch: branchName,
			Path:   wt.path,
		})
	}
	return managed, nil
}

// findManagedWorktree returns the ManagedWorktree whose branch matches branchName.
func findManagedWorktree(repoPath, branchName string) (ManagedWorktree, error) {
	worktrees, err := listManagedWorktrees(repoPath)
	if err != nil {
		return ManagedWorktree{}, err
	}
	for _, wt := range worktrees {
		if wt.Branch == branchName {
			return wt, nil
		}
	}
	return ManagedWorktree{}, fmt.Errorf("no managed worktree found for branch %q", branchName)
}

// resolveDefaultUpstream reads the symbolic ref at refs/remotes/origin/HEAD to
// determine the default upstream branch (e.g. "origin/main").
func resolveDefaultUpstream(repoPath string) (string, error) {
	repo, err := gogit.PlainOpenWithOptions(repoPath, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}
	ref, err := repo.Reference("refs/remotes/origin/HEAD", true)
	if err != nil {
		return "", fmt.Errorf("resolve refs/remotes/origin/HEAD: %w", err)
	}
	// ref.Name() is e.g. "refs/remotes/origin/main"
	remoteBranch := strings.TrimPrefix(ref.Name().String(), "refs/remotes/")
	return remoteBranch, nil
}

// resolveBranchUpstream returns the configured upstream tracking branch for
// the given local branch name (e.g. "origin/main").
// It shells out to git config so it correctly reads from the shared .git/config
// even when called from a linked worktree path.
func resolveBranchUpstream(repoPath, branchName string) (string, error) {
	remote, err := gitConfigValue(repoPath, "branch."+branchName+".remote")
	if err != nil {
		return "", fmt.Errorf("branch %q has no upstream tracking configured", branchName)
	}
	merge, err := gitConfigValue(repoPath, "branch."+branchName+".merge")
	if err != nil {
		return "", fmt.Errorf("branch %q has no upstream merge configured", branchName)
	}
	// merge is e.g. "refs/heads/main"; convert to "origin/main"
	mergeName := strings.TrimPrefix(merge, "refs/heads/")
	return remote + "/" + mergeName, nil
}

// gitConfigValue reads a single git config value by key from the repo at repoPath.
func gitConfigValue(repoPath, key string) (string, error) {
	cmd := exec.Command("git", "config", "--local", key)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// isBranchMergedToUpstream reports whether the HEAD of the worktree at worktreePath
// is an ancestor of the given upstream ref (e.g. "origin/main").
func isBranchMergedToUpstream(worktreePath, upstream string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", "HEAD", upstream)
	cmd.Dir = worktreePath
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
		// exit 1 means HEAD is not an ancestor — not merged.
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor: %w", err)
}

// isWorktreeClean reports whether the working directory at worktreePath has no
// uncommitted changes (staged or unstaged).
// It shells out to git status --porcelain because go-git does not correctly
// report status for linked worktrees.
func isWorktreeClean(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain at %s: %w", worktreePath, err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

// runGitCommand runs an arbitrary git command in the given directory and returns
// its combined output on error.
func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// pathExists reports whether the given path exists in the filesystem.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolveRepoDotGitDir returns the top-level directory of the Git repository
// that contains the current working directory.
func resolveRepoDotGitDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
