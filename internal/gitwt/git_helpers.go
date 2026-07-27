package gitwt

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type gitCommandResult struct {
	stdout string
	stderr string
}

func gitOutput(directory string, args ...string) (gitCommandResult, error) {
	command := exec.Command("git", args...)
	command.Dir = directory

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		result := gitCommandResult{
			stdout: strings.TrimSpace(stdout.String()),
			stderr: strings.TrimSpace(stderr.String()),
		}

		return result, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, result.stderr)
	}

	result := gitCommandResult{
		stdout: strings.TrimSpace(stdout.String()),
		stderr: strings.TrimSpace(stderr.String()),
	}

	return result, nil
}

func managedWorktreePath(mainPath string, branchName string) string {
	return filepath.Join(filepath.Dir(mainPath), branchName)
}

func ensureWorktreeParent(worktreePath string) error {
	parent := filepath.Dir(worktreePath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create worktree parent directory %q: %w", parent, err)
	}
	return nil
}

func currentRelativePath(currentDirectory string, targetPath string) string {
	relativePath, err := filepath.Rel(currentDirectory, targetPath)
	if err != nil {
		return targetPath
	}

	return relativePath
}

func worktreeIsClean(repository *Repository, worktreePath string) (bool, error) {
	worktreeRepository := *repository
	worktreeRepository.WorkTree = worktreePath

	result, err := worktreeRepository.git("status", "--porcelain")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(result.stdout) == "", nil
}

func branchDeleteFlag(force bool) string {
	if force {
		return "-D"
	}

	return "-d"
}
