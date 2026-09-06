package timber

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const repoRenamePathFileEnvVarName = "TIMBER_RENAME_PATH_FILE"

type (
	renamePathFunc      func(string, string) error
	repairWorktreesFunc func(string, []string) error
)

type repoRenameCommandOptions struct {
	runtime         Runtime
	renamePath      renamePathFunc
	repairWorktrees repairWorktreesFunc
}

func NewRepoRenameCommand(runtime Runtime) *cobra.Command {
	options := &repoRenameCommandOptions{
		runtime:    runtime,
		renamePath: os.Rename,
		repairWorktrees: func(barePath string, worktreePaths []string) error {
			return repairWorktrees(runtime, barePath, worktreePaths)
		},
	}
	return &cobra.Command{
		Use:               "rename <old-name> <new-name>",
		Aliases:           []string{"mv"},
		Short:             "Rename a registered repository and its managed worktrees",
		Args:              cobra.ExactArgs(2),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeRepoRenameArguments,
	}
}

func (x *repoRenameCommandOptions) Execute(command *cobra.Command, args []string) error {
	plan, err := x.runtime.buildRepositoryRenamePlan(args[0], args[1])
	if err != nil {
		return err
	}

	if err := x.runtime.reportRenamedCurrentPath(plan.currentTargetDir); err != nil {
		return err
	}
	if err := plan.apply(x.renamePath, x.repairWorktrees); err != nil {
		return err
	}

	message := fmt.Sprintf("renamed repository %s to %s", plan.sourceRepo.Name, plan.destinationRepo.Name)
	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message))
	return err
}
