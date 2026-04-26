package gitwt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	
	repoDir := filepath.Join(dir, "myrepo")
	err := os.Mkdir(repoDir, 0755)
	if err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", "--initial-branch=main")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// config
	execGitInDirTest(t, repoDir, "config", "user.name", "Test User")
	execGitInDirTest(t, repoDir, "config", "user.email", "test@example.com")

	// create initial commit on main
	execGitInDirTest(t, repoDir, "commit", "--allow-empty", "-m", "Initial commit")

	// Create origin
	remoteDir := filepath.Join(dir, "remote.git")
	execGitInDirTest(t, dir, "git", "init", "--bare", "--initial-branch=main", remoteDir)
	execGitInDirTest(t, repoDir, "remote", "add", "origin", remoteDir)
	execGitInDirTest(t, repoDir, "push", "-u", "origin", "main")
	execGitInDirTest(t, repoDir, "remote", "set-head", "origin", "-a")

	return repoDir
}

func execGitInDirTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	// if args[0] is "git", it means we shouldn't add "git" again
	var cmd *exec.Cmd
	if args[0] == "git" {
		cmd = exec.Command(args[0], args[1:]...)
	} else {
		cmd = exec.Command("git", args...)
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func runGitWt(dir string, args ...string) error {
	cmd := NewRootCmd()
	cmd.SetArgs(args)

	cwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(cwd)

	return cmd.Execute()
}

func TestGitWtE2E(t *testing.T) {
	repoDir := setupRepo(t)

	// 1. Create worktree
	err := runGitWt(repoDir, "create", "feature/foo")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+".feature.foo")
	if stat, err := os.Stat(wtPath); err != nil || !stat.IsDir() {
		t.Fatalf("worktree directory not created correctly: %s", wtPath)
	}

	// Check if branch was created and upstream is set
	out := execGitInDirTest(t, repoDir, "branch", "-vv")
	if !strings.Contains(out, "feature/foo") || !strings.Contains(out, "[origin/main]") {
		t.Fatalf("branch not created or tracking not set properly: %s", out)
	}

	// 2. List worktrees
	err = runGitWt(repoDir, "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	// 3. Remove worktree (should fail because not merged)
	execGitInDirTest(t, wtPath, "commit", "--allow-empty", "-m", "feature commit")
	err = runGitWt(repoDir, "remove", "feature/foo")
	if err == nil {
		t.Fatalf("remove should have failed because not merged")
	}

	// 4. Force remove worktree
	err = runGitWt(repoDir, "remove", "-f", "feature/foo")
	if err != nil {
		t.Fatalf("force remove failed: %v", err)
	}

	// verify removed
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still exists after remove: %s", wtPath)
	}
	out = execGitInDirTest(t, repoDir, "branch")
	if strings.Contains(out, "feature/foo") {
		t.Fatalf("branch still exists after remove: %s", out)
	}

	// 5. Prune
	err = runGitWt(repoDir, "create", "feature/bar")
	if err != nil {
		t.Fatalf("create second feature failed: %v", err)
	}
	wtPathBar := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+".feature.bar")

	execGitInDirTest(t, wtPathBar, "commit", "--allow-empty", "-m", "bar commit")
	// merge into main so it's clean and merged
	execGitInDirTest(t, repoDir, "merge", "feature/bar")
	execGitInDirTest(t, repoDir, "push", "origin", "main")

	err = runGitWt(repoDir, "prune")
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(wtPathBar); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still exists after prune: %s", wtPathBar)
	}
}
