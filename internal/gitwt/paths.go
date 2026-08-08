package gitwt

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	reposDirName           = "git-wt/repos"
	worktreesDirName       = "worktrees"
	bareRepoSuffix         = ".git"
	worktreeRootEnvVarName = "GIT_WT_WORKTREE_ROOT"
)

func xdgDataHome() string {
	return cmp.Or(os.Getenv("XDG_DATA_HOME"), filepath.Join(os.Getenv("HOME"), ".local", "share"))
}

func reposDirectory() string {
	return filepath.Join(xdgDataHome(), reposDirName)
}

func worktreeRoot() string {
	return cmp.Or(os.Getenv(worktreeRootEnvVarName), filepath.Join(os.Getenv("HOME"), worktreesDirName))
}

func bareRepoPath(repoName string) string {
	return filepath.Join(reposDirectory(), repoName+bareRepoSuffix)
}

// normalizeRepoName strips a trailing ".git" so worktree paths use the short
// repo name (e.g. "roam") rather than the bare-dir style name ("roam.git").
func normalizeRepoName(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), bareRepoSuffix)
}

func managedWorktreePath(repoName string, worktreeName string) string {
	return filepath.Join(worktreeRoot(), worktreeName, repoName)
}

func ensureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}
