package gitwt

import (
	"strings"
	"testing"
)

func TestListCommand_showsManagedWorktrees(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	// Create two worktrees.
	create := &CreateCommand{}
	if err := create.Execute([]string{"feat-1"}); err != nil {
		t.Fatalf("create feat-1: %v", err)
	}
	if err := create.Execute([]string{"nn/feat-2"}); err != nil {
		t.Fatalf("create nn/feat-2: %v", err)
	}

	worktrees, err := listManagedWorktrees(repo.mainPath)
	if err != nil {
		t.Fatalf("listManagedWorktrees: %v", err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("expected 2 managed worktrees, got %d", len(worktrees))
	}

	branches := make(map[string]bool)
	for _, wt := range worktrees {
		branches[wt.Branch] = true
	}
	if !branches["feat-1"] {
		t.Error("expected feat-1 in managed worktrees")
	}
	if !branches["nn/feat-2"] {
		t.Error("expected nn/feat-2 in managed worktrees")
	}
}

func TestListCommand_tableContainsBranches(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	create := &CreateCommand{}
	if err := create.Execute([]string{"feat-table"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	worktrees, err := listManagedWorktrees(repo.mainPath)
	if err != nil {
		t.Fatalf("listManagedWorktrees: %v", err)
	}

	mainPath, err := resolveMainWorktreePath(repo.mainPath)
	if err != nil {
		t.Fatalf("resolveMainWorktreePath: %v", err)
	}

	rendered := renderWorktreeTable(worktrees, mainPath)
	if !strings.Contains(rendered, "feat-table") {
		t.Errorf("rendered table does not contain 'feat-table':\n%s", rendered)
	}
}

func TestListCommand_excludesNonSiblings(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	// The main worktree itself should not appear.
	worktrees, err := listManagedWorktrees(repo.mainPath)
	if err != nil {
		t.Fatalf("listManagedWorktrees: %v", err)
	}
	for _, wt := range worktrees {
		if wt.Path == repo.mainPath {
			t.Errorf("main worktree should not appear in managed list")
		}
	}
}
