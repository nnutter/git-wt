package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreate(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "feat-1"); err != nil {
		t.Fatalf("create feat-1: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.feat-1")
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Errorf("worktree directory %q does not exist", wtPath)
	}
}

func TestCreateWithSlash(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "nn/feat-1"); err != nil {
		t.Fatalf("create nn/feat-1: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.nn.feat-1")
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Errorf("worktree directory %q does not exist", wtPath)
	}
}

func TestCreateWithUpstream(t *testing.T) {
	repoDir := setupTestRepo(t)

	if err := executeCmd(t, "create", "feat-2", "-u", "origin/main"); err != nil {
		t.Fatalf("create feat-2 -u origin/main: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), "myrepo.feat-2")
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Errorf("worktree directory %q does not exist", wtPath)
	}
}

func TestCreateDuplicateBranch(t *testing.T) {
	setupTestRepo(t)

	if err := executeCmd(t, "create", "dup-test"); err != nil {
		t.Fatalf("create dup-test: %v", err)
	}

	err := executeCmd(t, "create", "dup-test")
	if err == nil {
		t.Fatal("expected error for duplicate branch, got nil")
	}
}

func TestCreateDuplicateDirectory(t *testing.T) {
	setupTestRepo(t)

	if err := executeCmd(t, "create", "dir-test"); err != nil {
		t.Fatalf("create dir-test: %v", err)
	}

	err := executeCmd(t, "create", "dir-test")
	if err == nil {
		t.Fatal("expected error for duplicate directory, got nil")
	}
}
