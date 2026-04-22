package gitwt

import (
	"fmt"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"
)

func NewListCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: "List worktrees created by git-wt",
		RunE:  listExecute,
	}
	return c
}

func listExecute(_ *cobra.Command, _ []string) error {
	mainPath, err := mainWorktreePath()
	if err != nil {
		return err
	}

	worktrees, err := listGitWtWorktrees()
	if err != nil {
		return err
	}

	if len(worktrees) == 0 {
		return nil
	}

	renderWorktreeTable(mainPath, worktrees)
	return nil
}

func renderWorktreeTable(mainPath string, worktrees []struct {
	name string
	path string
	branch string
}) {
	rows := make([][]string, len(worktrees))
	for i, wt := range worktrees {
		rel, err := filepath.Rel(mainPath, wt.path)
		if err != nil {
			rel = wt.path
		}
		rows[i] = []string{wt.name, rel}
	}

	purp := lipgloss.Color("99")
	gray := lipgloss.Color("245")

	t := table.New().
		Border(lipgloss.NormalBorder()).
		Headers("NAME", "PATH").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return lipgloss.NewStyle().Foreground(purp).Bold(true).Padding(0, 1)
			}
			return lipgloss.NewStyle().Foreground(gray).Padding(0, 1)
		})

	// Apply minimal styling through huh theme if needed
	fmt.Println(t)
}
