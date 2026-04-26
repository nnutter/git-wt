package gitwt

import (
	"path/filepath"
	"strings"
)

func NormalizeName(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

func WorktreePath(mainWorktreePath, name string) string {
	base := filepath.Base(mainWorktreePath)
	dir := filepath.Dir(mainWorktreePath)
	suffix := NormalizeName(name)
	return filepath.Join(dir, base+"."+suffix)
}

func IsGitWtWorktree(mainWorktreePath, worktreePath string) (bool, string) {
	mainBase := filepath.Base(mainWorktreePath)
	mainDir := filepath.Dir(mainWorktreePath)
	wtDir := filepath.Dir(worktreePath)
	wtBase := filepath.Base(worktreePath)

	if wtDir != mainDir {
		return false, ""
	}

	prefix := mainBase + "."
	if !strings.HasPrefix(wtBase, prefix) {
		return false, ""
	}

	suffix := strings.TrimPrefix(wtBase, prefix)
	branchName := strings.ReplaceAll(suffix, ".", "/")
	return true, branchName
}

type WorktreeInfo struct {
	Path       string
	HEAD       string
	Branch     string
	IsBare     bool
	Detached   bool
}

func ParseWorktreeList(output string) []WorktreeInfo {
	var entries []WorktreeInfo
	var current *WorktreeInfo

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			if current != nil {
				entries = append(entries, *current)
				current = nil
			}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 1 {
			continue
		}

		switch parts[0] {
		case "worktree":
			if current != nil {
				entries = append(entries, *current)
			}
			current = &WorktreeInfo{}
			if len(parts) > 1 {
				current.Path = parts[1]
			}
		case "HEAD":
			if current != nil && len(parts) > 1 {
				current.HEAD = parts[1]
			}
		case "branch":
			if current != nil && len(parts) > 1 {
				branchRef := parts[1]
				current.Branch = strings.TrimPrefix(branchRef, "refs/heads/")
			}
		case "bare":
			if current != nil {
				current.IsBare = true
			}
		case "detached":
			if current != nil {
				current.Detached = true
			}
		}
	}

	if current != nil {
		entries = append(entries, *current)
	}

	return entries
}
