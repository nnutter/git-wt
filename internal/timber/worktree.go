package timber

import (
	"cmp"
	"fmt"
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
	ListStatus      listStatus
	Clean           bool
	Merged          bool
}

func (x managedWorktree) shortCommitHash() string {
	if len(x.CommitHash) <= 7 {
		return x.CommitHash
	}

	return x.CommitHash[:7]
}

type worktreeEnricher func(*Repository, managedWorktree) (managedWorktree, error)

func compareManagedWorktrees(left, right managedWorktree) int {
	if repoOrder := cmp.Compare(left.Repo, right.Repo); repoOrder != 0 {
		return repoOrder
	}
	return cmp.Compare(left.Name, right.Name)
}

func managedWorktreeByName(worktrees []managedWorktree, name string) (managedWorktree, error) {
	index := slices.IndexFunc(worktrees, func(worktree managedWorktree) bool {
		return worktree.Name == name
	})
	if index >= 0 {
		return worktrees[index], nil
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
