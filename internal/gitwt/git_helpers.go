package gitwt

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

const remoteName = "origin"

type Repository struct {
	*git.Repository

	GitDir   string
	WorkTree string
}

type gitCommandResult struct {
	stdout string
	stderr string
}

type porcelainWorktree struct {
	Path       string
	BranchRef  string
	CommitHash string
	Detached   bool
	Prunable   string
}

type managedWorktree struct {
	Name            string
	NormalizedName  string
	Path            string
	DisplayPath     string
	BranchReference plumbing.ReferenceName
	UpstreamRef     plumbing.ReferenceName
	Main            bool
	Clean           bool
	Merged          bool
}

func (repository Repository) git(args ...string) (gitCommandResult, error) {
	allArgs := append([]string{"--git-dir", repository.GitDir, "--work-tree", repository.WorkTree}, args...)
	return gitOutput(repository.WorkTree, allArgs...)
}

func PlainOpenWithOptions(path string) (*Repository, error) {
	gitRepository, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}

	workTreeResult, err := gitOutput(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}

	gitDirResult, err := gitOutput(path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}

	return &Repository{
		Repository: gitRepository,
		GitDir:     gitDirResult.stdout,
		WorkTree:   workTreeResult.stdout,
	}, nil
}

func gitOutput(directory string, args ...string) (gitCommandResult, error) {
	command := exec.Command("git", args...)
	command.Dir = directory

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		result := gitCommandResult{
			stdout: strings.TrimSpace(stdout.String()),
			stderr: strings.TrimSpace(stderr.String()),
		}

		return result, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, result.stderr)
	}

	result := gitCommandResult{
		stdout: strings.TrimSpace(stdout.String()),
		stderr: strings.TrimSpace(stderr.String()),
	}

	return result, nil
}

func (repository *Repository) listPorcelainWorktrees() ([]porcelainWorktree, error) {
	result, err := repository.git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	blocks := strings.Split(strings.TrimSpace(result.stdout), "\n\n")
	worktrees := make([]porcelainWorktree, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		var worktree porcelainWorktree
		for _, line := range strings.Split(block, "\n") {
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

func (repository *Repository) mainWorktreePath() (string, error) {
	worktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return "", err
	}

	if len(worktrees) == 0 {
		return "", errors.New("no worktrees found")
	}

	return worktrees[0].Path, nil
}

func normalizeWorktreeName(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

func managedWorktreePath(mainPath string, branchName string) string {
	parentDirectory := filepath.Dir(mainPath)
	baseName := filepath.Base(mainPath)
	return filepath.Join(parentDirectory, baseName+"."+normalizeWorktreeName(branchName))
}

func (repository *Repository) branchExists(branchName string) (bool, error) {
	_, err := repository.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup branch %q: %w", branchName, err)
	}

	return true, nil
}

func defaultUpstreamBranch(repository *Repository) (string, plumbing.ReferenceName, error) {
	remoteHeadRef, err := repository.Reference(plumbing.NewRemoteHEADReferenceName(remoteName), false)
	if err == nil && remoteHeadRef.Type() == plumbing.SymbolicReference {
		return remoteHeadRef.Target().Short(), remoteHeadRef.Target(), nil
	}

	result, commandErr := repository.git("symbolic-ref", "refs/remotes/origin/HEAD")
	if commandErr != nil {
		return "", "", fmt.Errorf("resolve origin/HEAD: %w", err)
	}

	resolved := strings.TrimSpace(result.stdout)
	return plumbing.ReferenceName(resolved).Short(), plumbing.ReferenceName(resolved), nil
}

func currentRelativePath(currentDirectory string, targetPath string) string {
	relativePath, err := filepath.Rel(currentDirectory, targetPath)
	if err != nil {
		return targetPath
	}

	return relativePath
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
		if porcelainWorktree.BranchRef == "" || !strings.HasPrefix(porcelainWorktree.BranchRef, "refs/heads/") {
			continue
		}

		branchName := plumbing.ReferenceName(porcelainWorktree.BranchRef).Short()
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

func worktreeIsClean(repository *Repository, worktreePath string) (bool, error) {
	worktreeRepository := *repository
	worktreeRepository.WorkTree = worktreePath

	result, err := worktreeRepository.git("status", "--porcelain")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(result.stdout) == "", nil
}

func upstreamReference(repository *Repository, branchName string) (plumbing.ReferenceName, error) {
	branchConfig, err := repository.Branch(branchName)
	if err != nil {
		return "", fmt.Errorf("read branch config for %q: %w", branchName, err)
	}

	if branchConfig.Merge == "" {
		return "", fmt.Errorf("branch %q has no upstream branch", branchName)
	}

	if branchConfig.Remote == "" || branchConfig.Remote == "." {
		return branchConfig.Merge, nil
	}

	return plumbing.NewRemoteReferenceName(branchConfig.Remote, branchConfig.Merge.Short()), nil
}

func branchMergedToUpstream(repository *Repository, branchRef plumbing.ReferenceName, upstreamRef plumbing.ReferenceName) (bool, error) {
	_, err := repository.git("merge-base", "--is-ancestor", branchRef.String(), upstreamRef.String())
	if err == nil {
		return true, nil
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func enrichManagedWorktree(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
	upstreamRef, err := upstreamReference(repository, worktree.Name)
	if err != nil {
		return managedWorktree{}, err
	}

	clean, err := worktreeIsClean(repository, worktree.Path)
	if err != nil {
		return managedWorktree{}, err
	}

	merged, err := branchMergedToUpstream(repository, worktree.BranchReference, upstreamRef)
	if err != nil {
		return managedWorktree{}, err
	}

	worktree.UpstreamRef = upstreamRef
	worktree.Clean = clean
	worktree.Merged = merged

	return worktree, nil
}

func branchDeleteFlag(force bool) string {
	if force {
		return "-D"
	}

	return "-d"
}

func addBranchConfig(repository *Repository, branchName string, upstream string) error {
	upstreamRef := plumbing.ReferenceName(upstream)
	if !strings.HasPrefix(upstream, "refs/") {
		if strings.HasPrefix(upstream, remoteName+"/") {
			upstreamRef = plumbing.NewRemoteReferenceName(remoteName, strings.TrimPrefix(upstream, remoteName+"/"))
		} else {
			upstreamRef = plumbing.NewBranchReferenceName(upstream)
		}
	}

	branchConfig := &gitconfig.Branch{
		Name:   branchName,
		Remote: remoteName,
		Merge:  plumbing.NewBranchReferenceName(upstreamRef.Short()),
	}

	config, err := repository.Config()
	if err != nil {
		return fmt.Errorf("read repository config: %w", err)
	}

	if config.Branches == nil {
		config.Branches = map[string]*gitconfig.Branch{}
	}
	config.Branches[branchName] = branchConfig

	if err := repository.SetConfig(config); err != nil {
		return fmt.Errorf("write repository config: %w", err)
	}

	return nil
}
func branchStillExists(repository *Repository, branchRef plumbing.ReferenceName) (bool, error) {
	_, err := repository.Reference(branchRef, true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}
