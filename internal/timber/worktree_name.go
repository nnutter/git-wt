package timber

import (
	"fmt"
	"math/rand/v2"
	"os"
)

const maxUnusedWorktreeNameAttempts = 64

func unusedWorktreeName(repoName string) (string, error) {
	return firstUnusedWorktreeName(randomWorktreeName, func(name string) (bool, error) {
		return worktreeDirectoryExists(repoName, name)
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
	adjective := worktreeNameAdjectives[rand.IntN(len(worktreeNameAdjectives))]
	noun := worktreeNameNouns[rand.IntN(len(worktreeNameNouns))]
	return formatWorktreeName(adjective, noun)
}

func formatWorktreeName(adjective string, noun string) string {
	return adjective + "-" + noun
}

func worktreeDirectoryExists(repoName string, name string) (bool, error) {
	path := managedWorktreePath(repoName, name)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("inspect worktree directory %q: %w", path, err)
}
