package gitwt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveCommand_Success(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "base.txt", "base")

	os.Chdir(repo)

	create := NewCreateCommand()
	create.SetArgs([]string{"to-remove"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	wtPath := repo + ".to-remove"
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree not created")
	}

	// Merge the branch back to main so it can be removed cleanly
	execGitInDir(t, wtPath, "commit", "--allow-empty", "-m", "wip")
	execGitInDir(t, repo, "merge", "to-remove", "-m", "merge")

	cmd := NewRemoveCommand()
	cmd.SetArgs([]string{"to-remove"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree directory %q still exists after remove", wtPath)
	}
}

func TestRemoveCommand_UnmergedFails(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "base.txt", "base")

	os.Chdir(repo)

	create := NewCreateCommand()
	create.SetArgs([]string{"unmerged"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	wtPath := repo + ".unmerged"
	execGitInDir(t, wtPath, "commit", "--allow-empty", "-m", "wip")

	cmd := NewRemoveCommand()
	cmd.SetArgs([]string{"unmerged"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected remove to fail on unmerged branch")
	}

	// Ensure worktree still exists
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatal("worktree should not have been removed")
	}
}

func TestRemoveCommand_Force(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "base.txt", "base")

	os.Chdir(repo)

	create := NewCreateCommand()
	create.SetArgs([]string{"forced"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	wtPath := repo + ".forced"
	execGitInDir(t, wtPath, "commit", "--allow-empty", "-m", "wip")

	cmd := NewRemoveCommand()
	cmd.SetArgs([]string{"forced", "-f"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("force remove failed: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree directory %q still exists after force remove", wtPath)
	}
}

func TestIsWorktreeClean_Clean(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "clean.txt", "clean")

	clean, err := isWorktreeClean(repo)
	if err != nil {
		t.Fatalf("isWorktreeClean: %v", err)
	}
	if !clean {
		t.Error("expected clean worktree")
	}
}

func TestIsWorktreeClean_Dirty(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "dirty.txt", "dirty")

	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new"), 0644)

	clean, err := isWorktreeClean(repo)
	if err != nil {
		t.Fatalf("isWorktreeClean: %v", err)
	}
	if clean {
		t.Error("expected dirty worktree")
	}
}

func TestBranchExists(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "exist.txt", "exist")
	os.Chdir(repo)

	if !branchExists("main") {
		t.Error("expected main branch to exist")
	}

	execGitInDir(t, repo, "branch", "test-branch")
	if !branchExists("test-branch") {
		t.Error("expected test-branch to exist")
	}
}

func TestWorktreeExists(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "wt.txt", "wt")
	os.Chdir(repo)

	absRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	exists, err := worktreeExists(absRepo)
	if err != nil {
		t.Fatalf("worktreeExists: %v", err)
	}
	if !exists {
		t.Error("expected main worktree to exist")
	}

	exists, err = worktreeExists("/nonexistent/path")
	if err != nil {
		t.Fatalf("worktreeExists: %v", err)
	}
	if exists {
		t.Error("expected nonexistent worktree to not exist")
	}
}
