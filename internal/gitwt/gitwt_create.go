package gitwt

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

// CreateCommand holds the flags and arguments for the create subcommand.
type CreateCommand struct {
	upstream string
}

// Execute runs the create subcommand logic.
// args[0] is the branch name to create.
//
// Clean Code rules applied:
//   - Single Responsibility: each step delegated to small helper functions.
//   - Intention-revealing names: upstream, branchName, worktreePath.
//   - Fail fast on invalid preconditions.
func (c *CreateCommand) Execute(args []string) error {
	branchName := args[0]

	repoPath, err := resolveRepoDotGitDir()
	if err != nil {
		return err
	}

	mainPath, err := resolveMainWorktreePath(repoPath)
	if err != nil {
		return err
	}

	upstream, err := c.resolveUpstream(repoPath)
	if err != nil {
		return err
	}

	worktreePath := siblingWorktreePath(mainPath, branchName)

	if err := c.validateWorktreeDoesNotExist(worktreePath, branchName); err != nil {
		return err
	}

	if err := c.addWorktree(mainPath, branchName, worktreePath, upstream); err != nil {
		return err
	}

	if err := c.setUpstreamTracking(mainPath, branchName, upstream); err != nil {
		return err
	}

	printStatus(os.Stderr, fmt.Sprintf("Created worktree %q at %s tracking %s", branchName, worktreePath, upstream))
	return nil
}

func (c *CreateCommand) resolveUpstream(repoPath string) (string, error) {
	if c.upstream != "" {
		return c.upstream, nil
	}
	return resolveDefaultUpstream(repoPath)
}

func (c *CreateCommand) validateWorktreeDoesNotExist(worktreePath, branchName string) error {
	if pathExists(worktreePath) {
		return fmt.Errorf("worktree path already exists: %s", worktreePath)
	}
	return nil
}

func (c *CreateCommand) addWorktree(mainPath, branchName, worktreePath, upstream string) error {
	err := runGitCommand(mainPath, "worktree", "add", "-b", branchName, worktreePath, upstream)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	return nil
}

func (c *CreateCommand) setUpstreamTracking(mainPath, branchName, upstream string) error {
	err := runGitCommand(mainPath, "branch", "--set-upstream-to="+upstream, branchName)
	if err != nil {
		return fmt.Errorf("set upstream tracking: %w", err)
	}
	return nil
}

// NewCreateCommand constructs the cobra.Command for `git-wt create`.
func NewCreateCommand() *cobra.Command {
	c := &CreateCommand{}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new worktree for a branch",
		Long: `Create a new Git worktree as a sibling directory of the main worktree.

The worktree path is derived from <name> by replacing '/' with '.', creating
a sibling directory. For example, if the main repo is at ~/myrepo and <name>
is "nn/feat-1", the worktree is created at ~/myrepo.nn.feat-1.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.Execute(args)
		},
	}
	cmd.Flags().StringVarP(&c.upstream, "upstream", "u", "", "upstream branch to base the new branch on (default: origin/HEAD)")
	return cmd
}

// printStatus writes a styled status message to the given writer (intended for os.Stderr).
func printStatus(w io.Writer, message string) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	fmt.Fprintln(w, style.Render(message))
}

// printWarning writes a styled warning message to the given writer (intended for os.Stderr).
func printWarning(w io.Writer, message string) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	fmt.Fprintln(w, style.Render("Warning: "+message))
}

// printError writes a styled error message to the given writer (intended for os.Stderr).
func printError(w io.Writer, message string) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	fmt.Fprintln(w, style.Render("Error: "+message))
}
