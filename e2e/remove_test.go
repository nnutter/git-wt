package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemove(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "remove-test"); err != nil {
		t.Fatalf("create remove-test: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.remove-test")
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree directory %q should exist before remove", wtPath)
	}

	if err := executeCmd(t, "remove", "remove-test"); err != nil {
		t.Fatalf("remove remove-test: %v", err)
	}

	if _, err := os.Stat(wtPath); err == nil {
		t.Errorf("worktree directory %q should not exist after remove", wtPath)
	}
}

func TestRemoveUnmergedFails(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "unmerged-test"); err != nil {
		t.Fatalf("create unmerged-test: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.unmerged-test")
	runGit(t, wtPath, "commit", "--allow-empty", "-m", "unmerged work")

	err := executeCmd(t, "remove", "unmerged-test")
	if err == nil {
		t.Fatal("expected error removing unmerged worktree, got nil")
	}
}

func TestRemoveForce(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "force-test"); err != nil {
		t.Fatalf("create force-test: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.force-test")
	runGit(t, wtPath, "commit", "--allow-empty", "-m", "unmerged work")

	if err := executeCmd(t, "remove", "-f", "force-test"); err != nil {
		t.Fatalf("remove -f force-test: %v", err)
	}

	if _, err := os.Stat(wtPath); err == nil {
		t.Errorf("worktree directory %q should not exist after force remove", wtPath)
	}
}
