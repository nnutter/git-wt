package timber

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type gitCommandResult struct {
	stdout string
	stderr string
}

func gitOutput(runtime Runtime, directory string, args ...string) (gitCommandResult, error) {
	command := runtime.command(context.Background(), "git", args...)
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
