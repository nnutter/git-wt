package gitwt

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
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

func normalizeWorktreeName(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

func managedWorktreePath(mainPath string, branchName string) string {
	parentDirectory := filepath.Dir(mainPath)
	baseName := filepath.Base(mainPath)
	return filepath.Join(parentDirectory, baseName+"."+normalizeWorktreeName(branchName))
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

func upstreamReference(repository *Repository, branchName string) (plumbing.ReferenceName, error) {
	branchConfig, err := repository.Branch(branchName)
	if err != nil {
		return "", fmt.Errorf("read branch config for %q: %w", branchName, err)
	}

	if branchConfig.Merge == "" {
		return "", fmt.Errorf("branch %q has no upstream branch", branchName)
	}

	if branchConfig.Remote == "" || branchConfig.Remote == "." {
		return branchConfig.Merge, nil
	}

	return plumbing.NewRemoteReferenceName(branchConfig.Remote, branchConfig.Merge.Short()), nil
}

func branchDeleteFlag(force bool) string {
	if force {
		return "-D"
	}

	return "-d"
}
