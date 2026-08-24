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

## Herdr Plugin

There is a Herdr plugin in `herdr`.
Install it with `mise run install-herdr-plugin`.
That task copies the plugin to `~/.config/herdr/plugins/nnutter.git-wt` and runs `herdr plugin link` on the copy.
Copying the files is not enough on its own: Herdr only registers actions after `plugin link` or `plugin install`.
The popup runs `git-wt tui create --herdr` after it adds common tool paths.
Then assign a keybinding in `~/.config/herdr/config.toml`,

```toml
[[keys.command]]
key = "prefix+shift+s"
type = "plugin_action"
command = "nnutter.git-wt.open"
description = "create git-wt space"
```

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
- `wt switch <name>` uses that worktree if the name exists in exactly one registered repository
- `wt switch <Tab>` completes worktree names from every registered repository
- Unique names complete without `@`
- If a name exists in more than one repository, completion offers `<name>@<repo>` for each one (`artisinal@liaison`, `artisinal@persona`)
- after a successful `wt remove` or `wt migrate`, `cd`s to `$HOME`

```bash
wt repo add nnutter/git-wt
wt create feature/login@git-wt   # then cd into it
wt create @git-wt                # random name, then cd into it
wt switch feature/login@git-wt
wt switch feature/login          # unique name across repositories
wt switch -c feature/new@git-wt  # create then cd
wt switch feature/new@git-wt -c  # same; -c can follow the name
wt tui create                    # pick a repo and name
wt setup-space                   # current worktree
wt setup-space feature/login@git-wt
wt create --no-cd other@git-wt   # create only
wt remove feature/login          # then cd $HOME
wt list
wt list @git-wt
```

If you use [carapace](https://carapace.sh), exclude its built-in `wt` completer (worktrunk) so zsh uses the generated completion instead:

```bash
export CARAPACE_EXCLUDES=wt
```

Set this **before** `source <(carapace _carapace)`.
You may need `carapace --clear-cache` after changing excludes.

## Commands

### Repository selection

Worktree commands take an optional `<worktree>@<repo>` qualifier on the name argument.

- `feature/login@git-wt` selects that worktree in the `git-wt` repository
- `feature/login` is enough when the name exists in exactly one registered repository
- `@git-wt` selects the repository with no worktree name (`create` generates a random name; `list` and `prune` pin that repository)

`list` and `prune` use every registered repository unless `@<repo>` pins one.

`create` (and `switch -c`) use the current repository when the cwd is a managed worktree of a registered repo, otherwise an interactive picker.
`remove` and `setup-space` with no name target the managed worktree that contains the cwd.

In non-interactive environments commands that need a single repository fail unless `@<repo>` is set or the cwd auto-detects a managed repo.

Worktree names must not contain `@`.

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

### `git-wt create [name[@repo]]`

Create a managed worktree for a branch.

- Qualify the name as `<worktree>@<repo>` to select the repository; `@<repo>` alone generates a random name in that repository
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
git-wt create feature/login@git-wt
git-wt create @git-wt
git-wt create -u origin/v1.2 hotfix/1.2.1
git-wt create --herdr feature/login@git-wt
```

### `git-wt tui create`

Interactively pick a registered repository and type a worktree name, then create that worktree.

- Always lists every registered repository, including when the current directory is already a managed worktree
- Never generates a random name
- Requires an interactive terminal
- When run inside [Herdr](https://herdr.dev) (`HERDR_ENV=1`), opens a new standard Herdr space unless `--no-herdr` is set
- Use `--herdr` to open the space explicitly

Example:

```bash
git-wt tui create
git-wt tui create --herdr
```

### `git-wt setup-space [name[@repo]]`

Set up a standard [Herdr](https://herdr.dev) space for a managed worktree.
By default the command defines the tabs in the current Herdr workspace.
It renames the current tab to `Agent` and adds the `Editor` and `Shell` tabs.
Use `-n` | `--new` to open a new Herdr workspace instead.

The workspace contains three tabs:

- `Agent`: runs `pi` in the worktree
- `Editor`: runs `nvim .` in the worktree
- `Shell`: opens an interactive shell in the worktree

If `name` is omitted, the command uses the managed worktree that contains the current directory.
Qualify the name as `<worktree>@<repo>` to select a worktree in another repository.
The command requires `herdr` on `PATH` and a running Herdr server.

Example:

```bash
git-wt setup-space feature/login@git-wt
git-wt setup-space --new feature/login@git-wt
cd ~/worktrees/git-wt/feature/login/git-wt
git-wt setup-space
```

### `git-wt list [@repo]`

List managed worktrees in a table.

- Default: list worktrees from every registered repository
- `@<repo>`: list only the named repository

Columns:

- `Name`: branch / worktree name
- `Repo`: registered repository name
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

### `git-wt prune [@repo]`

Remove managed worktrees that are both clean and merged into their upstream branch.

Without `@<repo>`, prune considers every registered repository.
Use `--prompt` | `-p` to choose which worktrees to prune interactively.
Use `-n` | `--dry-run` to list the worktrees that would be pruned without removing them.

### `git-wt remove [name[@repo]]`

Remove a managed worktree and delete its branch.

When `name` is omitted, removes the managed worktree that contains the current directory (auto-detects the registered repo from cwd, or use `<worktree>@<repo>` / the repo picker).
A unique worktree name is enough from outside a managed worktree.
Refuses dirty or unmerged worktrees by default.
Use `--force` | `-f` to force (destructive) removal.

When invoked through the shell wrapper (`wt remove`), the shell also `cd`s to `$HOME` after a successful removal.

Example:

```bash
git-wt remove
git-wt remove feature/login@git-wt
git-wt remove --force feature/login@git-wt
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
wt create feature/login@git-wt
wt switch feature/login@git-wt
# ... work ...
wt switch main@git-wt   # if you created a main worktree
wt prune @git-wt
# or:
wt remove feature/login
```
