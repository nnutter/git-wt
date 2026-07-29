package gitwt

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type offCommandOptions struct {
	force bool
}

func NewOffCommand() *cobra.Command {
	options := new(offCommandOptions)

	command := &cobra.Command{
		Use:   "off",
		Short: "Tear down managed worktrees and collapse to a single checkout",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}

	command.Flags().BoolVarP(&options.force, "force", "f", false, "Allow dirty worktrees")

	return command
}

func (x *offCommandOptions) Execute(command *cobra.Command, args []string) error {
	repository, err := openRepository(".")
	if err != nil {
		return err
	}

	worktrees, mainPath, err := managedWorktreesFromRepository(repository)
	if err != nil {
		return err
	}

	if !mainIsNestedLayout(mainPath) {
		return fmt.Errorf("main worktree is not in managed nested layout (%s)", mainPath)
	}

	rootPath := worktreeRoot(mainPath)
	if filepath.Clean(repository.WorkTree) != filepath.Clean(mainPath) {
		repository, err = openRepository(mainPath)
		if err != nil {
			return err
		}
	}

	if err := ensureManagedWorktreesClean(worktrees, x.force); err != nil {
		return err
	}

	for _, worktree := range worktrees {
		if worktree.Main {
			continue
		}
		if err := removeManagedWorktreeForOff(repository, command.ErrOrStderr(), worktree, x.force); err != nil {
			return err
		}
	}

	if err := collapseMainToRoot(repository, command.ErrOrStderr(), mainPath, rootPath); err != nil {
		return err
	}

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("collapsed main to "+rootPath))
	return err
}

func ensureManagedWorktreesClean(worktrees []managedWorktree, force bool) error {
	if force {
		return nil
	}

	for _, worktree := range worktrees {
		worktreeRepository, err := openRepository(worktree.Path)
		if err != nil {
			return err
		}
		clean, err := worktreeRepository.isClean()
		if err != nil {
			return err
		}
		if !clean {
			return fmt.Errorf("worktree %q is not clean", worktree.Name)
		}
	}

	return nil
}

func removeManagedWorktreeForOff(repository *Repository, stderr io.Writer, worktree managedWorktree, force bool) error {
	removeArguments := []string{"worktree", "remove"}
	if force {
		removeArguments = append(removeArguments, "--force")
	}
	removeArguments = append(removeArguments, worktree.Path)
	if _, err := repository.git(removeArguments...); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stderr, "%s\n", statusStyle.Render("removed worktree "+worktree.Name)); err != nil {
		return err
	}

	branchExists, err := repository.branchStillExists(worktree.BranchReference)
	if err != nil {
		return err
	}
	if !branchExists {
		return removeEmptyParents(worktree.Path, worktreeRoot(repository.WorkTree))
	}

	if _, err := repository.git("branch", "-d", worktree.Name); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "%s\n", warningStyle.Render("kept branch "+worktree.Name+" (not fully merged)")); writeErr != nil {
			return writeErr
		}
	} else {
		if _, writeErr := fmt.Fprintf(stderr, "%s\n", statusStyle.Render("deleted branch "+worktree.Name)); writeErr != nil {
			return writeErr
		}
	}

	return removeEmptyParents(worktree.Path, worktreeRoot(repository.WorkTree))
}

// collapseMainToRoot moves <root>/main/<repo> to <root> via a temporary sibling
// path, then merges the checkout into the root directory.
func collapseMainToRoot(repository *Repository, stderr io.Writer, mainPath string, rootPath string) error {
	parentOfRoot := filepath.Dir(rootPath)
	temporaryPath := filepath.Join(parentOfRoot, ".git-wt-off-"+filepath.Base(rootPath))
	if _, err := os.Stat(temporaryPath); err == nil {
		return fmt.Errorf("temporary off path %q already exists", temporaryPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect temporary off path %q: %w", temporaryPath, err)
	}

	if err := os.Rename(mainPath, temporaryPath); err != nil {
		return fmt.Errorf("move main worktree to temporary path: %w", err)
	}

	if err := removeEmptyParents(mainPath, rootPath); err != nil {
		return err
	}

	if err := mergeDirectoryContents(temporaryPath, rootPath); err != nil {
		return err
	}
	if err := os.Remove(temporaryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove temporary off path %q: %w", temporaryPath, err)
	}

	repository.WorkTree = rootPath
	repository.GitDir = filepath.Join(rootPath, ".git")
	if _, err := repository.git("worktree", "repair"); err != nil {
		return err
	}

	_, err := fmt.Fprintf(stderr, "%s\n", statusStyle.Render("moved main worktree to "+rootPath))
	return err
}

func mergeDirectoryContents(sourceDirectory string, destinationDirectory string) error {
	entries, err := os.ReadDir(sourceDirectory)
	if err != nil {
		return fmt.Errorf("read temporary checkout %q: %w", sourceDirectory, err)
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDirectory, entry.Name())
		destinationPath := filepath.Join(destinationDirectory, entry.Name())
		if _, err := os.Stat(destinationPath); err == nil {
			return fmt.Errorf("cannot collapse main to %q: %q already exists", destinationDirectory, destinationPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect %q: %w", destinationPath, err)
		}
		if err := os.Rename(sourcePath, destinationPath); err != nil {
			return fmt.Errorf("move %q to %q: %w", sourcePath, destinationPath, err)
		}
	}

	return nil
}

// removeEmptyParents removes path and empty ancestor directories up to (but not
// including) stopPath.
func removeEmptyParents(path string, stopPath string) error {
	current := filepath.Clean(path)
	stopPath = filepath.Clean(stopPath)

	for {
		if current == stopPath || current == string(filepath.Separator) || current == "." {
			return nil
		}
		if !pathIsWithin(stopPath, current) {
			return nil
		}

		err := os.Remove(current)
		if err == nil {
			current = filepath.Dir(current)
			continue
		}
		if os.IsNotExist(err) {
			current = filepath.Dir(current)
			continue
		}
		// Directory not empty or not removable — stop walking up.
		if isNotEmptyError(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", current, err)
	}
}

func isNotEmptyError(err error) bool {
	if err == nil {
		return false
	}
	// POSIX: directory not empty; also match path errors from os.Remove.
	if pathError, ok := err.(*fs.PathError); ok {
		err = pathError.Err
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not empty") || strings.Contains(message, "directory not empty")
}
