package gitwt

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// testRepo holds the paths needed for end-to-end tests.
type testRepo struct {
	// mainPath is the absolute path of the main worktree (the cloned repo).
	mainPath string
	// originPath is the absolute path of the bare remote (origin).
	originPath string
}

// projectRoot returns the absolute path of the module root (two levels up from
// internal/gitwt).
func projectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine source file path")
	}
	// filename is .../internal/gitwt/gitwt_testhelper_test.go
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// setupTestRepo creates a temporary bare "origin" repo and a clone of it
// under testdata/ inside the project root, makes an initial commit, and sets
// up refs/remotes/origin/HEAD.  The test will clean up via t.Cleanup.
func setupTestRepo(t *testing.T) testRepo {
	t.Helper()

	root := projectRoot(t)
	testdataDir := filepath.Join(root, "testdata")
	if err := os.MkdirAll(testdataDir, 0o755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}

	// Use t.TempDir scoped under testdata so that sibling worktrees created by
	// git-wt land inside the project directory as well.
	tmpDir, err := os.MkdirTemp(testdataDir, "test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	originPath := filepath.Join(tmpDir, "origin.git")
	mainPath := filepath.Join(tmpDir, "main")

	// Create the bare origin.
	mustGit(t, tmpDir, "init", "--bare", originPath)

	// Clone it.
	mustGit(t, tmpDir, "clone", originPath, mainPath)

	// Set user identity inside the clone so commits work.
	mustGit(t, mainPath, "config", "user.email", "test@example.com")
	mustGit(t, mainPath, "config", "user.name", "Test User")

	// Create an initial commit so the branch (main) exists.
	readmeFile := filepath.Join(mainPath, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustGit(t, mainPath, "add", "README.md")
	mustGit(t, mainPath, "commit", "-m", "initial commit")
	mustGit(t, mainPath, "push", "-u", "origin", "HEAD")

	// Ensure refs/remotes/origin/HEAD points to origin/main.
	mustGit(t, mainPath, "remote", "set-head", "origin", "--auto")

	return testRepo{mainPath: mainPath, originPath: originPath}
}

// mustGit runs a git command with Dir set to dir and fails the test on error.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// runInDir changes the process working directory for the duration of the test.
// This is necessary because resolveRepoDotGitDir uses os/exec without an explicit Dir.
func runInDir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Logf("restore chdir: %v", err)
		}
	})
}
