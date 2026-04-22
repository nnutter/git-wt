package gitwt

import (
	"os"
	"testing"
)

func TestCreateCommand_Success(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "feat.txt", "feature")

	cmd := NewCreateCommand()
	cmd.SetArgs([]string{"feat-1"})
	os.Chdir(repo)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("create command failed: %v", err)
	}

	wtPath := repo + ".feat-1"
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree directory %q was not created", wtPath)
	}

	out, err := execGit("branch", "--list", "feat-1")
	if err != nil {
		t.Fatalf("check branch: %v", err)
	}
	if out == "" {
		t.Fatal("branch feat-1 was not created")
	}
}

func TestCreateCommand_WithUpstream(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "one.txt", "one")
	execGitInDir(t, repo, "checkout", "-b", "develop")
	commitFile(t, repo, "two.txt", "two")
	execGitInDir(t, repo, "checkout", "main")

	cmd := NewCreateCommand()
	cmd.SetArgs([]string{"feat-2", "-u", "develop"})
	os.Chdir(repo)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("create command failed: %v", err)
	}

	wtPath := repo + ".feat-2"
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree directory %q was not created", wtPath)
	}
}

func TestCreateCommand_BranchExists(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "a.txt", "a")
	execGitInDir(t, repo, "branch", "existing")

	cmd := NewCreateCommand()
	cmd.SetArgs([]string{"existing"})
	os.Chdir(repo)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected create to fail when branch exists")
	}
}

func TestCreateCommand_WorktreeExists(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "b.txt", "b")
	os.MkdirAll(repo+".already", 0755)

	cmd := NewCreateCommand()
	cmd.SetArgs([]string{"already"})
	os.Chdir(repo)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected create to fail when worktree directory exists")
	}
}

func TestBuildWorktreePath(t *testing.T) {
	tests := []struct {
		mainPath   string
		normalized string
		want       string
	}{
		{"/home/user/myrepo", "feat-1", "/home/user/myrepo.feat-1"},
		{"/home/user/myrepo", "nn.feat-1", "/home/user/myrepo.nn.feat-1"},
	}

	for _, tt := range tests {
		got := buildWorktreePath(tt.mainPath, tt.normalized)
		if got != tt.want {
			t.Errorf("buildWorktreePath(%q, %q) = %q, want %q", tt.mainPath, tt.normalized, got, tt.want)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feat-1", "feat-1"},
		{"nn/feat-1", "nn.feat-1"},
		{"a/b/c", "a.b.c"},
	}

	for _, tt := range tests {
		got := normalizeName(tt.input)
		if got != tt.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveUpstream(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "main.txt", "main")
	os.Chdir(repo)

	got, err := resolveUpstream("")
	if err == nil {
		// origin/HEAD might not exist in a bare repo
		t.Logf("resolved upstream: %q", got)
	}

	got, err = resolveUpstream("main")
	if err != nil {
		t.Fatalf("resolveUpstream(main) failed: %v", err)
	}
	if got != "main" {
		t.Errorf("resolveUpstream(main) = %q, want main", got)
	}
}

func TestCreateCommand_NormalizedName(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "norm.txt", "norm")

	cmd := NewCreateCommand()
	cmd.SetArgs([]string{"nn/feat-1"})
	os.Chdir(repo)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("create command failed: %v", err)
	}

	wtPath := repo + ".nn.feat-1"
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree directory %q was not created", wtPath)
	}
}
