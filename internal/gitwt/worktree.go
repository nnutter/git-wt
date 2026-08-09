package gitwt

import (
	"cmp"
	"fmt"
	"os"
	"slices"
)

type managedWorktree struct {
	Repo            string
	Name            string
	Path            string
	DisplayPath     string
	CommitHash      string
	BranchReference referenceName
	UpstreamRef     referenceName
	Status          string
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
	wtRepository, err := openRepository(worktree.Path)
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

	worktree.Status = status
	worktree.Clean = clean

	upstreamRef, err := repository.upstreamReference(worktree.Name)
	if err != nil {
		return managedWorktree{}, err
	}

	merged, err := repository.branchMergedToUpstream(worktree.BranchReference, upstreamRef)
	if err != nil {
		return managedWorktree{}, err
	}

	worktree.UpstreamRef = upstreamRef
	worktree.Merged = merged

	return worktree, nil
}

func managedWorktreesFromRepository(repository *Repository, repoName string) ([]managedWorktree, error) {
	porcelainWorktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return nil, err
	}

	currentDirectory, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current directory: %w", err)
	}

	managedWorktrees := make([]managedWorktree, 0)
	for _, porcelainWorktree := range porcelainWorktrees {
		branchName := porcelainWorktree.branchName()
		if branchName == "" {
			continue
		}

		expectedPath := managedWorktreePath(repoName, branchName)
		same, err := samePath(expectedPath, porcelainWorktree.Path)
		if err != nil {
			return nil, err
		}
		if !same {
			continue
		}

		managedWorktrees = append(managedWorktrees, managedWorktree{
			Repo:            repoName,
			Name:            branchName,
			Path:            porcelainWorktree.Path,
			DisplayPath:     currentRelativePath(currentDirectory, porcelainWorktree.Path),
			CommitHash:      porcelainWorktree.CommitHash,
			BranchReference: referenceName(porcelainWorktree.BranchRef),
		})
	}

	slices.SortFunc(managedWorktrees, compareManagedWorktrees)

	return managedWorktrees, nil
}

func compareManagedWorktrees(left, right managedWorktree) int {
	if repoOrder := cmp.Compare(left.Repo, right.Repo); repoOrder != 0 {
		return repoOrder
	}
	return cmp.Compare(left.Name, right.Name)
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
	for _, worktree := range worktrees {
		same, err := samePath(worktree.Path, path)
		if err != nil {
			return managedWorktree{}, err
		}
		if same {
			return worktree, nil
		}
	}

	return managedWorktree{}, fmt.Errorf("not inside a managed worktree")
}
