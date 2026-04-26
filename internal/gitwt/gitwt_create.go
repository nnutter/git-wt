package gitwt

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type CreateCmd struct {
	Name     string
	Upstream string
}

func (c *CreateCmd) Execute(args []string) error {
	c.Name = args[0]

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	upstream := c.Upstream
	if upstream == "" {
		upstream, err = gitResolveSymref(dir, "refs/remotes/origin/HEAD")
		if err != nil {
			return fmt.Errorf("failed to resolve upstream: %w", err)
		}
	}

	exists, err := gitBranchExists(dir, c.Name)
	if err != nil {
		return fmt.Errorf("failed to check if branch exists: %w", err)
	}
	if exists {
		return fmt.Errorf("branch %q already exists", c.Name)
	}

	path := WorktreePath(dir, c.Name)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("directory %q already exists", path)
	}

	if err := gitWorktreeAdd(dir, c.Name, path, upstream); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	if err := gitBranchSetUpstream(dir, c.Name, upstream); err != nil {
		printWarn("failed to set upstream for %q: %v", c.Name, err)
	}

	relPath, err := relPath(dir, path)
	if err != nil {
		relPath = path
	}

	printInfo("Created worktree %q at %s", c.Name, relPath)
	return nil
}

func NewCreate() *cobra.Command {
	c := &CreateCmd{}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new worktree",
		Long:  "Create a new Git worktree with a branch named <name> tracking <upstream_branch>.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.Execute(args)
		},
	}
	cmd.Flags().StringVarP(&c.Upstream, "upstream", "u", "", "upstream branch (default: origin/HEAD)")
	return cmd
}
