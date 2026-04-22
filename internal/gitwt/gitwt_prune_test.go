package gitwt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneCommand_removesOnlyMergedClean(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	// Create and merge feat-merged so its commit is in origin/main.
	createWorktreeAndMerge(t, repo, "feat-merged")

	// Create feat-unmerged and add a commit so HEAD is ahead of origin/main.
	create := &CreateCommand{}
	if err := create.Execute([]string{"feat-unmerged"}); err != nil {
		t.Fatalf("create feat-unmerged: %v", err)
	}
	unmergedWTPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-unmerged")
	if err := os.WriteFile(filepath.Join(unmergedWTPath, "work.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}
	mustGit(t, unmergedWTPath, "add", "work.txt")
	mustGit(t, unmergedWTPath, "commit", "-m", "unmerged work")

	prune := &PruneCommand{}
	if err := prune.Execute(nil); err != nil {
		t.Fatalf("prune Execute: %v", err)
	}

	// feat-merged should be gone.
	mergedPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-merged")
	if _, err := os.Stat(mergedPath); !os.IsNotExist(err) {
		t.Errorf("expected feat-merged worktree to be pruned, but it still exists")
	}

	// feat-unmerged should still exist.
	if _, err := os.Stat(unmergedWTPath); err != nil {
		t.Errorf("expected feat-unmerged worktree to remain, but got: %v", err)
	}
}

func TestPruneCommand_skipsUnclean(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	// Merged but dirty.
	createWorktreeAndMerge(t, repo, "feat-dirty-merged")
	wtPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-dirty-merged")
	// Stage a new file to make it dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatalf("write extra file: %v", err)
	}
	mustGit(t, wtPath, "add", "extra.txt")

	prune := &PruneCommand{}
	if err := prune.Execute(nil); err != nil {
		t.Fatalf("prune Execute: %v", err)
	}

	// Should still exist because it's dirty.
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("expected dirty worktree to remain, but got: %v", err)
	}
}

func TestPruneCommand_nothingToPrune(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	prune := &PruneCommand{}
	if err := prune.Execute(nil); err != nil {
		t.Fatalf("prune on empty repo: %v", err)
	}
}
