# git-wt

`git-wt` manages Git worktrees from registered bare repositories.

There is no required “main” worktree.
Repositories are stored as bare Git directories, and worktrees are created on demand under a shared root:

`<worktree-root>/<repo-name>/<worktree-name>/<repo-name>`

Defaults:

- bare repos: `$XDG_DATA_HOME/git-wt/repos/<repo-name>.git` (fallback: `~/.local/share/git-wt/repos/<repo-name>.git`)
- worktrees: `$GIT_WT_WORKTREE_ROOT/<repo-name>/<worktree-name>/<repo-name>` (fallback: `~/worktrees/<repo-name>/<worktree-name>/<repo-name>`)

The worktree name and branch name are identical (including `/`).

Example:

- repo name: `git-wt`
- bare repo: `~/.local/share/git-wt/repos/git-wt.git`
- branch: `nn/my-feature`
- worktree path: `~/worktrees/git-wt/nn/my-feature/git-wt`

Use `git-wt migrate` inside an existing clone to register it as a bare repo and rehome its worktrees (including the former main checkout) into this layout.
Use `git-wt migrate` inside a registered worktree to move that repository’s worktrees into this layout.
Use `git-wt migrate --all` to rehome worktrees for every registered repository.
When invoked through the shell wrapper (`wt migrate`), the shell also `cd`s to `$HOME` after success.

## Installation

Install using Go,

```bash
go install github.com/nnutter/git-wt@latest
```

`git-wt` requires Git on `PATH`.

## Development

This repository uses [mise](https://mise.jdx.dev) for tools and tasks.

```bash
mise install
lefthook install
mise run check
```

`mise run fmt` formats Go files.
`mise run fmt-check` reports formatting that does not match gofumpt.
`mise run lint` runs golangci-lint, nilaway, and phase-shift.
`mise run ci` runs `check`, gitleaks, and govulncheck.
`lefthook install` enables pre-commit formatting and gitleaks.
Pull requests run `mise run ci`.

## Shell integration

Generate a zsh function that wraps the CLI (default name `wt`):

```bash
git-wt generate zsh
# or: git-wt generate zsh --name wt --out $XDG_DATA_HOME/zsh/site-functions --force
```

The default command generates the wrapper `wt`, the completion `_wt`, and the autoload helper `_wt_autoload`.
The helper starts with `#autoload wt`, which lets `compinit` make `wt` available without `source` or an explicit `autoload` command.
With `--name foo`, the command generates `foo`, `_foo`, and `_foo_autoload`, and the helper starts with `#autoload foo`.
Ensure the output directory is on `fpath` before zsh runs `compinit`, then restart zsh or run `compinit`.

The generated function:

- routes most commands to `git-wt` (`wt create`, `wt list`, `wt prune`, …)
- after a successful `wt create`, `cd`s into the new worktree unless `--no-cd`, `--herdr`, or automatic Herdr workspace creation applies
- provides a shell-only `switch` that `cd`s into a worktree
- `wt switch -c` | `--create` creates the worktree first, then `cd`s, unless `--no-cd` or a Herdr space is opened
- When you are not in a managed worktree, `wt switch <name>` uses that worktree if the name exists in exactly one registered repository
- When you are not in a managed worktree, `wt switch <Tab>` completes worktree names from every registered repository
- If a completed name exists in more than one repository, completion adds `--repo` next
- `wt switch --all` | `-a` ignores the current worktree repository and uses the same unique-name rules
- after a successful `wt remove` or `wt migrate`, `cd`s to `$HOME`

```bash
wt repo add nnutter/git-wt
wt create --repo git-wt feature/login   # then cd into it
wt create --repo git-wt                 # random name, then cd into it
wt switch --repo git-wt feature/login
wt switch --all feature/login            # consider all repositories, not just the current repository
wt switch -c --repo git-wt feature/new   # create then cd
wt switch --repo git-wt feature/new -c   # same; -c can follow the name
wt setup-space                           # current worktree
wt setup-space --repo git-wt feature/login
wt create --no-cd --repo git-wt other   # create only
wt remove feature/login                 # then cd $HOME
wt list
wt list --all
wt list --repo git-wt
```

If you use [carapace](https://carapace.sh), exclude its built-in `wt` completer (worktrunk) so zsh uses the generated completion instead:

```bash
export CARAPACE_EXCLUDES=wt
```

Set this **before** `source <(carapace _carapace)`.
You may need `carapace --clear-cache` after changing excludes.

## Commands

### Repository selection

Worktree commands accept `-r` | `--repo <name>` to select a registered repository.

If `--repo` is omitted and the current directory is inside a managed worktree of a registered repository, that repository is used automatically.

`list` and `prune` use the current repository when inside a managed worktree.
Outside a managed worktree they use every registered repository.
Use `--repo` to force a single repository.
`list -a` | `--all` lists every registered repository even when inside a worktree.

Otherwise an interactive filter picker is shown for commands that need a single repository.
In non-interactive environments those commands fail unless `--repo` is set or the cwd auto-detects a managed repo.

### `git-wt repo add <url-or-path>`

Register a bare repository.

- Schema-less relative paths map to GitHub: `nnutter/git-wt` → `https://github.com/nnutter/git-wt`
- Full URLs, `git@host:path`, and local paths pass through unchanged
- `--name` overrides the derived repository name (default: basename of the URL)

Example:

```bash
git-wt repo add nnutter/git-wt
git-wt repo add --name my-fork git@github.com:me/git-wt.git
git-wt repo add /path/to/existing.git
```

### `git-wt repo list`

List registered repositories.
Use `-q` or `--quiet` to print only repository names, one name per line.

### `git-wt repo remove <name>`

Remove a registered bare repository.
Refuses if any worktrees remain.

### `git-wt repo rename <old-name> <new-name>`

Rename a registered bare repository and its managed worktree directories.
The command preserves local changes and leaves unmanaged linked worktrees at their existing paths.

The managed worktree path changes from:

```text
<worktree-root>/<old-name>/<worktree-name>/<old-name>
```

to:

```text
<worktree-root>/<new-name>/<worktree-name>/<new-name>
```

The `wt` wrapper changes to the new path if the current directory is inside a moved worktree.
The command refuses the rename if the destination repository or a destination worktree path exists.

Example:

```bash
git-wt repo rename git-wt git-worktree
```

### `git-wt create [name]`

Create a managed worktree for a branch.

- If the name is omitted, generates a random `<adjective>-<noun>` name themed around SpaceX, Starlink, and Tesla
- If the branch already exists, the worktree is created from that branch
- If the branch does not exist, it is created from the branch pointed at by `origin/HEAD`, or if that is unset from `origin/master` then `origin/main`; set it explicitly with `--upstream` | `-u`
- When run inside [Herdr](https://herdr.dev) (`HERDR_ENV=1`), automatically open the new worktree in a standard Herdr space
- The space contains an `Agent` tab that runs `pi`, an `Editor` tab that runs `nvim .`, and a `Shell` tab
- Use `--herdr` to open the space explicitly, or `--no-herdr` to suppress automatic creation
- Opening a Herdr space through `wt create` implies `--no-cd`
- Opening a Herdr space requires `herdr` on `PATH` and a running Herdr server

Example:

```bash
git-wt create --repo git-wt feature/login
git-wt create --repo git-wt
git-wt create -u origin/v1.2 hotfix/1.2.1
git-wt create --repo git-wt --herdr feature/login
```

### `git-wt setup-space [name]`

Set up a standard [Herdr](https://herdr.dev) space for a managed worktree.
By default the command defines the tabs in the current Herdr workspace.
It renames the current tab to `Agent` and adds the `Editor` and `Shell` tabs.
Use `-n` | `--new` to open a new Herdr workspace instead.

The workspace contains three tabs:

- `Agent`: runs `pi` in the worktree
- `Editor`: runs `nvim .` in the worktree
- `Shell`: opens an interactive shell in the worktree

If `name` is omitted, the command uses the managed worktree that contains the current directory.
Use `-r` | `--repo <name>` to select the repository for a specified worktree.
The command requires `herdr` on `PATH` and a running Herdr server.

Example:

```bash
git-wt setup-space --repo git-wt feature/login
git-wt setup-space --new --repo git-wt feature/login
cd ~/worktrees/git-wt/feature/login/git-wt
git-wt setup-space
```

### `git-wt list`

List managed worktrees in a table.

- Outside a managed worktree: list worktrees from every registered repository
- Inside a managed worktree: list only that repository’s worktrees
- `-a` | `--all`: list every registered repository even when inside a worktree
- `-r` | `--repo <name>`: list only the named repository

Columns:

- `Repo`: registered repository name
- `Name`: branch / worktree name
- `Status`: first line of `git status -sb`
- `Commit`: short commit hash
- `Dirty`: whether the worktree has uncommitted changes

### `git-wt migrate`

Register a clone as a bare repo, or rehome existing worktrees into the managed layout.

- Inside an unregistered clone: creates `$XDG_DATA_HOME/git-wt/repos/<name>.git` (override name with `--name`)
- Moves every branched worktree (including the former main checkout) to `$GIT_WT_WORKTREE_ROOT/<repo-name>/<branch>/<repo-name>` (fallback: `~/worktrees/...`)
- Inside a registered worktree: moves that repository’s worktrees that are not already at the managed path
- `--all` | `-a`: rehomes worktrees for every registered repository
- Removes empty parent directories of the old checkout, up to `$HOME`
- If the clone has no linked worktrees and HEAD is the default branch (`origin/HEAD`, else `origin/master` / `origin/main`), only the bare repo is registered (no managed worktree is created)
- Does not create worktrees for local branches that do not already have one
- Use `--prompt` | `-p` to choose which worktrees to migrate

Example:

```bash
cd ~/src/github.com/nnutter/git-wt
git-wt migrate
git-wt migrate --name git-wt --prompt
cd ~/worktrees/next/git-wt
git-wt migrate
git-wt migrate --all
```

### `git-wt prune`

Remove managed worktrees that are both clean and merged into their upstream branch.

Without `-r` | `--repo`, prune uses the current repository inside a managed worktree and every registered repository otherwise.
Use `--prompt` | `-p` to choose which worktrees to prune interactively.
Use `-n` | `--dry-run` to list the worktrees that would be pruned without removing them.

### `git-wt remove [name]`

Remove a managed worktree and delete its branch.

When `name` is omitted, removes the managed worktree that contains the current directory (auto-detects the registered repo from cwd, or use `-r` | `--repo` / the repo picker).
Refuses dirty or unmerged worktrees by default.
Use `--force` | `-f` to force (destructive) removal.

When invoked through the shell wrapper (`wt remove`), the shell also `cd`s to `$HOME` after a successful removal.

Example:

```bash
git-wt remove
git-wt remove --repo git-wt feature/login
git-wt remove --repo git-wt --force feature/login
```

### `git-wt generate zsh`

Generate a zsh wrapper function, completion, and autoload helper (see [Shell integration](#shell-integration)).

## Typical Flow

```bash
# once: install wrapper
git-wt generate zsh

# register a repo
wt repo add nnutter/git-wt

# day to day
wt create --repo git-wt feature/login
wt switch --repo git-wt feature/login
# ... work ...
wt switch --repo git-wt main   # if you created a main worktree
wt prune --repo git-wt
# or:
wt remove feature/login
```
