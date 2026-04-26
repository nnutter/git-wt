package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nnutter/git-wt/internal/gitwt"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()

	projectRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := filepath.Join(projectRoot, ".tmp", t.Name())
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	originDir := filepath.Join(tmpDir, "origin.git")

	runGit(t, tmpDir, "init", "--bare", "-b", "main", originDir)

	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "config", "user.email", "test@test.com")
	runGit(t, repoDir, "config", "user.name", "Test")
	runGit(t, repoDir, "remote", "add", "origin", originDir)
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "initial")
	runGit(t, repoDir, "push", "-u", "origin", "main")

	runGit(t, originDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runGit(t, repoDir, "fetch", "origin")

	t.Chdir(repoDir)

	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func executeCmd(t *testing.T, args ...string) error {
	t.Helper()
	RootCmd := gitwt.RootCmd
	RootCmd.SetArgs(args)
	err := RootCmd.Execute()
	RootCmd.SetArgs(nil)
	return err
}
