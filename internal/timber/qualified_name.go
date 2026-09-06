package timber

import (
	"fmt"
	"strings"
)

type qualifiedName struct {
	Name string
	Repo string
}

func rejectAtInWorktreeName(name string) error {
	if strings.Contains(name, "@") {
		return fmt.Errorf("worktree name %q must not contain @", name)
	}
	return nil
}

func nameArgs(name string) []string {
	if name == "" {
		return nil
	}
	return []string{name}
}
