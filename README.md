# git-wt

`git-wt` manages Git worktrees using a consistent naming convention.

Managed worktrees are stored next to the main repository using this path format:

`<repo>.<normalized-branch-name>`

Example:

- repo: `my-repo`
- branch: `feature/login`
- worktree path: `../my-repo.feature.login`

## Installation

Install using Go,

```bash
go install github.com/nnutter/git-wt@latest
```

## Shell integration

Generate a zsh function that wraps the CLI (default name `wt`):

```bash
git-wt generate zsh
# or: git-wt generate zsh --name wt --out $XDG_DATA_HOME/zsh/site-functions --force
```

Ensure the output directory is on `fpath`, then restart zsh or run `compinit`.

The generated function:

- routes most commands to `git-wt` (`wt create`, `wt list`, `wt prune`, …)
- after a successful `wt create`, `cd`s into the new worktree (`--no-cd` to stay put)
- provides a shell-only `switch` that `cd`s into a worktree
- after a successful `wt remove`, `cd`s to the main worktree

```bash
wt switch main
wt switch feature/login
wt create feature/login          # then cd into it
wt create --no-cd feature/login  # create only
wt remove feature/login          # then cd main
wt list
```

If you use [carapace](https://carapace.sh), exclude its built-in `wt` completer (worktrunk) so zsh uses the generated completion instead:

```bash
export CARAPACE_EXCLUDES=wt
```

Set this **before** `source <(carapace _carapace)`. You may need `carapace --clear-cache` after changing excludes.

## Commands

### `git-wt create <name>`

Create a managed worktree for a branch.

- If the branch already exists, the worktree is created from that branch.
- If the branch does not exist, it is created from the upstream branch; which defaults to the default origin branch but can be set explicity with `--upstream` | `-u`.

Example:

```bash
git-wt create feature/login
git-wt create -u origin/v1.2 hotfix/1.2.1
```

### `git-wt list`

List managed worktrees in a table.

Columns:

- `Name`: branch name
- `Path`: relative worktree path
- `Status`: first line of `git status -sb`
- `Dirty`: whether the worktree has uncommitted changes

### `git-wt migrate`

Bring existing branch worktrees under `git-wt` management.

- Creates managed worktrees for local branches that do not already have one.
- Renames existing non-managed branch worktrees into the managed path format.

Use `--prompt` | `-p` to review the proposed migrations before applying them.

Example:

```bash
git-wt migrate
git-wt migrate --prompt
```

### `git-wt prune`

Remove managed worktrees that are both clean, no uncommitted changes, and merged into their upstream branch.

Use `--prompt` | `-p` to choose which worktrees to prune interactively.

### `git-wt remove [name]`

Remove a managed worktree and delete its branch.

When `name` is omitted, removes the managed worktree that contains the current directory.
It refuses to remove the main worktree, and refuses dirty or unmerged worktrees by default.
Use `--force` | `-f` to force (destructive) removal.

When invoked through the shell wrapper (`wt remove`), the shell also switches to the main worktree after a successful removal.

Example:

```bash
git-wt remove
git-wt remove feature/login
git-wt remove --force feature/login
```

### `git-wt generate zsh`

Generate a zsh wrapper function and completion (see [Shell integration](#shell-integration)).

## Typical Flow

```bash
# once: install wrapper
git-wt generate zsh

# in a repo
wt create feature/login
wt switch feature/login
# ... work ...
wt switch main
wt prune
# or:
wt remove feature/login
```

For jumping between repositories under a path, you can still use something like
[git-cd](https://github.com/nnutter/dotfiles/blob/master/bin/git-cd).
