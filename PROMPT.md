# Overview

Create a command line tool, named git-wt, that will use 'git worktree' under the hood, to manage Git worktrees.
It will have several subcommands:

- create <name> [-u|--upstream <upstream_branch>]
  - upstream should default to whatever resolved sym-ref is in .git/refs/remotes/origin/HEAD (hard-coded to origin/HEAD), e.g., origin/main
  - path should create a sibling directory of the workdir using a suffix that based on the normalized <name> that replaces / with .; for example, if the main worktree is at ~/myrepo then if <name> were nn/feat-1 it would create ~/myrepo.nn.feat-1
  - if the branch or directory already exist create should fail
  - will run `git worktree add -b <name> <path> <upstream_branch>`
  - the branch named <name> should have its upstream set to <upstream_branch>
  - if the normalized <name> is an invalid directory name then `git worktree add` will fail, we need not validate the name further
- list
  - can use `git worktree list --porcelain` but should just show "pretty" names for the worktrees and the relative path to its workdir
  - it should only show any Git worktrees that appear to have been created by git-wt based on them being siblings of the "main worktree" and using the . name normalization suffix
  - use a libgloss table
- prune [-p|--prompt]
  - can use the remove subcommand on any worktrees, from the list subcommand, that have had their branch merged to upstream branch and have clean workdirs (use the same logic as in the remove command)
  - unmerged/unclean worktrees should be silently skipped unless the --prompt option is provide in which case the user can decide, per worktree, whether to force removal
  - merged/clean worktree should be removed without confirmation
  - use github.com/charmbracelet/huh for the prompt, use a multi-select with the merged/clean worktree already selected, then remove can be called with --force since the user confirmed
- remove [-f|--force] <name>
  - can use `git worktree remove` but resolves the <worktree> argument from the <name>
  - if the worktree's workdir is not removed after running `git worktree remove` it should warn the user
  - `git worktree remove` might delete the branch but if not use `git branch -d` unless force was specified then use `git branch -D`
  - after remove runs both the worktree and its branch should be removed, the upstream_branch should not be touched
  - should fail if their branch has note been merged to its own upstream branch (use git merge-base --is-ancestor) or the workdir is not clean unless the --force option is provided to override this check
- fang should automatically inject the completion <shell> subcommand

# Technical Implementation Details

- Initialize a go module named github.com/nnutter/git-wt at the repo root, using Go 1.26.2
- All code should have test coverage. Tests should create a Git repo in order to actually be able to run Git commands. Focus on end-to-end tests that actually run git-wt commands inside the test Git repo over unit tests but create unit tests if you create helper functions that are moderately complex.
- Prefer using github.com/go-git/go-git/v6 over executing Git commands. I think that that package calls workdirs worktrees which is a misnomer so you will need to shell out to the git command to interact with Git worktrees.
- Use github.com/spf13/cobra to create the command
- Use github.com/charmbracelet/lipgloss/v2 for styling output (STDOUT and STDERR)
- log/slog can be used for logging but logging should really only be used for debugging message
- STDERR should be used for any status messages, using lipgloss
- STDOUT should only be used for output that could be piped to another command (perhaps no STDOUT)
- Create a struct to represent the subcommand flags and arguments
  - The struct should have a method Execute([]string) error to implement the RunE on a cobra.Command
  - A New\*() constructor should construct the cobra.Command, bind flags, etc. and return the cobra.Command to be bound to the root command
- Root command can just be a package var, in gitwt.go
- Each subcommand should be in its own file, e.g., gitwt_create.go
- The root command and all subcommands should be in an internal/gitwt package
- A main.go can execute the root command using charm.land/fang/v2
