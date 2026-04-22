package gitwt

import (
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feat-1", "feat-1"},
		{"nn/feat-1", "nn.feat-1"},
		{"a/b/c", "a.b.c"},
		{"no-slashes", "no-slashes"},
	}
	for _, tc := range tests {
		got := normalizeName(tc.input)
		if got != tc.want {
			t.Errorf("normalizeName(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestSiblingWorktreePath(t *testing.T) {
	tests := []struct {
		mainPath   string
		branchName string
		want       string
	}{
		{"/home/user/myrepo", "nn/feat-1", "/home/user/myrepo.nn.feat-1"},
		{"/home/user/myrepo", "feat-1", "/home/user/myrepo.feat-1"},
		{"/projects/foo", "a/b/c", "/projects/foo.a.b.c"},
	}
	for _, tc := range tests {
		got := siblingWorktreePath(tc.mainPath, tc.branchName)
		if got != tc.want {
			t.Errorf("siblingWorktreePath(%q, %q) = %q; want %q", tc.mainPath, tc.branchName, got, tc.want)
		}
	}
}

func TestParsePorcelainWorktrees(t *testing.T) {
	input := `worktree /home/user/myrepo
HEAD abc123
branch refs/heads/main

worktree /home/user/myrepo.nn.feat-1
HEAD def456
branch refs/heads/nn/feat-1

`
	result := parsePorcelainWorktrees(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(result))
	}
	if result[0].path != "/home/user/myrepo" {
		t.Errorf("result[0].path = %q; want /home/user/myrepo", result[0].path)
	}
	if result[1].branch != "refs/heads/nn/feat-1" {
		t.Errorf("result[1].branch = %q; want refs/heads/nn/feat-1", result[1].branch)
	}
}

func TestListManagedWorktrees_empty(t *testing.T) {
	repo := setupTestRepo(t)
	runInDir(t, repo.mainPath)

	worktrees, err := listManagedWorktrees(repo.mainPath)
	if err != nil {
		t.Fatalf("listManagedWorktrees: %v", err)
	}
	if len(worktrees) != 0 {
		t.Errorf("expected 0 managed worktrees, got %d", len(worktrees))
	}
}

func TestResolveDefaultUpstream(t *testing.T) {
	repo := setupTestRepo(t)

	upstream, err := resolveDefaultUpstream(repo.mainPath)
	if err != nil {
		t.Fatalf("resolveDefaultUpstream: %v", err)
	}
	if upstream != "origin/main" {
		t.Errorf("resolveDefaultUpstream = %q; want %q", upstream, "origin/main")
	}
}
