package gitwt

import (
	"fmt"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"
)

type ListCmd struct{}

func (l *ListCmd) Execute(_ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	output, err := gitWorktreeList(dir)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	entries := ParseWorktreeList(output)

	var mainPath string
	for _, e := range entries {
		if !e.IsBare && e.Branch != "" {
			mainPath = e.Path
			break
		}
	}
	if mainPath == "" {
		return fmt.Errorf("could not determine main worktree")
	}

	t := table.New().
		Headers("Name", "Path")

	for _, e := range entries {
		if e.IsBare || e.Branch == "" {
			continue
		}
		ok, branchName := IsGitWtWorktree(mainPath, e.Path)
		if !ok {
			continue
		}
		relPath, err := relPath(dir, e.Path)
		if err != nil {
			relPath = e.Path
		}
		_ = branchName
		t.Row(e.Branch, relPath)
	}

	fmt.Fprintln(os.Stdout, t.Render())
	return nil
}

func NewList() *cobra.Command {
	l := &ListCmd{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List worktrees",
		Long:  "List Git worktrees managed by git-wt.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return l.Execute(args)
		},
	}
	return cmd
}

func relPath(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	return rel, nil
}
