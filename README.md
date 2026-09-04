# timber

`timber` manages Git worktrees from registered bare repositories.

There is no required “main” worktree.
Repositories are stored as bare Git directories, and worktrees are created on demand under a shared root:

`<worktree-root>/<repo-name>/<worktree-name>/<repo-name>`

Defaults:

- bare repos: `$XDG_DATA_HOME/timber/repos/<repo-name>.git` (fallback: `~/.local/share/timber/repos/<repo-name>.git`)
- worktrees: `$TIMBER_WORKTREE_ROOT/<repo-name>/<worktree-name>/<repo-name>` (fallback: `~/worktrees/<repo-name>/<worktree-name>/<repo-name>`)

The worktree name and branch name are identical (including `/`).

Example:

- repo name: `timber`
- bare repo: `~/.local/share/timber/repos/timber.git`
- branch: `nn/my-feature`
- worktree path: `~/worktrees/timber/nn/my-feature/timber`

Use `timber migrate` inside an existing clone to register it as a bare repo and rehome its worktrees (including the former main checkout) into this layout.
Use `timber migrate` inside a registered worktree to move that repository’s worktrees into this layout.
Use `timber migrate --all` to rehome worktrees for every registered repository.
When invoked through the shell wrapper (`t migrate`), the shell also `cd`s to `$HOME` after success.

## Installation

Install using Go,

```bash
go install github.com/nnutter/timber@latest
```

`timber` requires Git on `PATH`.

## Herdr Plugin

Install the bundled [Herdr](https://herdr.dev) plugin with:

```bash
timber herdr install
```

That command writes the plugin to `~/.config/herdr/plugins/timber` (`$XDG_CONFIG_HOME/herdr/plugins/timber` when set) and runs `herdr plugin link` on the copy.
Copying the files is not enough on its own: Herdr only registers actions after `plugin link` or `plugin install`.
The popup runs `timber tui --herdr --no-title` after it adds common tool paths.
The TUI can create a new worktree or open a Herdr space for an existing one.
The command prints a keybinding snippet to add to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+shift+s"
type = "plugin_action"
command = "nnutter.timber.open"
description = "open or create Timber Space"
```

## Development

This repository uses [mise](https://mise.jdx.dev) for tools and tasks.

```bash
mise install
lefthook install
mise run all
```

`mise run all` runs formatting, fixes, tests, and static analysis.
`mise run format` (or `mise run fmt`) formats Go files.
`mise run fix` applies `go fix`, tidies the modules, and runs `go vet -fix`.
`mise run tests` (or `mise run test`) runs the test suite with the race detector.
`mise run static-analysis` runs the configured static-analysis tools, including gitleaks.
`mise run coverage` (or `mise run cover`) runs the tests and prints coverage.
`lefthook install` enables the pre-commit hook, which runs `mise run pre-commit` to format staged Go files.

## Shell integration

Generate a zsh function that wraps the CLI (default name `t`):

```bash
timber generate zsh
# or: timber generate zsh --name t --out $XDG_DATA_HOME/zsh/site-functions --force
```

The default command generates the wrapper `t`, the completion `_t`, and the autoload helper `_t_autoload`.
The helper starts with `#autoload t`, which lets `compinit` make `t` available without `source` or an explicit `autoload` command.
With `--name foo`, the command generates `foo`, `_foo`, and `_foo_autoload`, and the helper starts with `#autoload foo`.
Ensure the output directory is on `fpath` before zsh runs `compinit`, then restart zsh or run `compinit`.

The generated function:

- routes most commands to `timber` (`t create`, `t list`, `t prune`, …)
- after a successful `t create`, `cd`s into the new worktree unless `--no-cd`, `--herdr`, or automatic Herdr workspace creation applies
- provides a shell-only `switch` that `cd`s into a worktree
- `t switch -c` | `--create` creates the worktree first, then `cd`s, unless `--no-cd` or a Herdr space is opened
- `t switch <name>` uses that worktree if the name exists in exactly one registered repository
- `t switch <Tab>` completes worktree names from every registered repository
- Unique names complete without `@`
- If a name exists in more than one repository, completion offers `<name>@<repo>` for each one (`artisinal@liaison`, `artisinal@persona`)
- after a successful `t remove` or `t migrate`, `cd`s to `$HOME`

```bash
t repo add nnutter/timber
t create feature/login@timber   # then cd into it
t create @timber                # random name, then cd into it
t switch feature/login@timber
t switch feature/login          # unique name across repositories
t switch -c feature/new@timber  # create then cd
t switch feature/new@timber -c  # same; -c can follow the name
t tui                           # open an existing worktree or create a new one
t herdr install                 # install the Herdr plugin
t herdr space                   # current worktree
t herdr space feature/login@timber
t create --no-cd other@timber   # create only
t remove feature/login          # then cd $HOME
t list
t list @timber
```

## Commands

The following aliases are available for the commands below:

| Command | Alias |
| --- | --- |
| `list` | `ls` |
| `prune` | `clean` |
| `remove` | `rm` |

The nested command groups have these aliases as well:

- `repo list` → `repo ls`
- `repo remove` → `repo rm`
- `repo rename` → `repo mv`

### Repository selection

Worktree commands take an optional `<worktree>@<repo>` qualifier on the name argument.

- `feature/login@timber` selects that worktree in the `timber` repository
- `feature/login` is enough when the name exists in exactly one registered repository
- `@timber` selects the repository with no worktree name (`create` generates a random name; `list` and `prune` pin that repository)

`list` and `prune` use every registered repository unless `@<repo>` pins one.

`create` (and `switch -c`) use the current repository when the cwd is a managed worktree of a registered repo, otherwise an interactive picker.
`remove` and `herdr space` with no name target the managed worktree that contains the cwd.

In non-interactive environments commands that need a single repository fail unless `@<repo>` is set or the cwd auto-detects a managed repo.

Worktree names must not contain `@`.

### `timber repo add <url-or-path>`

Register a bare repository.

- Schema-less relative paths map to GitHub: `nnutter/timber` → `https://github.com/nnutter/timber`
- Full URLs, `git@host:path`, and local paths pass through unchanged
- `--name` overrides the derived repository name (default: basename of the URL)

Example:

```bash
timber repo add nnutter/timber
timber repo add --name my-fork git@github.com:me/timber.git
timber repo add /path/to/existing.git
```

### `timber repo list`

List registered repositories, including each repository's origin URL.
Use `-q` or `--quiet` to print only repository names, one name per line.

### `timber repo remove <name>`

Remove a registered bare repository.
Refuses if any worktrees remain.

### `timber repo rename <old-name> <new-name>`

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

The `t` wrapper changes to the new path if the current directory is inside a moved worktree.
The command refuses the rename if the destination repository or a destination worktree path exists.

Example:

```bash
timber repo rename timber git-worktree
```

### `timber create [name[@repo]]`

Create a managed worktree for a branch.

- Qualify the name as `<worktree>@<repo>` to select the repository; `@<repo>` alone generates a random name in that repository
- If the name is omitted, generates a random `<adjective>-<noun>` name themed around SpaceX, Starlink, and Tesla
- If the branch already exists, the worktree is created from that branch
- If the branch does not exist, it is created from the branch pointed at by `origin/HEAD`, or if that is unset from `origin/master` then `origin/main`; set it explicitly with `--upstream` | `-u`
- When run inside [Herdr](https://herdr.dev) (`HERDR_ENV=1`), automatically open the new worktree in a standard Herdr space
- The space contains an `Agent` tab that runs `pi` in a pane named after the branch and a `Shell` tab
- Use `--herdr` to open the space explicitly, or `--no-herdr` to suppress automatic creation
- Opening a Herdr space through `t create` implies `--no-cd`
- Opening a Herdr space requires `herdr` on `PATH` and a running Herdr server

Example:

```bash
timber create feature/login@timber
timber create @timber
timber create -u origin/v1.2 hotfix/1.2.1
timber create --herdr feature/login@timber
```

### `timber tui`

Interactively create a managed worktree or open a Herdr space for an existing one.

- Existing worktrees are displayed as `<worktree>@<repo>`
- Type to filter worktree names; after typing `@`, the text after it filters repository names
- With `@` present, matching existing worktrees are labeled **open existing worktree** and other repositories are labeled **create new worktree**
- Use the arrow keys to select an existing worktree or a new-worktree option
- Press Enter on an existing worktree to open a new Herdr space
- Press Enter on a new-worktree option, or an unmatched `<worktree>@<repo>`, to create it
- New worktree names must include a registered repository as `<worktree>@<repo>`
- Never generates a random name
- Requires an interactive terminal
- Use `--no-title` to hide the "Open or Create Worktree" header
- When creating inside [Herdr](https://herdr.dev) (`HERDR_ENV=1`), opens a new standard Herdr space unless `--no-herdr` is set
- Use `--herdr` to open the space explicitly after create

Example:

```bash
timber tui
timber tui --herdr
timber tui --no-title
```

### `timber herdr install`

Install the bundled [Herdr](https://herdr.dev) plugin into `~/.config/herdr/plugins/timber` and register it with `herdr plugin link --enabled`.
Prints a keybinding snippet for `~/.config/herdr/config.toml`.

The plugin files are embedded in the `timber` binary, so this works without a source checkout.
Requires `herdr` on `PATH`.

Example:

```bash
timber herdr install
```

### `timber herdr space [name[@repo]]`

Set up a standard [Herdr](https://herdr.dev) space for a managed worktree.
By default the command defines the tabs in the current Herdr workspace.
It renames the current tab to `Agent` and adds a `Shell` tab.
Use `-n` | `--new` to open a new Herdr workspace instead.

The workspace contains two tabs:

- `Agent`: runs `pi` in the worktree in a pane named after the branch
- `Shell`: opens an interactive shell in the worktree

To show the branch name in the expanded sidebar, customize Herdr's
[Sidebar row layouts](https://herdr.dev/docs/configuration/#sidebar-row-layouts)
and use `pane` in `ui.sidebar.agents.rows`:

```toml
[ui.sidebar.agents]
rows = [
  ["state_icon", "workspace"],
  ["pane", "tab"],
]
```

If `name` is omitted, the command uses the managed worktree that contains the current directory.
Qualify the name as `<worktree>@<repo>` to select a worktree in another repository.
The command requires `herdr` on `PATH` and a running Herdr server.

Example:

```bash
timber herdr space feature/login@timber
timber herdr space --new feature/login@timber
cd ~/worktrees/timber/feature/login/timber
timber herdr space
```

### `timber list [@repo]`

List managed worktrees in a table.

- Default: list worktrees from every registered repository
- `@<repo>`: list only the named repository

Columns:

- `Name`: branch / worktree name
- `Repo`: registered repository name
- `Status`: aligned ahead (`↑`, green) and behind (`↓`, blue) counts, followed by the upstream branch
- `Commit`: short commit hash
- `Dirty`: whether the worktree has uncommitted changes (`true` is highlighted in yellow)

### `timber migrate`

Register a clone as a bare repo, or rehome existing worktrees into the managed layout.

- Inside an unregistered clone: creates `$XDG_DATA_HOME/timber/repos/<name>.git` (override name with `--name`)
- Moves every branched worktree (including the former main checkout) to `$TIMBER_WORKTREE_ROOT/<repo-name>/<branch>/<repo-name>` (fallback: `~/worktrees/...`)
- Inside a registered worktree: moves that repository’s worktrees that are not already at the managed path
- `--all` | `-a`: rehomes worktrees for every registered repository
- Removes empty parent directories of the old checkout, up to `$HOME`
- If the clone has no linked worktrees and HEAD is the default branch (`origin/HEAD`, else `origin/master` / `origin/main`), only the bare repo is registered (no managed worktree is created)
- Does not create worktrees for local branches that do not already have one
- Use `--prompt` | `-p` to choose which worktrees to migrate

Example:

```bash
cd ~/src/github.com/nnutter/timber
timber migrate
timber migrate --name timber --prompt
cd ~/worktrees/next/timber
timber migrate
timber migrate --all
```

### `timber prune [@repo]`

Remove managed worktrees that are both clean and merged into their upstream branch.

Without `@<repo>`, prune considers every registered repository.
Use `--prompt` | `-p` to choose which worktrees to prune interactively.
Use `-n` | `--dry-run` to list the worktrees that would be pruned without removing them.

### `timber remove [name[@repo]]`

Remove a managed worktree and delete its branch.

When `name` is omitted, removes the managed worktree that contains the current directory (auto-detects the registered repo from cwd, or use `<worktree>@<repo>` / the repo picker).
A unique worktree name is enough from outside a managed worktree.
Refuses dirty or unmerged worktrees by default.
Use `--force` | `-f` to force (destructive) removal.

When invoked through the shell wrapper (`t remove`), the shell also `cd`s to `$HOME` after a successful removal.

Example:

```bash
timber remove
timber remove feature/login@timber
timber remove --force feature/login@timber
```

### `timber generate zsh`

Generate a zsh wrapper function, completion, and autoload helper (see [Shell integration](#shell-integration)).

## Typical Flow

```bash
# once: install wrapper
timber generate zsh

# register a repo
t repo add nnutter/timber

# day to day
t create feature/login@timber
t switch feature/login@timber
# ... work ...
t switch main@timber   # if you created a main worktree
t prune @timber
# or:
t remove feature/login
```
