package gitwt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-git/go-git/v5/plumbing"
)

type managedWorktree struct {
	Name            string
	NormalizedName  string
	Path            string
	DisplayPath     string
	BranchReference plumbing.ReferenceName
	UpstreamRef     plumbing.ReferenceName
	Status          string
	Main            bool
	Clean           bool
	Merged          bool
}

func enrichManagedWorktree(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
	upstreamRef, err := repository.upstreamReference(worktree.Name)
	if err != nil {
		return managedWorktree{}, err
	}

	wtRepository, err := PlainOpenWithOptions(worktree.Path)
	if err != nil {
		return managedWorktree{}, err
	}

	clean, err := wtRepository.isClean()
	if err != nil {
		return managedWorktree{}, err
	}

	status, err := wtRepository.status()
	if err != nil {
		return managedWorktree{}, err
	}

	merged, err := wtRepository.branchMergedToUpstream(worktree.BranchReference, upstreamRef)
	if err != nil {
		return managedWorktree{}, err
	}

	worktree.UpstreamRef = upstreamRef
	worktree.Status = status
	worktree.Clean = clean
	worktree.Merged = merged

	return worktree, nil
}

func managedWorktreesFromRepository(repository *Repository) ([]managedWorktree, string, error) {
	porcelainWorktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return nil, "", err
	}

	mainPath, err := repository.mainWorktreePath()
	if err != nil {
		return nil, "", err
	}

	currentDirectory, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("get current directory: %w", err)
	}

	managedWorktrees := make([]managedWorktree, 0)
	for _, porcelainWorktree := range porcelainWorktrees {
		branchName := porcelainWorktree.branchName()
		if branchName == "" {
			continue
		}

		expectedPath := managedWorktreePath(mainPath, branchName)
		if filepath.Clean(expectedPath) != filepath.Clean(porcelainWorktree.Path) {
			continue
		}

		managedWorktrees = append(managedWorktrees, managedWorktree{
			Name:            branchName,
			NormalizedName:  normalizeWorktreeName(branchName),
			Path:            porcelainWorktree.Path,
			DisplayPath:     currentRelativePath(currentDirectory, porcelainWorktree.Path),
			BranchReference: plumbing.ReferenceName(porcelainWorktree.BranchRef),
			Main:            filepath.Clean(porcelainWorktree.Path) == filepath.Clean(mainPath),
		})
	}

	sort.Slice(managedWorktrees, func(leftIndex int, rightIndex int) bool {
		return managedWorktrees[leftIndex].Name < managedWorktrees[rightIndex].Name
	})

	return managedWorktrees, mainPath, nil
}

func managedWorktreeByName(worktrees []managedWorktree, name string) (managedWorktree, error) {
	for _, worktree := range worktrees {
		if worktree.Name == name {
			return worktree, nil
		}
	}

	return managedWorktree{}, fmt.Errorf("unknown worktree %q", name)
}
