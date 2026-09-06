package timber

import (
	"strings"
)

type porcelainWorktree struct {
	Path       string
	BranchRef  string
	CommitHash string
	Detached   bool
	Prunable   string
}

func (x porcelainWorktree) branchName() string {
	branchName, found := strings.CutPrefix(x.BranchRef, branchRefPrefix)
	if !found {
		return ""
	}
	return branchName
}
