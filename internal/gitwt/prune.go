package gitwt

import (
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

type pruneCommand struct {
	prompt bool
}

func NewPruneCommand() *cobra.Command {
	cmd := &pruneCommand{}
	c := &cobra.Command{
		Use:   "prune",
		Short: "Remove merged, clean worktrees",
		RunE:  cmd.Execute,
	}
	c.Flags().BoolVarP(&cmd.prompt, "prompt", "p", false, "Prompt for each worktree, allowing force removal of unmerged/unclean worktrees")
	return c
}

func (p *pruneCommand) Execute(_ *cobra.Command, _ []string) error {
	mainPath, err := mainWorktreePath()
	if err != nil {
		return err
	}

	candidates, err := listGitWtWorktrees()
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		return nil
	}

	var toRemove []struct {
		name string
		path string
	}
	var promptOptions []huh.Option[string]

	for _, c := range candidates {
		isClean, err := isWorktreeClean(c.path)
		if err != nil {
			continue
		}

		upstream, err := getBranchUpstream(c.name)
		if err != nil {
			continue
		}

		isMerged, err := isBranchMerged(c.name, upstream)
		if err != nil {
			continue
		}

		if isClean && isMerged {
			toRemove = append(toRemove, struct {
				name string
				path string
			}{name: c.name, path: c.path})
		} else if p.prompt {
			label := fmt.Sprintf("%s (unclean or unmerged)", c.name)
			promptOptions = append(promptOptions, huh.NewOption(label, c.name))
		}
	}

	if p.prompt && len(promptOptions) > 0 {
		selected, err := runPrunePrompt(promptOptions, toRemove)
		if err != nil {
			return err
		}
		for _, name := range selected {
			normalized := normalizeName(name)
			path := buildWorktreePath(mainPath, normalized)
			toRemove = append(toRemove, struct {
				name string
				path string
			}{name: name, path: path})
		}
	}

	for _, r := range toRemove {
		if err := removeWorktree(r.path); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove worktree %q: %v\n", r.path, err)
			continue
		}
		if err := warnIfDirectoryRemains(r.path); err != nil {
			continue
		}
		if err := deleteBranch(r.name, true); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete branch %q: %v\n", r.name, err)
		}
	}

	return nil
}

func runPrunePrompt(options []huh.Option[string], preselected []struct {
	name string
	path string
}) ([]string, error) {
	for _, p := range preselected {
		options = append(options, huh.NewOption(p.name+" (ready to remove)", p.name).Selected(true))
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select worktrees to remove").
				Options(options...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}
	return selected, nil
}
