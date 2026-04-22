package gitwt

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	initGitRepo(t, tmpDir)
	commitFile(t, tmpDir, "README.md", "# test")
	execGitInDir(t, tmpDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/heads/main")
	return tmpDir
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	execGitInDir(t, dir, "init")
	execGitInDir(t, dir, "config", "user.email", "test@test.com")
	execGitInDir(t, dir, "config", "user.name", "Test User")
}

func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	execGitInDir(t, dir, "add", name)
	execGitInDir(t, dir, "commit", "-m", "add "+name)
}

func execGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %q: %v\n%s", args, dir, err, out)
	}
}
