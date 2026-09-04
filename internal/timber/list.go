package timber

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

type listCommandOptions struct {
	repoSelection
}

func NewListCommand(runtime Runtime) *cobra.Command {
	options := &listCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:               "list [@repo]",
		Aliases:           []string{"ls"},
		Short:             "List managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeRepoQualifiers,
	}
	return command
}

func (x *listCommandOptions) Execute(command *cobra.Command, args []string) error {
	if err := x.applyRepoArg(args); err != nil {
		return err
	}

	worktrees, err := x.collectWorktrees()
	if err != nil {
		return err
	}

	statusFormatter := newListStatusFormatter(worktrees)
	tableView := newOutputTable("Name", "Repo", "Status", "Commit", "Dirty").BorderRow(true)
	tableView.Rows(groupListTableRows(worktrees, statusFormatter)...)

	_, err = fmt.Fprintln(command.OutOrStdout(), dottedListRowRules(tableView.String()))
	return err
}

func dottedListRowRules(tableOutput string) string {
	lines := strings.Split(tableOutput, "\n")
	headerRuleFound := false
	for index, line := range lines {
		if !strings.HasPrefix(line, "├") {
			continue
		}
		if !headerRuleFound {
			headerRuleFound = true
			continue
		}
		lines[index] = strings.ReplaceAll(line, "─", "┈")
	}
	return strings.Join(lines, "\n")
}

func groupListTableRows(worktrees []managedWorktree, statusFormatter listStatusFormatter) [][]string {
	rows := make([][]string, 0, (len(worktrees)+1)/2)
	for index, worktree := range worktrees {
		row := []string{
			worktree.Name,
			worktree.Repo,
			statusFormatter.format(worktree.ListStatus),
			worktree.shortCommitHash(),
			formatDirtyStatus(worktree.Clean),
		}
		if index%2 == 0 {
			rows = append(rows, row)
			continue
		}
		for column := range row {
			rows[len(rows)-1][column] += "\n" + row[column]
		}
	}
	return rows
}

type listStatusFormatter struct {
	aheadCountWidth  int
	behindCountWidth int
}

func newListStatusFormatter(worktrees []managedWorktree) listStatusFormatter {
	var formatter listStatusFormatter
	for _, worktree := range worktrees {
		status := worktree.ListStatus
		if status.Ahead > 0 {
			formatter.aheadCountWidth = max(formatter.aheadCountWidth, len(strconv.Itoa(status.Ahead)))
		}
		if status.Behind > 0 {
			formatter.behindCountWidth = max(formatter.behindCountWidth, len(strconv.Itoa(status.Behind)))
		}
	}
	return formatter
}

func (x listStatusFormatter) format(status listStatus) string {
	parts := make([]string, 0, 3)
	if x.aheadCountWidth > 0 {
		indicator := formatListStatusIndicator("↑", status.Ahead, x.aheadCountWidth)
		if status.Ahead > 0 {
			indicator = aheadStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	if x.behindCountWidth > 0 {
		indicator := formatListStatusIndicator("↓", status.Behind, x.behindCountWidth)
		if status.Behind > 0 {
			indicator = behindStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	if status.Upstream != "" {
		parts = append(parts, "["+status.Upstream+"]")
	}
	return strings.TrimRight(strings.Join(parts, " "), " ")
}

func formatListStatusIndicator(arrow string, count int, countWidth int) string {
	if count == 0 {
		return strings.Repeat(" ", lipgloss.Width(arrow)+countWidth)
	}
	return arrow + fmt.Sprintf("%*d", countWidth, count)
}

func formatDirtyStatus(clean bool) string {
	dirty := strconv.FormatBool(!clean)
	if !clean {
		return warningStyle.Render(dirty)
	}
	return dirty
}

func (x *listCommandOptions) collectWorktrees() ([]managedWorktree, error) {
	repos, err := x.reposToConsider()
	if err != nil {
		return nil, err
	}
	return x.runtime.collectListedWorktrees(repos)
}

func (x *listCommandOptions) applyRepoArg(args []string) error {
	if len(args) == 0 {
		return nil
	}
	repo, err := x.runtime.parseRepoOnlyArg(args[0])
	if err != nil {
		return err
	}
	x.RepoName = repo
	return nil
}
