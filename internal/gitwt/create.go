package gitwt

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

type createCommand struct {
	upstream string
}

func NewCreateCommand() *cobra.Command {
	cmd := &createCommand{}
	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new worktree",
		Args:  cobra.ExactArgs(1),
		RunE:  cmd.Execute,
	}
	c.Flags().StringVarP(&cmd.upstream, "upstream", "u", "", "Upstream branch to base the new worktree on (defaults to origin/HEAD)")
	return c
}

func (c *createCommand) Execute(_ *cobra.Command, args []string) error {
	name := args[0]

	mainPath, err := mainWorktreePath()
	if err != nil {
		return err
	}

	normalized := normalizeName(name)
	worktreePath := buildWorktreePath(mainPath, normalized)

	if branchExists(name) {
		return fmt.Errorf("branch %q already exists", name)
	}

	exists, err := worktreeExists(worktreePath)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("worktree %q already exists", worktreePath)
	}

	if _, err := os.Stat(worktreePath); err == nil {
		return fmt.Errorf("worktree directory %q already exists", worktreePath)
	}

	upstreamBranch, err := resolveUpstream(c.upstream)
	if err != nil {
		return err
	}

	if err := addWorktree(name, worktreePath, upstreamBranch); err != nil {
		return err
	}

	if err := setUpstream(name, upstreamBranch); err != nil {
		return err
	}

	return nil
}

func buildWorktreePath(mainPath, normalized string) string {
	return mainPath + "." + normalized
}

func resolveUpstream(userUpstream string) (string, error) {
	if userUpstream != "" {
		return userUpstream, nil
	}
	return resolveOriginHead()
}

func addWorktree(name, path, upstream string) error {
	cmd := exec.Command("git", "worktree", "add", "-b", name, path, upstream)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w\n%s", err, string(out))
	}
	return nil
}

func setUpstream(branch, upstream string) error {
	cmd := exec.Command("git", "branch", "-u", upstream, branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch -u failed: %w\n%s", err, string(out))
	}
	return nil
}
