package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneMerged(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "prune-merged"); err != nil {
		t.Fatalf("create prune-merged: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.prune-merged")

	runGit(t, wtPath, "commit", "--allow-empty", "-m", "work on feature")
	runGit(t, repoDir, "merge", "prune-merged")

	if err := executeCmd(t, "prune"); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, err := os.Stat(wtPath); err == nil {
		t.Errorf("worktree directory %q should not exist after prune", wtPath)
	}
}

func TestPruneSkipsUnmerged(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "prune-unmerged"); err != nil {
		t.Fatalf("create prune-unmerged: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.prune-unmerged")
	runGit(t, wtPath, "commit", "--allow-empty", "-m", "unmerged work")

	if err := executeCmd(t, "prune"); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Errorf("unmerged worktree directory %q should still exist after prune without --prompt", wtPath)
	}
}
