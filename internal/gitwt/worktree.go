package gitwt

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type managedWorktree struct {
	Name            string
	Path            string
	DisplayPath     string
	CommitHash      string
	BranchReference referenceName
	UpstreamRef     referenceName
	Status          string
	Main            bool
	Clean           bool
	Merged          bool
}

func (x managedWorktree) shortCommitHash() string {
	if len(x.CommitHash) <= 7 {
		return x.CommitHash
	}

	return x.CommitHash[:7]
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

	merged, err := repository.branchMergedToUpstream(worktree.BranchReference, upstreamRef)
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
			Path:            porcelainWorktree.Path,
			DisplayPath:     currentRelativePath(currentDirectory, porcelainWorktree.Path),
			CommitHash:      porcelainWorktree.CommitHash,
			BranchReference: referenceName(porcelainWorktree.BranchRef),
			Main:            filepath.Clean(porcelainWorktree.Path) == filepath.Clean(mainPath),
		})
	}

	slices.SortFunc(managedWorktrees, func(left, right managedWorktree) int {
		return cmp.Compare(left.Name, right.Name)
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

func managedWorktreeForPath(worktrees []managedWorktree, path string) (managedWorktree, error) {
	cleanedPath := filepath.Clean(path)
	for _, worktree := range worktrees {
		if filepath.Clean(worktree.Path) == cleanedPath {
			return worktree, nil
		}
	}

	return managedWorktree{}, fmt.Errorf("not inside a managed worktree")
}
