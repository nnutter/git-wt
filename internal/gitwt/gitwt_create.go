package gitwt

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type createOptions struct {
	upstream string
}

func NewCreateCmd() *cobra.Command {
	opts := &createOptions{}

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new Git worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.Execute(args)
		},
	}

	cmd.Flags().StringVarP(&opts.upstream, "upstream", "u", "", "Upstream branch (defaults to origin/HEAD)")

	return cmd
}

func (o *createOptions) Execute(args []string) error {
	name := args[0]

	upstream := o.upstream
	if upstream == "" {
		defUpstream, err := getDefaultUpstream()
		if err != nil {
			return fmt.Errorf("failed to determine default upstream: %w", err)
		}
		upstream = defUpstream
	}

	mainPath, err := getMainWorktreePath()
	if err != nil {
		return fmt.Errorf("failed to get main worktree path: %w", err)
	}

	path := getNormalizedPath(mainPath, name)

	// Check if directory exists
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("directory already exists: %s", path)
	}

	// Check if branch exists
	// git rev-parse --verify refs/heads/<name>
	_, err = execGit("rev-parse", "--verify", "refs/heads/"+name)
	if err == nil {
		return fmt.Errorf("branch already exists: %s", name)
	}

	// Run git worktree add
	out, err := execGit("worktree", "add", "-b", name, path, upstream)
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w", err)
	}
	fmt.Fprintln(os.Stdout, out)

	// Ensure upstream tracking is set
	if strings.Contains(upstream, "/") { // simple heuristic for remote branch
		_, err = execGit("branch", "--set-upstream-to="+upstream, name)
		if err != nil {
			return fmt.Errorf("failed to set upstream: %w", err)
		}
	}

	return nil
}
