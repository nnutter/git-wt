# git-wt

`git-wt` manages Git worktrees using a consistent path layout.

Every managed worktree lives under a shared root, with the worktree/branch name as an intermediate directory and the repository name as the final checkout directory:

`<worktree-root>/<worktree-name>/<repo-name>`

The worktree name and branch name are identical (including `/`).
The main worktree uses the name `main`.

Example:

- worktree root: `~/src/github.com/nnutter/git-wt`
- repo name: `git-wt`
- main worktree: `~/src/github.com/nnutter/git-wt/main/git-wt`
- branch: `nn/my-feature`
- worktree path: `~/src/github.com/nnutter/git-wt/nn/my-feature/git-wt`

Use `git-wt migrate` to move existing worktrees (including main) into this layout.

## Installation

Install using Go,

```bash
go install github.com/nnutter/git-wt@latest
```

`git-wt` requires Git on `PATH`.

## Shell integration

Generate a zsh function that wraps the CLI (default name `wt`):

```bash
git-wt generate zsh
# or: git-wt generate zsh --name wt --out $XDG_DATA_HOME/zsh/site-functions --force
```

Ensure the output directory is on `fpath`, then restart zsh or run `compinit`.

The generated function:

- routes most commands to `git-wt` (`wt create`, `wt list`, `wt prune`, …)
- after a successful `wt create`, `cd`s into the new worktree unless `--no-cd`, `-r` | `--herdr`, or automatic Herdr workspace creation applies
- provides a shell-only `switch` that `cd`s into a worktree
- after a successful `wt remove`, `cd`s to the main worktree
- after a successful `wt off`, `cd`s to the collapsed worktree root

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
- If the branch does not exist, it is created from the branch pointed at by `origin/HEAD`, or if that is unset from `origin/master` then `origin/main`; set it explicitly with `--upstream` | `-u`.
- When run inside [Herdr](https://herdr.dev) (`HERDR_ENV=1`), automatically create a Herdr workspace whose `--cwd` is the new worktree and whose `--label` is the worktree name (branch name).
- Use `-r` | `--herdr` to create a Herdr workspace explicitly, or `-R` | `--no-herdr` to suppress automatic creation.
- Herdr workspace creation through `wt create` implies `--no-cd`.
- Herdr workspace creation requires `herdr` on `PATH` and a running Herdr server.

Example:

```bash
git-wt create feature/login
git-wt create -u origin/v1.2 hotfix/1.2.1
git-wt create -r feature/login
git-wt create -R feature/login
```

### `git-wt list`

List managed worktrees in a table.

Columns:

- `Name`: `main (<branch>)` for the main worktree, otherwise the branch name
- `Status`: first line of `git status -sb`
- `Dirty`: whether the worktree has uncommitted changes

### `git-wt migrate`

Bring existing Git worktrees under `git-wt` management.

- Moves the main worktree into `<root>/main/<repo-name>` when it is still a plain clone at `<root>` or on the old layout at `<root>/main`.
- Renames existing non-managed branch worktrees into the managed path format.
- Does not create worktrees for local branches that do not already have one.

Use `--prompt` | `-p` to review the proposed migrations before applying them.

Example:

```bash
git-wt migrate
git-wt migrate --prompt
```

### `git-wt off`

Tear down the managed worktree layout into a single checkout at the worktree root.

- Refuses if any managed worktree (including main) is dirty unless `--force` | `-f`.
- Removes every non-main managed worktree.
- Deletes a feature branch with `git branch -d` when it is fully merged; otherwise keeps the branch.
- Moves `<root>/main/<repo-name>` to `<root>` so the repository is a normal single checkout.

When invoked through the shell wrapper (`wt off`), the shell also `cd`s to the collapsed root after success.

Example:

```bash
git-wt off
git-wt off --force
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
