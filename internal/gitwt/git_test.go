package gitwt

import (
	"os/exec"
	"testing"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
}

func TestGitBranchExists(t *testing.T) {
	skipIfNoGit(t)

	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "commit", "--allow-empty", "-m", "initial")
	runGit(t, dir, "branch", "test-branch")

	exists, err := gitBranchExists(dir, "test-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected branch to exist")
	}

	exists, err = gitBranchExists(dir, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected branch not to exist")
	}
}

func TestGitWorkdirClean(t *testing.T) {
	skipIfNoGit(t)

	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "commit", "--allow-empty", "-m", "initial")

	clean, err := gitWorkdirClean(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clean {
		t.Error("expected workdir to be clean")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}
