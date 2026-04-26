package gitwt

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type PruneCmd struct {
	Prompt bool
}

type pruneCandidate struct {
	Name     string
	Path     string
	Merged   bool
	Clean    bool
}

func (p *PruneCmd) Execute(_ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	output, err := gitWorktreeList(dir)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	entries := ParseWorktreeList(output)

	var mainPath string
	for _, e := range entries {
		if !e.IsBare && e.Branch != "" {
			mainPath = e.Path
			break
		}
	}
	if mainPath == "" {
		return fmt.Errorf("could not determine main worktree")
	}

	var candidates []pruneCandidate
	for _, e := range entries {
		if e.IsBare || e.Branch == "" {
			continue
		}
		ok, _ := IsGitWtWorktree(mainPath, e.Path)
		if !ok {
			continue
		}

		upstream, err := gitBranchUpstream(dir, e.Branch)
		if err != nil {
			upstream = ""
		}

		merged := false
		if upstream != "" {
			m, _ := gitMergeBaseIsAncestor(dir, e.Branch, gitResolveLocalBranch(dir, upstream))
			merged = m
		}

		clean := false
		if c, err := gitWorkdirClean(e.Path); err == nil {
			clean = c
		}

		candidates = append(candidates, pruneCandidate{
			Name:   e.Branch,
			Path:   e.Path,
			Merged: merged,
			Clean:  clean,
		})
	}

	var toRemove []pruneCandidate
	var needsPrompt []pruneCandidate

	for _, c := range candidates {
		if c.Merged && c.Clean {
			toRemove = append(toRemove, c)
		} else {
			needsPrompt = append(needsPrompt, c)
		}
	}

	for _, c := range toRemove {
		if err := removeWorktree(dir, c.Name, c.Path, true); err != nil {
			printWarn("failed to remove worktree %q: %v", c.Name, err)
		} else {
			printInfo("Removed worktree %q", c.Name)
		}
	}

	if p.Prompt && len(needsPrompt) > 0 {
		options := make([]huh.Option[bool], len(needsPrompt))
		selected := make([]bool, len(needsPrompt))
		for i, c := range needsPrompt {
			status := ""
			if !c.Merged {
				status += " [unmerged]"
			}
			if !c.Clean {
				status += " [unclean]"
			}
			options[i] = huh.NewOption(fmt.Sprintf("%s%s", c.Name, status), true)
			selected[i] = false
		}

		var results []bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[bool]().
					Title("Select worktrees to remove").
					Options(options...).
					Value(&results),
			),
		)
		if err := form.Run(); err != nil {
			return fmt.Errorf("prompt failed: %w", err)
		}

		for i, val := range results {
			if val {
				c := needsPrompt[i]
				if err := removeWorktree(dir, c.Name, c.Path, true); err != nil {
					printWarn("failed to remove worktree %q: %v", c.Name, err)
				} else {
					printInfo("Removed worktree %q", c.Name)
				}
			}
		}
	}

	return nil
}

func removeWorktree(dir, name, path string, force bool) error {
	if err := gitWorktreeRemove(dir, path, force); err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		printWarn("directory %q still exists after remove", path)
	}

	exists, err := gitBranchExists(dir, name)
	if err == nil && exists {
		if err := gitBranchDelete(dir, name, force); err != nil {
			printWarn("failed to delete branch %q: %v", name, err)
		}
	}

	return nil
}

func NewPrune() *cobra.Command {
	p := &PruneCmd{}
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune merged worktrees",
		Long:  "Remove worktrees whose branches have been merged and have clean working directories.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return p.Execute(args)
		},
	}
	cmd.Flags().BoolVarP(&p.Prompt, "prompt", "p", false, "prompt for unmerged/unclean worktrees")
	return cmd
}
