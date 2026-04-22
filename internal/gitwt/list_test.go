package gitwt

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestListCommand(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "main.txt", "main")

	os.Chdir(repo)

	// Create two worktrees
	cmd1 := NewCreateCommand()
	cmd1.SetArgs([]string{"wt-1"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("create wt-1 failed: %v", err)
	}

	cmd2 := NewCreateCommand()
	cmd2.SetArgs([]string{"wt-2"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("create wt-2 failed: %v", err)
	}

	// Create a non-git-wt worktree manually
	otherDir := t.TempDir()
	otherRepo := otherDir + "/other"
	os.MkdirAll(otherRepo, 0755)
	initGitRepo(t, otherRepo)
	commitFile(t, otherRepo, "other.txt", "other")
	// Not a sibling with dot suffix, so should be ignored by listGitWtWorktrees

	listCmd := NewListCommand()
	output := captureStdout(t, listCmd)

	if !strings.Contains(output, "wt-1") {
		t.Errorf("list output missing wt-1: %q", output)
	}
	if !strings.Contains(output, "wt-2") {
		t.Errorf("list output missing wt-2: %q", output)
	}
}

func TestListCommand_Empty(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "main.txt", "main")
	os.Chdir(repo)

	listCmd := NewListCommand()
	output := captureStdout(t, listCmd)

	if strings.TrimSpace(output) != "" {
		t.Errorf("expected empty output, got: %q", output)
	}
}

func captureStdout(t *testing.T, cmd *cobra.Command) string {
	t.Helper()
	// Redirect stdout to capture table output
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	// For cobra commands, we need to set output
	cmd.SetOut(w)
	if err := cmd.Execute(); err != nil {
		os.Stdout = oldStdout
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}
