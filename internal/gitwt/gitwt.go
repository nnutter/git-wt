package gitwt

import (
"errors"
"fmt"
"os"
"os/exec"
"path"
"strings"

"charm.land/lipgloss/v2"
"github.com/spf13/cobra"
)

var rootCmd *cobra.Command

func Root() *cobra.Command {
	rootCmd = &cobra.Command{
		Use:   `git-wt`,
		Short: `Manage Git worktrees`,
		Long:  `A tool for managing Git worktrees using git worktree under the hood.`,
	}
	rootCmd.AddCommand(NewCreateCmd())
	rootCmd.AddCommand(NewListCmd())
	rootCmd.AddCommand(NewPruneCmd())
	rootCmd.AddCommand(NewRemoveCmd())
	return rootCmd
}

func mainWorktreeDir() (string, error) {
	result, err := runGit(`rev-parse`, `--show-toplevel`)
	if err != nil {
		return ``, errors.Join(errors.New(`failed to get main worktree directory`), err)
	}
	return strings.TrimSpace(result), nil
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command(`git`, args...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return ``, err
	}
	return strings.TrimSpace(string(output)), nil
}

func normalizeName(name string) string {
	return strings.ReplaceAll(name, `/`, `.`)
}

func siblingPath(mainDir, name string) string {
	return path.Join(path.Dir(mainDir), path.Base(mainDir)+`.`+normalizeName(name))
}

var errorStyle = lipgloss.Style{}
var warningStyle = lipgloss.Style{}
var tblStyle_ tableStyle

func init() {
	errorStyle = errorStyle.Foreground(lipgloss.Color(`196`))
	warningStyle = warningStyle.Foreground(lipgloss.Color(`226`))
	tblStyle_ = newTableStyle()
}

func errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, `%s %s\n`, errorStyle.Render(`error`), fmt.Sprintf(format, args...))
}

func warningf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, `%s %s\n`, warningStyle.Render(`warning`), fmt.Sprintf(format, args...))
}

type tableStyle struct {
	lipgloss.Style
}

func newTableStyle() tableStyle {
	return tableStyle{
		lipgloss.NewStyle(),
	}
}

type table struct {
	headers []string
	rows    [][]string
	style   tableStyle
}

func newTable(headers ...string) *table {
	return &table{
		headers: headers,
		rows:    make([][]string, 0),
		style:   tblStyle_,
	}
}

func (t *table) addRow(row ...string) {
	t.rows = append(t.rows, row)
}

func (t *table) render() string {
	if len(t.rows) == 0 {
		return ``
	}
	maxLens := make([]int, len(t.headers))
	for i, h := range t.headers {
		maxLens[i] = len(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if l := len(cell); l > maxLens[i] {
				maxLens[i] = l
			}
		}
	}
	var b strings.Builder
	for i, h := range t.headers {
		b.WriteString(h)
		b.WriteString(strings.Repeat(` `, maxLens[i]-len(h)))
		if i < len(t.headers)-1 {
			b.WriteString(`  `)
		}
	}
	b.WriteString("\n")
	for _, row := range t.rows {
		for i, cell := range row {
			b.WriteString(cell)
			b.WriteString(strings.Repeat(` `, maxLens[i]-len(cell)))
			if i < len(row)-1 {
				b.WriteString(`  `)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}