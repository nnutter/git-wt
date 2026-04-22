// Package gitwt implements the git-wt command and all its subcommands.
package gitwt

import (
	"github.com/spf13/cobra"
)

// RootCommand is the top-level cobra.Command for git-wt.
// fang.Execute wraps this to add completion, man, and styled help.
var RootCommand = &cobra.Command{
	Use:   "git-wt",
	Short: "Manage Git worktrees",
	Long: `git-wt manages Git worktrees as siblings of the main worktree.

Worktree paths are derived from the branch name by replacing '/' with '.',
producing a sibling directory next to the main repository.`,
}

func init() {
	RootCommand.AddCommand(NewCreateCommand())
	RootCommand.AddCommand(NewListCommand())
	RootCommand.AddCommand(NewRemoveCommand())
	RootCommand.AddCommand(NewPruneCommand())
}
