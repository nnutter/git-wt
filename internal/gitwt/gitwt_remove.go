package gitwt

import (
"errors"
"os"
"os/exec"
"strings"

"github.com/spf13/cobra"
)

type RemoveCmd struct {
	args struct {
		name string
	}
	flags struct {
		force bool
	}
}

func NewRemoveCmd() *cobra.Command {
	cmd := &RemoveCmd{}
	c := &cobra.Command{
		Use:   `remove [-f|--force] <name>`,
		Short: `Remove a Git worktree`,
		Args:  cobra.ExactArgs(1),
		RunE:  cmd.Execute,
	}
	c.Flags().BoolVarP(&cmd.flags.force, `force`, `f`, false, `Force removal even if branch is unmerged or workdir is unclean`)
	return c
}

func (c *RemoveCmd) Execute(cmd *cobra.Command, args []string) error {
	c.args.name = args[0]
	mainDir, err := mainWorktreeDir()
	if err != nil {
		return err
	}
	worktreePath := siblingPath(mainDir, c.args.name)
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return errors.New(`worktree not found: ` + worktreePath)
	} else if err != nil {
		return err
	}
	branch := c.args.name
	if err := checkMergedAndClean(branch, worktreePath, c.flags.force); err != nil {
		return err
	}
	if err := removeWorktree(worktreePath, c.flags.force); err != nil {
		return err
	}
	if err := deleteBranch(branch, c.flags.force); err != nil {
		warningf(`failed to delete branch %s: %v`, branch, err)
	}
	return nil
}

func checkMergedAndClean(branch, worktreePath string, force bool) error {
	upstream, err := getUpstream(branch)
	if err != nil {
		if force {
			return nil
		}
		return err
	}
	merged, err := isAncestor(branch, upstream)
	if err != nil {
		return err
	}
	if !merged {
		if force {
			return nil
		}
		return errors.New(`branch ` + branch + ` has not been merged to upstream ` + upstream)
	}
	clean, err := isWorktreeClean(worktreePath)
	if err != nil {
		return err
	}
	if !clean {
		if force {
			return nil
		}
		return errors.New(`worktree at ` + worktreePath + ` is not clean`)
	}
	return nil
}

func getUpstream(branch string) (string, error) {
	cmd := exec.Command(`git`, `rev-parse`, `--symbolic-full-name`, branch+`@{upstream}`)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return ``, errors.New(`branch ` + branch + ` has no upstream set`)
	}
	upstream := strings.TrimSpace(string(output))
	upstream = strings.TrimPrefix(upstream, `refs/remotes/`)
	return upstream, nil
}

func isAncestor(branch, upstream string) (bool, error) {
	cmd := exec.Command(`git`, `merge-base`, `--is-ancestor`, branch, upstream)
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func isWorktreeClean(worktreePath string) (bool, error) {
	cmd := exec.Command(`git`, `-C`, worktreePath, `status`, `--porcelain`)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(output)) == ``, nil
}

func removeWorktree(worktreePath string, force bool) error {
	args := []string{`worktree`, `remove`, worktreePath}
	if force {
		args = append(args, `--force`)
	}
	cmd := exec.Command(`git`, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if _, err := os.Stat(worktreePath); err == nil {
		warningf(`worktree directory was not removed: %s`, worktreePath)
	}
	return nil
}

func deleteBranch(branch string, force bool) error {
	args := []string{`branch`}
	if force {
		args = append(args, `-D`)
	} else {
		args = append(args, `-d`)
	}
	args = append(args, branch)
	cmd := exec.Command(`git`, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}