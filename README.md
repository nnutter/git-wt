# git-wt

`git-wt` manages Git worktrees using a consistent naming convention.

Managed worktrees are stored next to the main repository using this path format:

`<repo>.<normalized-branch-name>`

Example:

- repo: `my-repo`
- branch: `feature/login`
- worktree path: `../my-repo.feature.login`

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

### `git-wt remove <name>`

Remove a managed worktree and delete its branch.

It refuses to remove dirty or unmerged worktrees by default.
Use `--force` | `-f` to force (destructive) removal.

Example:

```bash
git-wt remove feature/login
git-wt remove --force feature/login
```

## Typical Flow

Checkout [git-cd](https://github.com/nnutter/dotfiles/blob/master/bin/git-cd) to generate shell functions to easily cd to worktrees.

Create a shell function, `nn`, to switch between the repos under my GitHub path,

```bash
git-cd --name nn --repos ~/src/github.com/nnutter
```

Then a typical flow might look like,

```bash
nn some-repo
git-wt create feature/login
...
nn some-repo.feature.login
nn some-repo
git-wt prune
```
