package gitwt

import (
	`bufio`
	`fmt`
	`os/exec`
	`path/filepath`
	`strings`

	`github.com/spf13/cobra`
)

type ListCmd struct{}

func NewListCmd() *cobra.Command {
	cmd := &ListCmd{}
	c := &cobra.Command{
		Use:   `list`,
		Short: `List Git worktrees managed by git-wt`,
		Args:  cobra.NoArgs,
		RunE:  cmd.Execute,
	}
	return c
}

type worktreeInfo struct {
	name string
	wtPath string
}

func (c *ListCmd) Execute(cmd *cobra.Command, args []string) error {
	mainDir, err := mainWorktreeDir()
	if err != nil {
		return err
	}
	mainDir = filepath.Clean(mainDir)
	worktrees, err := listWorktrees()
	if err != nil {
		return err
	}
	tbl := newTable(`NAME`, `PATH`)
	for _, wt := range worktrees {
		if wt.wtPath == mainDir {
			continue
		}
		name := nameFromPath(mainDir, wt.wtPath)
		if name == `` {
			continue
		}
		relPath, err := filepath.Rel(mainDir, wt.wtPath)
		if err != nil {
			continue
		}
		tbl.addRow(name, relPath)
	}
	fmt.Print(tbl.render())
	return nil
}

func listWorktrees() ([]worktreeInfo, error) {
	cmd := exec.Command(`git`, `worktree`, `list`, `--porcelain`)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var worktrees []worktreeInfo
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var current worktreeInfo
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, `worktree `) {
			if current.wtPath != `` {
				worktrees = append(worktrees, current)
			}
			current = worktreeInfo{wtPath: strings.TrimPrefix(line, `worktree `)}
		}
	}
	if current.wtPath != `` {
		worktrees = append(worktrees, current)
	}
	return worktrees, nil
}

func nameFromPath(mainDir, worktreePath string) string {
	mainDirBase := filepath.Base(mainDir)
	worktreeBase := filepath.Base(worktreePath)
	if !strings.HasPrefix(worktreeBase, mainDirBase+ `.`) {
		return ``
	}
	name := strings.TrimPrefix(worktreeBase, mainDirBase+ `.`)
	name = strings.ReplaceAll(name, `.`, `/`)
	return name
}