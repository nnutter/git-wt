package gitwt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateCommand_basicCreate(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	cmd := &CreateCommand{}
	if err := cmd.Execute([]string{"feat-1"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	expectedPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-1")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("expected worktree path to exist at %s: %v", expectedPath, err)
	}
}

func TestCreateCommand_withSlashInName(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	cmd := &CreateCommand{}
	if err := cmd.Execute([]string{"nn/feat-1"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	expectedPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".nn.feat-1")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("expected worktree path to exist at %s: %v", expectedPath, err)
	}
}

func TestCreateCommand_duplicatePathFails(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	cmd := &CreateCommand{}
	if err := cmd.Execute([]string{"feat-1"}); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	// Second create with same name must fail.
	if err := cmd.Execute([]string{"feat-1"}); err == nil {
		t.Error("expected error on duplicate create, got nil")
	}
}

func TestCreateCommand_customUpstream(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	cmd := &CreateCommand{upstream: "origin/main"}
	if err := cmd.Execute([]string{"feat-custom"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	expectedPath := filepath.Join(filepath.Dir(repo.mainPath), filepath.Base(repo.mainPath)+".feat-custom")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("expected worktree path at %s: %v", expectedPath, err)
	}
}

func TestCreateCommand_setsUpstreamTracking(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	cmd := &CreateCommand{}
	if err := cmd.Execute([]string{"feat-track"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	upstream, err := resolveBranchUpstream(repo.mainPath, "feat-track")
	if err != nil {
		t.Fatalf("resolveBranchUpstream: %v", err)
	}
	if upstream != "origin/main" {
		t.Errorf("upstream = %q; want origin/main", upstream)
	}
}
