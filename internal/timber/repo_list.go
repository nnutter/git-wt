package timber

import (
	"fmt"

	"github.com/spf13/cobra"
)

type repoListCommandOptions struct {
	runtime Runtime
	quiet   bool
}

func NewRepoListCommand(runtime Runtime) *cobra.Command {
	options := &repoListCommandOptions{runtime: runtime}
	command := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered repositories",
		Args:    cobra.NoArgs,
		RunE:    options.Execute,
	}
	command.Flags().BoolVarP(&options.quiet, "quiet", "q", false, "Print repository names only")

	return command
}

func (x *repoListCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := x.runtime.listRegisteredRepos()
	if err != nil {
		return err
	}
	if x.quiet {
		return writeRepoNames(command, repos)
	}

	tableView := newOutputTable("Name", "Path", "Origin")
	for _, repo := range repos {
		tableView.Row(repo.Name, x.runtime.displayHomePath(repo.BarePath), repo.originURL(x.runtime))
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}

func writeRepoNames(command *cobra.Command, repos []registeredRepo) error {
	for _, repo := range repos {
		if _, err := fmt.Fprintln(command.OutOrStdout(), repo.Name); err != nil {
			return err
		}
	}
	return nil
}
