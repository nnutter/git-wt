package timber

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateZshGeneratesWrapperCompletionAndAutoloadHelper(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	result := runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force")
	require.NoError(t, result.err, result.stderr)

	autoloadPath := filepath.Join(outDir, "_t_autoload")
	autoloadContents, err := os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "#autoload t", strings.SplitN(string(autoloadContents), "\n", 2)[0])

	completionPath := filepath.Join(outDir, "_t")
	completionContents, err := os.ReadFile(completionPath)
	require.NoError(t, err)
	assert.Equal(t, "#compdef t", strings.SplitN(string(completionContents), "\n", 2)[0])
	assert.NotContains(t, string(completionContents), "_foo() {")
	assert.Contains(t, string(completionContents), "_t() {")

	functionPath := filepath.Join(outDir, "t")
	functionContents, err := os.ReadFile(functionPath)
	require.NoError(t, err)
	assert.NotContains(t, string(functionContents), "foo() {")
	assert.Contains(t, string(functionContents), "t() {")
}

func TestGeneratedZshCompletionHasValidSyntax(t *testing.T) {
	t.Parallel()
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)

	output, err := exec.Command(zshPath, "-n", filepath.Join(outDir, "_t")).CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestGenerateZshUsesCustomWrapperName(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	result := runTimberCommand(t, "generate", "zsh", "--name", "foo", "--out", outDir)
	require.NoError(t, result.err, result.stderr)

	for _, defaultPath := range []string{"t", "_t", "_t_autoload"} {
		_, err := os.Stat(filepath.Join(outDir, defaultPath))
		require.ErrorIs(t, err, os.ErrNotExist, defaultPath)
	}

	autoloadContents, err := os.ReadFile(filepath.Join(outDir, "_foo_autoload"))
	require.NoError(t, err)
	assert.Equal(t, "#autoload foo", strings.SplitN(string(autoloadContents), "\n", 2)[0])

	completionContents, err := os.ReadFile(filepath.Join(outDir, "_foo"))
	require.NoError(t, err)
	assert.Equal(t, "#compdef foo", strings.SplitN(string(completionContents), "\n", 2)[0])
	assert.Contains(t, string(completionContents), "_foo() {")
	assert.NotContains(t, string(completionContents), "_t() {")

	functionContents, err := os.ReadFile(filepath.Join(outDir, "foo"))
	require.NoError(t, err)
	assert.Contains(t, string(functionContents), "foo() {")
	assert.NotContains(t, string(functionContents), "t() {")
}

func TestGeneratedCreateCompletesUniqueRepoPrefix(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	skipIfNoPty(t)

	home := t.TempDir()
	dataHome := filepath.Join(home, ".local", "share")
	worktreeRoot := filepath.Join(home, "worktrees")
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "timber", "repos", "timber.git"), 0o755))

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	scriptPath := filepath.Join(t.TempDir(), "complete.py")
	script := `import os, pty, select, time, sys

compdir = sys.argv[1]
zdot = sys.argv[2]
os.makedirs(zdot, exist_ok=True)
open(os.path.join(zdot, ".zshrc"), "w").write("")
os.environ["ZDOTDIR"] = zdot
os.environ["HOME"] = sys.argv[3]
os.environ["XDG_DATA_HOME"] = sys.argv[4]
os.environ["TIMBER_WORKTREE_ROOT"] = sys.argv[5]

pid, fd = pty.fork()
if pid == 0:
    os.execvp("zsh", ["zsh", "-f", "-i"])

def recv(timeout=1.0):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], max(0.05, end - time.time()))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        end = time.time() + 0.2
    return buf

def send(data):
    os.write(fd, data.encode() if isinstance(data, str) else data)

recv(0.3)
send("fpath=(" + compdir + " $fpath); autoload -Uz compinit; compinit -u -D\n")
recv(0.5)
send("zstyle ':completion:*' matcher-list 'm:{a-zA-Z}={A-Za-z}' 'r:|[._-]=* r:|=*' 'l:|=* r:|=*'\n")
recv(0.2)
send("\x15")
recv(0.1)
send("t create @t")
time.sleep(0.05)
send("\t")
output = recv(0.8).decode("utf-8", "replace")
send("exit\n")
recv(0.2)
sys.stdout.write(output)
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o644))

	command := exec.Command(
		"python3",
		scriptPath,
		outDir,
		filepath.Join(t.TempDir(), "zdot"),
		home,
		dataHome,
		worktreeRoot,
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "t create @timber")
}

func TestGeneratedSwitchCompletesWorktreeNamesAcrossRepos(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	skipIfNoPty(t)

	home := t.TempDir()
	dataHome := filepath.Join(home, ".local", "share")
	worktreeRoot := filepath.Join(home, "worktrees")
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "timber", "repos", "timber.git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "timber", "repos", "other.git"), 0o755))
	makeWorktree := func(repoName, worktreeName string) {
		t.Helper()
		worktreePath := filepath.Join(worktreeRoot, repoName, worktreeName, repoName)
		require.NoError(t, os.MkdirAll(worktreePath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(worktreePath, ".git"), nil, 0o644))
	}
	makeWorktree("timber", "feature/login")
	makeWorktree("other", "feature/api")

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	scriptPath := filepath.Join(t.TempDir(), "complete.py")
	script := `import os, pty, select, time, sys

compdir, zdot, home, data_home, worktree_root, line = sys.argv[1:7]
os.makedirs(zdot, exist_ok=True)
open(os.path.join(zdot, ".zshrc"), "w").write("")
os.environ["ZDOTDIR"] = zdot
os.environ["HOME"] = home
os.environ["XDG_DATA_HOME"] = data_home
os.environ["TIMBER_WORKTREE_ROOT"] = worktree_root

pid, fd = pty.fork()
if pid == 0:
    os.execvp("zsh", ["zsh", "-f", "-i"])

def recv(timeout=1.0):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], max(0.05, end - time.time()))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        end = time.time() + 0.2
    return buf

def send(data):
    os.write(fd, data.encode() if isinstance(data, str) else data)

recv(0.3)
send("cd " + home + "\n")
recv(0.2)
send("fpath=(" + compdir + " $fpath); autoload -Uz compinit; compinit -u -D\n")
recv(0.5)
send("\x15")
recv(0.1)
send(line)
time.sleep(0.05)
send("\t")
output = recv(0.8).decode("utf-8", "replace")
send("exit\n")
recv(0.2)
sys.stdout.write(output)
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o644))

	runComplete := func(line string) string {
		t.Helper()
		command := exec.Command(
			"python3",
			scriptPath,
			outDir,
			filepath.Join(t.TempDir(), "zdot"),
			home,
			dataHome,
			worktreeRoot,
			line,
		)
		output, err := command.CombinedOutput()
		require.NoError(t, err, string(output))
		return string(output)
	}

	unique := runComplete("t switch feature/l")
	assert.Contains(t, unique, "t switch feature/login")
	assert.NotContains(t, unique, "feature/login@")

	uniqueAlias := runComplete("t sw feature/l")
	assert.Contains(t, uniqueAlias, "t sw feature/login")
	assert.NotContains(t, uniqueAlias, "feature/login@")

	makeWorktree("other", "feature/login")
	ambiguous := runComplete("t switch feature/l")
	assert.Contains(t, ambiguous, "t switch feature/login@")
}

func TestGeneratedZshWrapperAutoloadsAfterCompinit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)

	binDir := t.TempDir()
	fakeTimber := `#!/bin/sh
printf '%s\n' "$@"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`fpath=("$1" $fpath)
autoload -Uz compinit
compinit -D 2>/dev/null
t list`,
		"--", outDir,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, "list", strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperChangesToRenamedCurrentWorktree(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	worktreeParent := t.TempDir()
	oldWorktree := filepath.Join(worktreeParent, "old")
	oldSubdirectory := filepath.Join(oldWorktree, "nested")
	require.NoError(t, os.MkdirAll(oldSubdirectory, 0o755))

	binDir := t.TempDir()
	fakeTimber := `#!/bin/sh
old_worktree=$(dirname "$PWD")
new_worktree=$(dirname "$old_worktree")/new
mv "$old_worktree" "$new_worktree" || exit $?
printf '%s\n' "$new_worktree/nested" > "$TIMBER_RENAME_PATH_FILE"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t repo rename old new >/dev/null; pwd -P`,
		"--", filepath.Join(outDir, "t"), oldSubdirectory,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(filepath.Join(worktreeParent, "new", "nested")), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperRestoresDirectoryOnFailure(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	startDir := t.TempDir()
	binDir := t.TempDir()
	fakeTimber := `#!/bin/sh
exit 17
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t remove feature@repo >/dev/null; exit_status=$?; printf '%s %s\n' "$exit_status" "$PWD"`,
		"--", filepath.Join(outDir, "t"), startDir,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, fmt.Sprintf("17 %s", canonicalPath(startDir)), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperChangesDirectoryOnSwitch(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	targetDir := t.TempDir()
	binDir := t.TempDir()
	fakeTimber := `#!/bin/sh
printf '%s\n' "$TIMBER_SWITCH_PATH_FILE_TARGET" > "$TIMBER_SWITCH_PATH_FILE"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`source "$1"; t switch feature@repo >/dev/null; pwd -P`,
		"--", filepath.Join(outDir, "t"),
	)
	command.Env = append(
		os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TIMBER_SWITCH_PATH_FILE_TARGET="+targetDir,
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(targetDir), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperLeavesImportedSourceDirectory(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	startDir := t.TempDir()
	sourceDir := t.TempDir()
	binDir := t.TempDir()
	fakeTimber := `#!/bin/sh
rm -rf "$3"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t repo import "$3" >/dev/null; printf '%s' "$PWD"`,
		"--", filepath.Join(outDir, "t"), startDir, sourceDir,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(os.Getenv("HOME")), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperRestoresDirectoryOnFailedImport(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	startDir := t.TempDir()
	sourceDir := t.TempDir()
	binDir := t.TempDir()
	fakeTimber := `#!/bin/sh
exit 17
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t repo import "$3" >/dev/null; exit_status=$?; printf '%s %s' "$exit_status" "$PWD"`,
		"--", filepath.Join(outDir, "t"), startDir, sourceDir,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, fmt.Sprintf("17 %s", canonicalPath(startDir)), strings.TrimSpace(string(output)))
}

func TestGenerateZshRefusesOverwriteWithoutForce(t *testing.T) {
	t.Parallel()
	outDir := t.TempDir()
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)
	result := runTimberCommand(t, "generate", "zsh", "--out", outDir)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
}

func TestGenerateZshChecksAutoloadHelperCollisionBeforeWriting(t *testing.T) {
	t.Parallel()
	outDir := t.TempDir()
	autoloadPath := filepath.Join(outDir, "_t_autoload")
	require.NoError(t, os.WriteFile(autoloadPath, []byte("existing helper\n"), 0o644))

	result := runTimberCommand(t, "generate", "zsh", "--out", outDir)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "autoload helper file")

	autoloadContents, err := os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "existing helper\n", string(autoloadContents))
	for _, untouchedPath := range []string{"t", "_t"} {
		_, err := os.Stat(filepath.Join(outDir, untouchedPath))
		require.ErrorIs(t, err, os.ErrNotExist, untouchedPath)
	}

	forceResult := runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force")
	require.NoError(t, forceResult.err, forceResult.stderr)
	autoloadContents, err = os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "#autoload t", strings.SplitN(string(autoloadContents), "\n", 2)[0])
}
