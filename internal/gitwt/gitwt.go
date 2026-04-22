package gitwt

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	RootCmd = &cobra.Command{
		Use:   "git-wt",
		Short: "Manage Git worktrees",
		Long:  "git-wt is a command line tool that uses 'git worktree' under the hood to manage Git worktrees.",
	}
)

func init() {
	RootCmd.AddCommand(NewCreateCommand(), NewListCommand(), NewRemoveCommand(), NewPruneCommand())
}

func mainWorktreePath() (string, error) {
	out, err := execGit("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func normalizeName(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

func resolveOriginHead() (string, error) {
	out, err := execGit("symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot resolve origin/HEAD: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func branchExists(name string) bool {
	_, err := execGit("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

func worktreeExists(path string) (bool, error) {
	worktrees, err := listWorktrees()
	if err != nil {
		return false, err
	}
	for _, wt := range worktrees {
		if wt.path == path {
			return true, nil
		}
	}
	return false, nil
}

func isWorktreeClean(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status failed: %w", err)
	}
	return len(strings.TrimSpace(string(out))) == 0, nil
}

func isBranchMerged(branch, upstream string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", branch, upstream)
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	} else {
		return false, fmt.Errorf("git merge-base failed: %w", err)
	}
}

type worktreeInfo struct {
	path   string
	branch string
}

func listWorktrees() ([]worktreeInfo, error) {
	out, err := execGit("worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list failed: %w", err)
	}

	var worktrees []worktreeInfo
	var current worktreeInfo
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.path != "" {
				worktrees = append(worktrees, current)
			}
			current = worktreeInfo{}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.path = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch ") {
			current.branch = strings.TrimPrefix(line, "branch ")
		}
	}
	if current.path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees, nil
}

func listGitWtWorktrees() ([]struct {
	name string
	path string
	branch string
}, error) {
	mainPath, err := mainWorktreePath()
	if err != nil {
		return nil, err
	}

	parentDir := filepath.Dir(mainPath)
	mainBase := filepath.Base(mainPath)

	worktrees, err := listWorktrees()
	if err != nil {
		return nil, err
	}

	var result []struct {
		name string
		path string
		branch string
	}

	for _, wt := range worktrees {
		if wt.path == mainPath {
			continue
		}
		wtParent := filepath.Dir(wt.path)
		if wtParent != parentDir {
			continue
		}
		wtBase := filepath.Base(wt.path)
		if !strings.HasPrefix(wtBase, mainBase+".") {
			continue
		}
		name := strings.TrimPrefix(wtBase, mainBase+".")
		result = append(result, struct {
			name string
			path string
			branch string
		}{name: name, path: wt.path, branch: wt.branch})
	}

	return result, nil
}

func execGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w\n%s", args, err, string(out))
	}
	return string(out), nil
}
