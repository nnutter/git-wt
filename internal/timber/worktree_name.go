package timber

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
)

const maxUnusedWorktreeNameAttempts = 64

func (x Runtime) unusedWorktreeName(repoName string) (string, error) {
	return firstUnusedWorktreeName(randomWorktreeName, func(name string) (bool, error) {
		return x.worktreeDirectoryExists(repoName, name)
	})
}

func firstUnusedWorktreeName(
	generate func() string,
	exists func(string) (bool, error),
) (string, error) {
	for range maxUnusedWorktreeNameAttempts {
		name := generate()
		taken, err := exists(name)
		if err != nil {
			return "", err
		}
		if !taken {
			return name, nil
		}
	}
	return "", fmt.Errorf("could not generate an unused worktree name")
}

func randomWorktreeName() string {
	adjective := worktreeNameAdjectives[rand.IntN(len(worktreeNameAdjectives))] // #nosec G404 -- generated names are not security-sensitive
	noun := worktreeNameNouns[rand.IntN(len(worktreeNameNouns))]                // #nosec G404 -- generated names are not security-sensitive
	return formatWorktreeName(adjective, noun)
}

func formatWorktreeName(adjective string, noun string) string {
	return adjective + "-" + noun
}

func (x Runtime) worktreeDirectoryExists(repoName string, name string) (bool, error) {
	path := x.managedWorktreePath(repoName, name)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("inspect worktree directory %q: %w", path, err)
}
