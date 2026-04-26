package gitwt

import (
"errors"
"fmt"
"os"
"os/exec"
"strings"

"github.com/spf13/cobra"
)

type CreateCmd struct {
	args struct {
		name string
	}
	flags struct {
		upstream string
	}
}

func NewCreateCmd() *cobra.Command {
	cmd := &CreateCmd{}
	c := &cobra.Command{
		Use:   `create <name> [-u|--upstream <upstream_branch>]`,
		Short: `Create a new Git worktree`,
		Args:  cobra.ExactArgs(1),
		RunE:  cmd.Execute,
	}
	c.Flags().StringVarP(&cmd.flags.upstream, `upstream`, `u`, ``, `Upstream branch (default: origin/HEAD)`)
	return c
}

func (c *CreateCmd) Execute(cmd *cobra.Command, args []string) error {
	c.args.name = args[0]
	mainDir, err := mainWorktreeDir()
	if err != nil {
		return err
	}
	upstream := c.flags.upstream
	if upstream == `` {
		upstream, err = resolveOriginHEAD()
		if err != nil {
			return err
		}
	}
	branch := c.args.name
	path := siblingPath(mainDir, branch)
	if _, err := os.Stat(path); err == nil {
		return errors.New(`directory already exists: ` + path)
	}
	if err := createWorktree(path, branch, upstream); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created worktree %s\n", path)
	return nil
}

func resolveOriginHEAD() (string, error) {
	ref, err := runGit(`symbolic-ref`, `-q`, `refs/remotes/origin/HEAD`)
	if err != nil {
		return ``, errors.New(`failed to resolve origin/HEAD: ensure origin remote exists`)
	}
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, `refs/remotes/`)
	return ref, nil
}

func createWorktree(path, branch, upstream string) error {
	cmd := exec.Command(`git`, `worktree`, `add`, `-b`, branch, path, upstream)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Join(errors.New(`git worktree add failed`), err)
	}
	if err := setUpstream(branch, upstream); err != nil {
		return err
	}
	return nil
}

func setUpstream(branch, upstream string) error {
	cmd := exec.Command(`git`, `branch`, `--set-upstream-to`, upstream, branch)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Join(errors.New(`failed to set upstream`), err)
	}
	return nil
}