package gitwt

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type removeCommand struct {
	force bool
}

func NewRemoveCommand() *cobra.Command {
	cmd := &removeCommand{}
	c := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a worktree and its branch",
		Args:  cobra.ExactArgs(1),
		RunE:  cmd.Execute,
	}
	c.Flags().BoolVarP(&cmd.force, "force", "f", false, "Force removal even if branch is unmerged or worktree is dirty")
	return c
}

func (r *removeCommand) Execute(_ *cobra.Command, args []string) error {
	name := args[0]

	mainPath, err := mainWorktreePath()
	if err != nil {
		return err
	}

	normalized := normalizeName(name)
	worktreePath := buildWorktreePath(mainPath, normalized)

	exists, err := worktreeExists(worktreePath)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("worktree %q does not exist", worktreePath)
	}

	if !r.force {
		if err := validateRemovable(name, worktreePath); err != nil {
			return err
		}
	}

	if err := removeWorktree(worktreePath); err != nil {
		return err
	}

	if err := warnIfDirectoryRemains(worktreePath); err != nil {
		return err
	}

	if err := deleteBranch(name, r.force); err != nil {
		return err
	}

	return nil
}

func validateRemovable(branch, worktreePath string) error {
	isClean, err := isWorktreeClean(worktreePath)
	if err != nil {
		return err
	}

	upstream, err := getBranchUpstream(branch)
	if err != nil {
		return err
	}

	isMerged, err := isBranchMerged(branch, upstream)
	if err != nil {
		return err
	}

	if !isClean || !isMerged {
		return fmt.Errorf("worktree %q is not clean or branch is not merged to upstream %q; use --force to override", worktreePath, upstream)
	}

	return nil
}

func getBranchUpstream(branch string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot get upstream for branch %q: %w", branch, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func removeWorktree(path string) error {
	cmd := exec.Command("git", "worktree", "remove", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %w\n%s", err, string(out))
	}
	return nil
}

func warnIfDirectoryRemains(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat worktree path: %w", err)
	}
	if info.IsDir() {
		fmt.Fprintf(os.Stderr, "Warning: worktree directory %q was not fully removed\n", path)
	}
	return nil
}

func deleteBranch(branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	cmd := exec.Command("git", "branch", flag, branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch %s failed: %w\n%s", flag, err, string(out))
	}
	return nil
}
