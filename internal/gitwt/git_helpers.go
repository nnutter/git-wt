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

// Layout (steady state):
//
//	<mainPath> = <root>/main/<repo>
//	managed    = <root>/<worktreeName>/<repo>
//
// Old main layout (basename mainPath == "main") is only handled by migrate.
func worktreeRoot(mainPath string) string {
	return filepath.Dir(filepath.Dir(mainPath))
}

func repoName(mainPath string) string {
	return filepath.Base(mainPath)
}

func managedWorktreePath(mainPath string, worktreeName string) string {
	return filepath.Join(worktreeRoot(mainPath), worktreeName, repoName(mainPath))
}

func mainNeedsLayoutMigration(mainPath string) bool {
	return filepath.Base(mainPath) == "main"
}

// migratedMainPath returns the nested main path when main is still on the old
// layout (<root>/main). repo name is the basename of the worktree root.
func migratedMainPath(mainPath string) string {
	root := filepath.Dir(mainPath)
	return filepath.Join(root, "main", filepath.Base(root))
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
