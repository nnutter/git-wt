package gitwt

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func createHerdrWorkspace(worktreePath string, label string) error {
	absolutePath, err := filepath.Abs(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree path for herdr: %w", err)
	}

	command := exec.Command("herdr", "workspace", "create", "--cwd", absolutePath, "--label", label)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return fmt.Errorf("herdr workspace create: %w", err)
		}
		return fmt.Errorf("herdr workspace create: %w: %s", err, message)
	}

	return nil
}
