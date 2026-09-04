package timber

import (
	"fmt"
	"os"
	"strings"
)

const (
	reposDirName           = "timber/repos"
	worktreesDirName       = "worktrees"
	bareRepoSuffix         = ".git"
	worktreeRootEnvVarName = "TIMBER_WORKTREE_ROOT"
)

// normalizeRepoName strips a trailing ".git" so worktree paths use the short
// repo name (e.g. "roam") rather than the bare-dir style name ("roam.git").
func normalizeRepoName(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), bareRepoSuffix)
}

func ensureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}
