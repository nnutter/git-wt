package gitwt

import (
	"os/exec"
	"strings"
)

func gitCmd(args ...string) *exec.Cmd {
	return exec.Command("git", args...)
}

func gitWorktreeAdd(dir, branch, path, upstreamBranch string) error {
	cmd := gitCmd("worktree", "add", "-b", branch, path, upstreamBranch)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &gitError{err: err, output: string(out)}
	}
	return nil
}

func gitWorktreeRemove(dir, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	cmd := gitCmd(args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &gitError{err: err, output: string(out)}
	}
	return nil
}

func gitWorktreeList(dir string) (string, error) {
	cmd := gitCmd("worktree", "list", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func gitMergeBaseIsAncestor(dir, ancestor, descendant string) (bool, error) {
	cmd := gitCmd("merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func gitBranchSetUpstream(dir, branch, upstream string) error {
	cmd := gitCmd("branch", "--set-upstream-to="+upstream, branch)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &gitError{err: err, output: string(out)}
	}
	return nil
}

func gitBranchDelete(dir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	cmd := gitCmd("branch", flag, branch)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &gitError{err: err, output: string(out)}
	}
	return nil
}

func gitBranchExists(dir, branch string) (bool, error) {
	cmd := gitCmd("branch", "--list", branch)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func gitResolveSymref(dir, symref string) (string, error) {
	cmd := gitCmd("symbolic-ref", symref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitWorkdirClean(dir string) (bool, error) {
	cmd := gitCmd("diff", "--quiet")
	cmd.Dir = dir
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}

	cmd = gitCmd("diff", "--cached", "--quiet")
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func gitBranchUpstream(dir, branch string) (string, error) {
	cmd := gitCmd("rev-parse", "--abbrev-ref", branch+"@{upstream}")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitResolveLocalBranch(dir, upstream string) string {
	return strings.TrimPrefix(upstream, "origin/")
}

type gitError struct {
	err   error
	output string
}

func (e *gitError) Error() string {
	msg := e.err.Error()
	if e.output != "" {
		msg = strings.TrimSpace(e.output)
	}
	return msg
}

func (e *gitError) Unwrap() error {
	return e.err
}
