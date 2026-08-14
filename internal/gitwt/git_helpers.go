package gitwt

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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

func ensureWorktreeDirectory(worktreePath string) error {
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		return fmt.Errorf("create worktree directory %q: %w", worktreePath, err)
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

// displayHomePath replaces a leading home directory with "~" for display.
func displayHomePath(path string) string {
	home := os.Getenv("HOME")
	if home == "" {
		return path
	}

	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if relative, ok := strings.CutPrefix(cleanPath, prefix); ok {
		return "~" + string(filepath.Separator) + relative
	}
	return path
}

func branchDeleteFlag(force bool) string {
	if force {
		return "-D"
	}

	return "-d"
}

func isNotEmptyError(err error) bool {
	if err == nil {
		return false
	}
	if pathError, ok := errors.AsType[*fs.PathError](err); ok {
		err = pathError.Err
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not empty") || strings.Contains(message, "directory not empty")
}
