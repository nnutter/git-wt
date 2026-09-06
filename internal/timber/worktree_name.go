package timber

import (
	"fmt"
	"math/rand/v2"
)

const maxUnusedWorktreeNameAttempts = 64

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
