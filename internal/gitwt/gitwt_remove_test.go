package gitwt

import (
	"os"
	"path/filepath"
	"testing"
)

// createWorktreeAndMerge is a test helper that creates a worktree, makes a commit,
// merges it into main, and pushes so that origin/main tracks the merged state.
func createWorktreeAndMerge(t *testing.T, repo testRepo, branchName string) {
	t.Helper()
	create := &CreateCommand{}
	if err := create.Execute([]string{branchName}); err != nil {
		t.Fatalf("create %s: %v", branchName, err)
	}

	// Make a commit in the new worktree so the branch diverges from origin/main.
	wtPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+"."+normalizeName(branchName))
	newFile := filepath.Join(wtPath, normalizeName(branchName)+".txt")
	if err := os.WriteFile(newFile, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	mustGit(t, wtPath, "add", normalizeName(branchName)+".txt")
	mustGit(t, wtPath, "commit", "-m", "feat commit")

	// Merge into main.
	mustGit(t, repo.mainPath, "merge", "--no-ff", branchName, "-m", "merge "+branchName)

	// Push so origin/main advances to include the merged commit.
	mustGit(t, repo.mainPath, "push", "origin", "main")
}

func TestRemoveCommand_removesWorktreeAndBranch(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	createWorktreeAndMerge(t, repo, "feat-remove")

	cmd := &RemoveCommand{}
	if err := cmd.Execute([]string{"feat-remove"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Worktree path should be gone.
	wtPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-remove")
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("expected worktree path %s to be removed", wtPath)
	}

	// Branch should be gone.
	worktrees, _ := listManagedWorktrees(repo.mainPath)
	for _, wt := range worktrees {
		if wt.Branch == "feat-remove" {
			t.Error("feat-remove branch/worktree still present after remove")
		}
	}
}

func TestRemoveCommand_failsIfUnmerged(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	// Create the worktree and add a commit so HEAD is ahead of origin/main.
	create := &CreateCommand{}
	if err := create.Execute([]string{"feat-unmerged"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-unmerged")
	if err := os.WriteFile(filepath.Join(wtPath, "work.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}
	mustGit(t, wtPath, "add", "work.txt")
	mustGit(t, wtPath, "commit", "-m", "unmerged work")

	cmd := &RemoveCommand{}
	if err := cmd.Execute([]string{"feat-unmerged"}); err == nil {
		t.Error("expected error removing unmerged worktree without --force, got nil")
	}
}

func TestRemoveCommand_forceRemovesUnmerged(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	create := &CreateCommand{}
	if err := create.Execute([]string{"feat-force"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	cmd := &RemoveCommand{force: true}
	if err := cmd.Execute([]string{"feat-force"}); err != nil {
		t.Fatalf("Execute --force: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-force")
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("expected worktree path %s to be removed after --force", wtPath)
	}
}

func TestRemoveCommand_failsIfDirty(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	create := &CreateCommand{}
	if err := create.Execute([]string{"feat-dirty"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Make the worktree dirty (untracked file creates dirty status via go-git).
	wtPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-dirty")
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	// Stage it to make status show modified.
	mustGit(t, wtPath, "add", "dirty.txt")

	cmd := &RemoveCommand{}
	if err := cmd.Execute([]string{"feat-dirty"}); err == nil {
		t.Error("expected error removing dirty worktree without --force, got nil")
	}
}
