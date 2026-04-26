package gitwt

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Worktree struct {
	Path   string
	Branch string
}

func execGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w (stderr: %s)", args, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func execGitInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v in %s: %w (stderr: %s)", args, dir, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func getMainWorktreePath() (string, error) {
	// "git rev-parse --path-format=absolute --show-toplevel" gives the path of the current worktree, which might not be the main one.
	// A better way is to parse "git worktree list --porcelain" and find the first one or the one without "worktree" prefix if any?
	// Actually "git worktree list --porcelain" returns the main worktree first.
	out, err := execGit("worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			return strings.TrimPrefix(line, "worktree "), nil
		}
	}
	return "", fmt.Errorf("could not find main worktree")
}

func getNormalizedPath(mainPath, name string) string {
	normalizedName := strings.ReplaceAll(name, "/", ".")
	return mainPath + "." + normalizedName
}

func getGitWtWorktrees() ([]Worktree, error) {
	mainPath, err := getMainWorktreePath()
	if err != nil {
		return nil, err
	}

	out, err := execGit("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var currentWT Worktree

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			if currentWT.Path != "" {
				worktrees = append(worktrees, currentWT)
			}
			currentWT = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			branchRef := strings.TrimPrefix(line, "branch ")
			currentWT.Branch = strings.TrimPrefix(branchRef, "refs/heads/")
		}
	}
	if currentWT.Path != "" {
		worktrees = append(worktrees, currentWT)
	}

	var result []Worktree
	for _, wt := range worktrees {
		if wt.Path == mainPath {
			continue // Skip main worktree
		}
		if wt.Branch == "" {
			continue // Detached head or similar
		}
		expectedPath := getNormalizedPath(mainPath, wt.Branch)
		if wt.Path == expectedPath {
			result = append(result, wt)
		}
	}

	return result, nil
}

func getDefaultUpstream() (string, error) {
	// Get symbolic ref of origin/HEAD
	out, err := execGit("symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", fmt.Errorf("could not resolve origin/HEAD: %w", err)
	}
	// Output is like refs/remotes/origin/main, we want origin/main
	ref := strings.TrimPrefix(out, "refs/remotes/")
	return ref, nil
}

func isClean(dir string) (bool, error) {
	out, err := execGitInDir(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

func isMerged(branch, upstream string) (bool, error) {
	// Use git merge-base --is-ancestor <branch> <upstream>
	// Wait, is it branch -> upstream or upstream -> branch?
	// The prompt says "branch merged to upstream branch".
	// git merge-base --is-ancestor branch upstream checks if branch is an ancestor of upstream.
	// If branch is an ancestor of upstream, it means branch's commits are included in upstream.
	err := exec.Command("git", "merge-base", "--is-ancestor", branch, upstream).Run()
	if err != nil {
		// Exit status 1 means not an ancestor
		return false, nil
	}
	return true, nil
}

func getUpstreamOfBranch(branch string) (string, error) {
	out, err := execGit("rev-parse", "--abbrev-ref", branch+"@{u}")
	if err != nil {
		return "", err
	}
	return out, nil
}
