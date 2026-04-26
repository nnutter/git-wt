package gitwt

import (
	"fmt"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"
)

type listOptions struct{}

func NewListCmd() *cobra.Command {
	opts := &listOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Git worktrees managed by git-wt",
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.Execute(args)
		},
	}

	return cmd
}

func (o *listOptions) Execute(args []string) error {
	worktrees, err := getGitWtWorktrees()
	if err != nil {
		return fmt.Errorf("failed to get worktrees: %w", err)
	}

	if len(worktrees) == 0 {
		fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("No git-wt worktrees found."))
		return nil
	}

	// Make paths relative to main worktree parent or current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("238"))).
		Headers("WORKTREE", "PATH").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Padding(0, 1)
			}
			st := lipgloss.NewStyle().Padding(0, 1)
			if col == 0 {
				return st.Foreground(lipgloss.Color("39"))
			}
			return st.Foreground(lipgloss.Color("246"))
		})

	for _, wt := range worktrees {
		relPath, err := filepath.Rel(cwd, wt.Path)
		if err != nil {
			relPath = wt.Path
		}
		t.Row(wt.Branch, relPath)
	}

	fmt.Fprintln(os.Stdout, t.Render())

	return nil
}
