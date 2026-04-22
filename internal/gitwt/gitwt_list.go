package gitwt

import (
	"fmt"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"
)

// ListCommand holds the flags and arguments for the list subcommand.
type ListCommand struct{}

// Execute runs the list subcommand logic.
//
// Clean Code rules applied:
//   - Single Responsibility: formatting separated from data retrieval.
//   - Command Query Separation: Execute only performs the listing action.
func (l *ListCommand) Execute(args []string) error {
	repoPath, err := resolveRepoDotGitDir()
	if err != nil {
		return err
	}

	worktrees, err := listManagedWorktrees(repoPath)
	if err != nil {
		return err
	}

	mainPath, err := resolveMainWorktreePath(repoPath)
	if err != nil {
		return err
	}

	fmt.Print(renderWorktreeTable(worktrees, mainPath))
	return nil
}

// renderWorktreeTable formats managed worktrees as a styled table string.
// The path column shows paths relative to the parent of the main worktree.
func renderWorktreeTable(worktrees []ManagedWorktree, mainPath string) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	cellStyle := lipgloss.NewStyle().Padding(0, 1)
	headerCellStyle := cellStyle.Inherit(headerStyle)

	parentDir := filepath.Dir(mainPath)

	t := table.New().
		Headers("Branch", "Path").
		BorderHeader(true).
		BorderColumn(false).
		BorderRow(false).
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerCellStyle
			}
			return cellStyle
		})

	for _, wt := range worktrees {
		relPath, err := filepath.Rel(parentDir, wt.Path)
		if err != nil {
			relPath = wt.Path
		}
		t.Row(wt.Branch, relPath)
	}

	return t.Render() + "\n"
}

// NewListCommand constructs the cobra.Command for `git-wt list`.
func NewListCommand() *cobra.Command {
	l := &ListCommand{}
	return &cobra.Command{
		Use:   "list",
		Short: "List managed worktrees",
		Long: `List all Git worktrees managed by git-wt.

Only worktrees that are siblings of the main worktree and follow the dot-suffix
naming convention are shown.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return l.Execute(args)
		},
	}
}
