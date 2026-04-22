package gitwt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneCommand_RemovesMergedClean(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "base.txt", "base")
	os.Chdir(repo)

	create := NewCreateCommand()
	create.SetArgs([]string{"prune-me"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	wtPath := repo + ".prune-me"
	execGitInDir(t, wtPath, "commit", "--allow-empty", "-m", "wip")
	execGitInDir(t, repo, "merge", "prune-me", "-m", "merge")

	cmd := NewPruneCommand()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("merged/clean worktree %q should have been pruned", wtPath)
	}
}

func TestPruneCommand_SkipsUnmerged(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "base.txt", "base")
	os.Chdir(repo)

	create := NewCreateCommand()
	create.SetArgs([]string{"keep-me"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	wtPath := repo + ".keep-me"
	execGitInDir(t, wtPath, "commit", "--allow-empty", "-m", "wip")

	cmd := NewPruneCommand()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatal("unmerged worktree should not have been pruned")
	}
}

func TestPruneCommand_SkipsDirty(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "base.txt", "base")
	os.Chdir(repo)

	create := NewCreateCommand()
	create.SetArgs([]string{"dirty-wt"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	wtPath := repo + ".dirty-wt"
	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty"), 0644)

	cmd := NewPruneCommand()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatal("dirty worktree should not have been pruned")
	}
}
