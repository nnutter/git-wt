package gitwt

import (
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"feat-1", "feat-1"},
		{"nn/feat-1", "nn.feat-1"},
		{"a/b/c", "a.b.c"},
		{"no-slash", "no-slash"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeName(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestWorktreePath(t *testing.T) {
	tests := []struct {
		mainPath string
		name     string
		expected string
	}{
		{"/home/user/myrepo", "feat-1", "/home/user/myrepo.feat-1"},
		{"/home/user/myrepo", "nn/feat-1", "/home/user/myrepo.nn.feat-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WorktreePath(tt.mainPath, tt.name)
			if got != tt.expected {
				t.Errorf("WorktreePath(%q, %q) = %q, want %q", tt.mainPath, tt.name, got, tt.expected)
			}
		})
	}
}

func TestIsGitWtWorktree(t *testing.T) {
	tests := []struct {
		mainPath     string
		worktreePath string
		isWt         bool
		branchName   string
	}{
		{"/home/user/myrepo", "/home/user/myrepo.feat-1", true, "feat-1"},
		{"/home/user/myrepo", "/home/user/myrepo.nn.feat-1", true, "nn/feat-1"},
		{"/home/user/myrepo", "/home/user/other", false, ""},
		{"/home/user/myrepo", "/opt/user/myrepo.feat-1", false, ""},
		{"/home/user/myrepo", "/home/user/myrepo", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.worktreePath, func(t *testing.T) {
			isWt, branchName := IsGitWtWorktree(tt.mainPath, tt.worktreePath)
			if isWt != tt.isWt || branchName != tt.branchName {
				t.Errorf("IsGitWtWorktree(%q, %q) = (%v, %q), want (%v, %q)", tt.mainPath, tt.worktreePath, isWt, branchName, tt.isWt, tt.branchName)
			}
		})
	}
}

func TestParseWorktreeList(t *testing.T) {
	input := `worktree /home/user/myrepo
HEAD abc123def456
branch refs/heads/main

worktree /home/user/myrepo.nn.feat-1
HEAD def456abc123
branch refs/heads/nn/feat-1

worktree /home/user/myrepo-bare
HEAD abc123def456
bare

`

	entries := ParseWorktreeList(input)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Path != "/home/user/myrepo" {
		t.Errorf("entry[0].Path = %q, want /home/user/myrepo", entries[0].Path)
	}
	if entries[0].Branch != "main" {
		t.Errorf("entry[0].Branch = %q, want main", entries[0].Branch)
	}
	if entries[0].IsBare {
		t.Errorf("entry[0].IsBare = true, want false")
	}

	if entries[1].Path != "/home/user/myrepo.nn.feat-1" {
		t.Errorf("entry[1].Path = %q, want /home/user/myrepo.nn.feat-1", entries[1].Path)
	}
	if entries[1].Branch != "nn/feat-1" {
		t.Errorf("entry[1].Branch = %q, want nn/feat-1", entries[1].Branch)
	}

	if !entries[2].IsBare {
		t.Errorf("entry[2].IsBare = false, want true")
	}
}
