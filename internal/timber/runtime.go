package timber

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

// Runtime contains the process state and operating-system settings used by a
// timber command. A Runtime is captured once at the CLI boundary and should
// be treated as immutable for the lifetime of a command.
type Runtime struct {
	CurrentDirectory string

	HomeDirectory      string
	DataHome           string
	ConfigHome         string
	WorktreeRoot       string
	TemporaryDirectory string

	HerdrEnvironment bool

	CreatePathFile string
	SwitchPathFile string
	RenamePathFile string

	// Environment is passed to child processes such as git and herdr.
	Environment []string

	// HerdrExecutable overrides the executable used for Herdr commands. It is
	// primarily useful for tests; an empty value uses PATH lookup.
	HerdrExecutable string

	// TrashExecutable overrides the executable used to move removed paths to
	// the system trash. It is primarily useful for tests; an empty value uses
	// PATH lookup of "trash".
	TrashExecutable string
}

// RuntimeFromProcess captures the process state needed by a timber command.
func RuntimeFromProcess() (Runtime, error) {
	currentDirectory, err := os.Getwd()
	if err != nil {
		return Runtime{}, fmt.Errorf("get current directory: %w", err)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return Runtime{}, fmt.Errorf("resolve home directory: %w", err)
	}

	return Runtime{
		CurrentDirectory:   currentDirectory,
		HomeDirectory:      homeDirectory,
		DataHome:           cmp.Or(os.Getenv("XDG_DATA_HOME"), filepath.Join(homeDirectory, ".local", "share")),
		ConfigHome:         cmp.Or(os.Getenv("XDG_CONFIG_HOME"), filepath.Join(homeDirectory, ".config")),
		WorktreeRoot:       cmp.Or(os.Getenv(worktreeRootEnvVarName), filepath.Join(homeDirectory, worktreesDirName)),
		TemporaryDirectory: os.TempDir(),
		HerdrEnvironment:   os.Getenv("HERDR_ENV") == "1",
		CreatePathFile:     os.Getenv(createPathFileEnvVarName),
		SwitchPathFile:     os.Getenv(switchPathFileEnvVarName),
		RenamePathFile:     os.Getenv(repoRenamePathFileEnvVarName),
		Environment:        slices.Clone(os.Environ()),
	}, nil
}

func (x Runtime) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if name == "herdr" && x.HerdrExecutable != "" {
		name = x.HerdrExecutable
	}

	command := exec.CommandContext(ctx, name, args...)
	if x.Environment != nil {
		command.Env = slices.Clone(x.Environment)
	}
	return command
}

func (x Runtime) xdgDataHome() string {
	return x.DataHome
}

func (x Runtime) xdgConfigHome() string {
	return x.ConfigHome
}

func (x Runtime) reposDirectory() string {
	return filepath.Join(x.xdgDataHome(), reposDirName)
}

func (x Runtime) worktreeRoot() string {
	return x.WorktreeRoot
}

func (x Runtime) bareRepoPath(repoName string) string {
	return filepath.Join(x.reposDirectory(), repoName+bareRepoSuffix)
}

// managedWorktreePath returns
// <worktree-root>/<repo-name>/<worktree-name>/<repo-name>.
func (x Runtime) managedWorktreePath(repoName string, worktreeName string) string {
	return filepath.Join(x.worktreeRoot(), repoName, worktreeName, repoName)
}

func (x Runtime) temporaryPath(pattern string) (string, error) {
	return os.MkdirTemp(x.TemporaryDirectory, pattern)
}

func (x Runtime) absolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if x.CurrentDirectory == "" {
		return "", fmt.Errorf("current directory is not set")
	}
	return filepath.Abs(filepath.Join(x.CurrentDirectory, path))
}
