package timber

import (
	"cmp"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	remoteName      = "origin"
	branchRefPrefix = "refs/heads/"
	remoteRefPrefix = "refs/remotes/"
)

type referenceName string

func openRepository(runtime Runtime, path string) (*Repository, error) {
	workTreeResult, err := gitOutput(runtime, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}

	gitDirResult, err := gitOutput(runtime, path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve Git directory: %w", err)
	}

	return &Repository{GitDir: gitDirResult.stdout, WorkTree: workTreeResult.stdout, Runtime: runtime}, nil
}

func openBareRepository(runtime Runtime, barePath string) (*Repository, error) {
	gitDirResult, err := gitOutput(runtime, barePath, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("open bare repository: %w", err)
	}

	bareResult, err := gitOutput(runtime, barePath, "rev-parse", "--is-bare-repository")
	if err != nil {
		return nil, fmt.Errorf("inspect bare repository: %w", err)
	}
	if bareResult.stdout != "true" {
		return nil, fmt.Errorf("repository at %q is not bare", barePath)
	}

	return &Repository{GitDir: gitDirResult.stdout, Runtime: runtime}, nil
}

type Repository struct {
	GitDir   string
	WorkTree string
	Runtime  Runtime
}

func (x *Repository) branchExists(branchName string) (bool, error) {
	return x.branchStillExists(branchReference(branchName))
}

func (x *Repository) branchMergedToUpstream(branchRef referenceName, upstreamRef referenceName) (bool, error) {
	upstreamExists, err := x.branchStillExists(upstreamRef)
	if err != nil {
		return false, err
	}
	if !upstreamExists {
		return false, nil
	}

	_, err = x.git("merge-base", "--is-ancestor", string(branchRef), string(upstreamRef))
	if err == nil {
		return true, nil
	}

	if exitError, ok := errors.AsType[*exec.ExitError](err); ok && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func (x *Repository) branchStillExists(branchRef referenceName) (bool, error) {
	_, err := x.git("show-ref", "--verify", "--quiet", string(branchRef))
	if err == nil {
		return true, nil
	}

	if exitError, ok := errors.AsType[*exec.ExitError](err); ok && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func (x Repository) git(args ...string) (gitCommandResult, error) {
	allArgs := []string{"--git-dir", x.GitDir}
	if x.WorkTree != "" {
		allArgs = append(allArgs, "--work-tree", x.WorkTree)
	}
	allArgs = append(allArgs, args...)
	directory := cmp.Or(x.WorkTree, x.GitDir)
	return gitOutput(x.Runtime, directory, allArgs...)
}

func (x *Repository) commonGitDir() (string, error) {
	result, err := x.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("resolve common git dir: %w", err)
	}
	return filepath.Clean(result.stdout), nil
}

func (x Repository) isClean() (bool, error) {
	result, err := x.git("status", "--porcelain")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(result.stdout) == "", nil
}

func (x Repository) status() (string, error) {
	result, err := x.git("status", "-sb")
	if err != nil {
		return "", err
	}

	statusLine, _, _ := strings.Cut(result.stdout, "\n")
	return strings.TrimPrefix(statusLine, "## "), nil
}

type listStatus struct {
	Upstream string
	Ahead    int
	Behind   int
}

func parsePorcelainStatus(output string) (listStatus, bool, error) {
	var status listStatus
	clean := true

	for line := range strings.SplitSeq(output, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.upstream "):
			status.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(line)
			if len(fields) != 4 {
				return listStatus{}, false, fmt.Errorf("unexpected branch divergence line %q", line)
			}

			var err error
			status.Ahead, err = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
			if err != nil {
				return listStatus{}, false, fmt.Errorf("parse ahead count: %w", err)
			}
			status.Behind, err = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
			if err != nil {
				return listStatus{}, false, fmt.Errorf("parse behind count: %w", err)
			}
		case line != "" && !strings.HasPrefix(line, "# "):
			clean = false
		}
	}

	return status, clean, nil
}

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

func (x *Repository) listPorcelainWorktrees() ([]porcelainWorktree, error) {
	result, err := x.git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	worktrees := make([]porcelainWorktree, 0)
	for block := range strings.SplitSeq(strings.TrimSpace(result.stdout), "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}

		var worktree porcelainWorktree
		for line := range strings.SplitSeq(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				worktree.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "branch "):
				worktree.BranchRef = strings.TrimPrefix(line, "branch ")
			case strings.HasPrefix(line, "HEAD "):
				worktree.CommitHash = strings.TrimPrefix(line, "HEAD ")
			case line == "detached":
				worktree.Detached = true
			case strings.HasPrefix(line, "prunable "):
				worktree.Prunable = strings.TrimPrefix(line, "prunable ")
			}
		}

		if worktree.Path != "" {
			worktrees = append(worktrees, worktree)
		}
	}

	return worktrees, nil
}

func (x *Repository) mainWorktreePath() (string, error) {
	worktrees, err := x.listPorcelainWorktrees()
	if err != nil {
		return "", err
	}

	if len(worktrees) == 0 {
		return "", errors.New("no worktrees found")
	}

	return worktrees[0].Path, nil
}

func (x *Repository) remoteHeadBranch() (string, error) {
	if branch, err := x.resolvedRemoteHeadBranch(); err == nil {
		return branch, nil
	}

	// Older bare registrations may lack remote.origin.fetch; repair once and retry.
	if repairErr := x.ensureOriginRemoteTracking(); repairErr == nil {
		if branch, err := x.resolvedRemoteHeadBranch(); err == nil {
			return branch, nil
		}
	}

	// Bare clones without remote-tracking refs can still start from a local branch.
	localFallback, localErr := x.firstExistingLocalBranch("master", "main")
	if localErr != nil {
		return "", localErr
	}
	if localFallback != "" {
		return localFallback, nil
	}

	return "", fmt.Errorf("resolve origin/HEAD: no origin/HEAD, origin/master, origin/main, or local main/master")
}

func (x *Repository) resolvedRemoteHeadBranch() (string, error) {
	result, err := x.git("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		return result.stdout, nil
	}

	fallback, fallbackErr := x.firstExistingRemoteBranch("master", "main")
	if fallbackErr != nil {
		return "", fallbackErr
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("remote head branch not found")
}

func (x *Repository) ensureOriginRemoteTracking() error {
	url, configured, err := x.gitConfigValue("remote." + remoteName + ".url")
	if err != nil {
		return err
	}
	if !configured || url == "" {
		return fmt.Errorf("remote %q is not configured", remoteName)
	}
	return configureBareOriginTracking(x.Runtime, x.GitDir)
}

func (x *Repository) firstExistingRemoteBranch(branchNames ...string) (string, error) {
	for _, branchName := range branchNames {
		remoteBranch := remoteName + "/" + branchName
		exists, err := x.branchStillExists(referenceName(remoteRefPrefix + remoteBranch))
		if err != nil {
			return "", err
		}
		if exists {
			return remoteBranch, nil
		}
	}

	return "", nil
}

func (x *Repository) firstExistingLocalBranch(branchNames ...string) (string, error) {
	for _, branchName := range branchNames {
		exists, err := x.branchExists(branchName)
		if err != nil {
			return "", err
		}
		if exists {
			return branchName, nil
		}
	}

	return "", nil
}

func (x *Repository) upstreamReference(branchName string) (referenceName, error) {
	branchRef := branchReference(branchName)
	result, err := x.git("for-each-ref", "--format=%(refname)%00%(upstream)", string(branchRef))
	if err != nil {
		return "", fmt.Errorf("read branch config for %q: %w", branchName, err)
	}

	for line := range strings.SplitSeq(result.stdout, "\n") {
		refName, upstreamRef, found := strings.Cut(line, "\x00")
		if !found || refName != string(branchRef) {
			continue
		}
		if upstreamRef == "" {
			return x.configuredUpstreamReference(branchName)
		}

		return referenceName(upstreamRef), nil
	}

	return "", fmt.Errorf("branch %q does not exist", branchName)
}

func (x *Repository) configuredUpstreamReference(branchName string) (referenceName, error) {
	mergeRef, mergeConfigured, err := x.gitConfigValue("branch." + branchName + ".merge")
	if err != nil {
		return "", err
	}
	if !mergeConfigured {
		return "", fmt.Errorf("branch %q has no upstream branch", branchName)
	}

	remote, remoteConfigured, err := x.gitConfigValue("branch." + branchName + ".remote")
	if err != nil {
		return "", err
	}
	if !remoteConfigured || remote == "." {
		return referenceName(mergeRef), nil
	}

	return x.mappedUpstreamReference(branchName, referenceName(mergeRef), remote)
}

func (x *Repository) mappedUpstreamReference(branchName string, mergeRef referenceName, remote string) (referenceName, error) {
	result, err := x.git("rev-parse", "--symbolic-full-name", branchName+"@{upstream}")
	if err != nil {
		return "", fmt.Errorf(
			"branch %q tracks %q at %q, which does not map to a known fetch refspec: %w",
			branchName,
			mergeRef,
			remote,
			err,
		)
	}

	return referenceName(result.stdout), nil
}

func (x *Repository) gitConfigValue(key string) (string, bool, error) {
	result, err := x.git("config", "--get", key)
	if err == nil {
		return result.stdout, true, nil
	}

	if exitError, ok := errors.AsType[*exec.ExitError](err); ok && exitError.ExitCode() == 1 {
		return "", false, nil
	}

	return "", false, fmt.Errorf("read Git config %q: %w", key, err)
}

func branchReference(branchName string) referenceName {
	return referenceName(branchRefPrefix + branchName)
}

func shortReference(ref referenceName) string {
	refName := string(ref)
	if shortName, found := strings.CutPrefix(refName, branchRefPrefix); found {
		return shortName
	}
	if shortName, found := strings.CutPrefix(refName, remoteRefPrefix); found {
		return shortName
	}
	return refName
}
