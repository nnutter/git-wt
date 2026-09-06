package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// trashCommandName is the trash CLI used to move removed paths to the
// system trash instead of deleting them outright.
const trashCommandName = "trash"

// importWorktree is one source worktree that will be recreated under the
// managed Timber layout.
type importWorktree struct {
	Name        string
	BranchName  string // empty for detached worktrees
	CommitHash  string
	CurrentPath string
	TargetPath  string
	Detached    bool
	StagingPath string
}

// importSkip is a source worktree that cannot be moved; its reason is shown in
// the summary so nothing disappears silently.
type importSkip struct {
	Path   string
	Reason string
}

type importPlan struct {
	runtime   Runtime
	source    *Repository
	repoName  string
	barePath  string
	mainPath  string
	worktrees []importWorktree
	skips     []importSkip
}

type repoImportCommandOptions struct {
	runtime Runtime
	name    string
}

func NewRepoImportCommand(runtime Runtime) *cobra.Command {
	options := &repoImportCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:   "import <path>",
		Short: "Convert an existing clone into a managed Timber repository",
		Args:  cobra.ExactArgs(1),
		RunE:  options.Execute,
	}
	command.Flags().StringVar(&options.name, "name", "", "Repository name (default: derived from remote or checkout)")

	return command
}

func (x *repoImportCommandOptions) Execute(command *cobra.Command, args []string) error {
	sourcePath, err := x.runtime.absolutePath(args[0])
	if err != nil {
		return err
	}

	plan, err := x.runtime.buildImportPlan(sourcePath, normalizeRepoName(x.name))
	if err != nil {
		return err
	}
	return plan.run(command)
}

// rejectRegisteredSource refuses to import a repository that is already
// registered so the bare clone cannot shadow an existing registration.
func rejectRegisteredSource(runtime Runtime, commonDir string) error {
	repos, err := runtime.listRegisteredRepos()
	if err != nil {
		return err
	}
	for _, repo := range repos {
		same, err := samePath(repo.BarePath, commonDir)
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("repository is already registered as %q", repo.Name)
		}
	}
	return nil
}

func pathIsBareRepository(runtime Runtime, path string) (bool, error) {
	result, err := gitOutput(runtime, path, "rev-parse", "--is-bare-repository")
	if err != nil {
		return false, err
	}
	return result.stdout == "true", nil
}

// collectWorktrees inventories the source repository. Prunable and unborn
// worktrees are recorded as skips instead of blocking the import; their
// branches still survive in the new bare clone.
func (x *importPlan) collectWorktrees(porcelainWorktrees []porcelainWorktree) error {
	for _, porcelainWorktree := range porcelainWorktrees {
		if porcelainWorktree.Prunable != "" {
			x.skips = append(x.skips, importSkip{
				Path:   porcelainWorktree.Path,
				Reason: porcelainWorktree.Prunable,
			})
			continue
		}
		if isZeroCommitHash(porcelainWorktree.CommitHash) {
			x.skips = append(x.skips, importSkip{
				Path:   porcelainWorktree.Path,
				Reason: "no commits yet",
			})
			continue
		}

		branchName := porcelainWorktree.branchName()
		worktree := importWorktree{
			CommitHash:  porcelainWorktree.CommitHash,
			CurrentPath: porcelainWorktree.Path,
			Detached:    branchName == "",
		}
		if worktree.Detached {
			worktree.Name = shortCommitHash(porcelainWorktree.CommitHash)
		} else {
			worktree.Name = branchName
			worktree.BranchName = branchName
		}
		worktree.TargetPath = x.runtime.managedWorktreePath(x.repoName, worktree.Name)
		x.worktrees = append(x.worktrees, worktree)
	}
	return nil
}

// validateTargets fails before anything is touched when two worktrees would
// collide or a target path already exists.
func (x *importPlan) validateTargets() error {
	targets := make(map[string]string, len(x.worktrees))
	for _, worktree := range x.worktrees {
		targetPath := filepath.Clean(worktree.TargetPath)
		if existingName, ok := targets[targetPath]; ok {
			return fmt.Errorf("worktrees %q and %q share target path %q", existingName, worktree.Name, worktree.TargetPath)
		}
		targets[targetPath] = worktree.Name

		if _, err := os.Stat(worktree.TargetPath); err == nil {
			return fmt.Errorf("worktree directory %q already exists", worktree.TargetPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect worktree directory %q: %w", worktree.TargetPath, err)
		}
	}
	return nil
}

// run performs the import in fail-safe order:
//
//  1. Stage every worktree's contents (including untracked and uncommitted
//     files) to temporary directories before anything is moved.
//  2. Clone the source bare and register it.
//  3. Recreate each worktree from the new bare and restore its staged
//     contents. Old worktrees are only removed after this succeeds.
//  4. Move the old worktrees to the system trash with the trash CLI and
//     remove their empty parent directories.
//
// A failure in step 3 rolls back the freshly created worktrees and the bare
// clone, leaving the source repository untouched.
func (x *importPlan) run(command *cobra.Command) (retErr error) {
	for index := range x.worktrees {
		worktree := &x.worktrees[index]
		stagingPath, err := x.runtime.temporaryPath("timber-import-")
		if err != nil {
			return fmt.Errorf("create import staging directory: %w", err)
		}
		worktree.StagingPath = stagingPath
	}
	defer func() {
		for _, worktree := range x.worktrees {
			if worktree.StagingPath == "" {
				continue
			}
			if err := os.RemoveAll(worktree.StagingPath); retErr == nil && err != nil {
				retErr = err
			}
		}
	}()

	for index := range x.worktrees {
		worktree := &x.worktrees[index]
		if err := copyDirectoryContents(worktree.CurrentPath, worktree.StagingPath, ".git"); err != nil {
			return fmt.Errorf("stage worktree %q: %w", worktree.CurrentPath, err)
		}
	}

	if err := ensureDirectory(filepath.Dir(x.barePath)); err != nil {
		return err
	}
	if _, err := gitOutput(x.runtime, x.mainPath, "clone", "--bare", x.mainPath, x.barePath); err != nil {
		return err
	}
	if err := setupMigratedBareOrigin(x.runtime, x.source, x.barePath); err != nil {
		return err
	}

	stderr := command.ErrOrStderr()
	registeredMessage := fmt.Sprintf(
		"registered repository %s at %s",
		x.repoName,
		x.runtime.displayHomePath(x.barePath),
	)
	if _, err := fmt.Fprintf(stderr, "%s\n", statusStyle.Render(registeredMessage)); err != nil {
		return err
	}

	bareRepository, err := openBareRepository(x.runtime, x.barePath)
	if err != nil {
		return err
	}

	created := make([]*importWorktree, 0, len(x.worktrees))
	for index := range x.worktrees {
		worktree := &x.worktrees[index]
		if err := x.createWorktree(bareRepository, worktree); err != nil {
			return errors.Join(err, x.rollbackCreated(bareRepository, created))
		}
		created = append(created, worktree)
	}

	trashablePaths := make([]string, 0, len(x.worktrees))
	for _, worktree := range x.worktrees {
		trashablePaths = append(trashablePaths, worktree.CurrentPath)
	}
	if err := x.runtime.trashPaths(trashablePaths...); err != nil {
		return err
	}
	for _, worktree := range x.worktrees {
		if err := x.runtime.removeEmptySourceParents(worktree.CurrentPath); err != nil {
			return err
		}
	}

	return x.reportSummary(command)
}

func (x *importPlan) createWorktree(bareRepository *Repository, worktree *importWorktree) error {
	var err error
	if worktree.Detached {
		_, err = bareRepository.git("worktree", "add", "--detach", worktree.TargetPath, worktree.CommitHash)
	} else {
		_, err = bareRepository.git("worktree", "add", worktree.TargetPath, worktree.BranchName)
	}
	if err != nil {
		return fmt.Errorf("create worktree %q: %w", worktree.TargetPath, err)
	}

	// Restore uncommitted, untracked, and ignored content over the clean
	// checkout. The source copy is still intact, so a failure here is safe.
	if err := copyDirectoryContents(worktree.StagingPath, worktree.TargetPath, ".git"); err != nil {
		return fmt.Errorf("restore worktree contents to %q: %w", worktree.TargetPath, err)
	}

	if worktree.Detached {
		return nil
	}
	return ensureBranchUpstream(bareRepository, worktree.BranchName)
}

// rollbackCreated removes worktrees freshly created under the managed layout
// plus the bare clone so a failed import can simply be retried. The source
// worktrees are still on disk at this point, and removals go to the system
// trash, so nothing is lost.
func (x *importPlan) rollbackCreated(bareRepository *Repository, created []*importWorktree) error {
	var rollbackErrors []error
	for _, worktree := range created {
		if _, err := bareRepository.git("worktree", "remove", "--force", worktree.TargetPath); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
		if _, err := os.Stat(worktree.TargetPath); err == nil {
			if trashErr := x.runtime.trashPaths(worktree.TargetPath); trashErr != nil {
				rollbackErrors = append(rollbackErrors, trashErr)
			}
		}
	}
	if _, err := os.Stat(x.barePath); err == nil {
		if trashErr := x.runtime.trashPaths(x.barePath); trashErr != nil {
			rollbackErrors = append(rollbackErrors, trashErr)
		}
	}
	return errors.Join(rollbackErrors...)
}

func (x *importPlan) reportSummary(command *cobra.Command) error {
	stderr := command.ErrOrStderr()

	for _, skip := range x.skips {
		message := fmt.Sprintf(
			"skipped worktree %s: %s",
			x.runtime.displayHomePath(skip.Path),
			skip.Reason,
		)
		if _, err := fmt.Fprintf(stderr, "%s\n", warningStyle.Render(message)); err != nil {
			return err
		}
	}

	message := fmt.Sprintf("imported %d worktrees:", len(x.worktrees))
	if _, err := fmt.Fprintf(stderr, "%s\n", statusStyle.Render(message)); err != nil {
		return err
	}
	for _, worktree := range x.worktrees {
		message := fmt.Sprintf(
			"  %s: %s -> %s",
			worktree.Name,
			x.runtime.displayHomePath(worktree.CurrentPath),
			x.runtime.displayHomePath(worktree.TargetPath),
		)
		if _, err := fmt.Fprintf(stderr, "%s\n", statusStyle.Render(message)); err != nil {
			return err
		}
	}

	if len(x.worktrees) > 0 {
		note := "old worktrees were moved to the system trash; restore them from there if needed"
		if _, err := fmt.Fprintf(stderr, "%s\n", statusStyle.Render(note)); err != nil {
			return err
		}
	}
	return nil
}

func shortCommitHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}

// isZeroCommitHash reports whether a worktree has no commits yet, where Git
// reports an all-zero HEAD.
func isZeroCommitHash(hash string) bool {
	return strings.Trim(hash, "0") == ""
}
