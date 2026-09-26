# wtx — the worktree exec harness

`scripts/wtx` is a thin wrapper around `worktree-studio create-worktree` that
creates (or reuses) a worktree for a task and then launches `claude` with that
worktree as the working directory.

```
wtx <repo> <name> [options] [-- <claude args>...]
```

The point is determinism. A repo's own `CLAUDE.md` and `.claude/` only load when
`claude` starts with that repo as its cwd. Relying on a person — or an agent —
to remember to `cd` first means it sometimes doesn't happen, and the failure is
silent: you get a session with none of the repo's guidance and no indication
that anything is missing. `wtx` makes the `cd` structural. Nothing it can do
ends with `claude` running anywhere except inside the intended worktree.

`<name>` is slugified (by the same rules as `internal/api/util.go`'s `slugify`)
into **both** the branch name and the worktree directory name. One task, one
worktree, one branch.

## Why it wraps the CLI instead of the HTTP API

`worktree-studio create-worktree <name> [--branch <ref>] [path]` already
resolves a repo from a filesystem path and creates the worktree through the
running server, so the result lands in the dashboard and the audit log exactly
like one created from the UI. Wrapping it means `wtx` needs no repo ids, no
JSON handling, and no HTTP client of its own — `git` is its only hard
dependency.

## Base branch: always `origin/<base>`, never a local branch

Every worktree is branched from a remote-tracking ref, and `wtx` resolves that
ref itself and passes it **explicitly** to whichever creation path runs.

Branching off a local `main` inherits whatever state that checkout is in: `main`
may be well behind `origin`, may be sitting mid-rebase, or may be carrying a
stray local commit. `origin/main` is a fetched ref that we never write to, so
every task started at the same time starts from the same commit. In practice the
gap is not small — at the time of writing, one registered repo's local `main`
was 394 commits behind its `origin/main`.

Passing the ref explicitly is what keeps the two creation paths honest.
`gitops.DetectDefaultBranch` strips the `origin/` prefix and hands git a bare
local branch name, so a repo with no `base_branch` override configured would
otherwise branch from local `main` via the server and from `origin/main` via the
git fallback — the same command producing two different trees depending on
whether a server happened to be running. Resolving once, up front, removes that.

Resolution order:

1. `--base <ref>`, if given.
2. `origin/HEAD`'s target (`git symbolic-ref --short refs/remotes/origin/HEAD`),
   which `git` already renders in `origin/<name>` form.
3. `origin/main`, then `origin/master`.
4. Local `main`/`master` — only when the repo has no remote at all, and it says
   so when it does this.

A bare name passed to `--base` is pinned to its remote-tracking counterpart when
one exists (`--base develop` → `origin/develop`). A value that already contains
a slash, or that has no counterpart on `origin`, is used verbatim — that is how
you branch from a tag, a SHA, or a local-only branch.

`wtx` does not fetch. `origin/main` is only as fresh as your last `git fetch`.

## The fallback

If the worktree can't be created through `worktree-studio`, `wtx` falls back to
plain `git worktree add` under `~/.wtx/worktrees/<repo>/<branch>`, kept out of
worktree-studio's own root so the two never disagree about which worktrees they
own. The fallback always prints a banner saying it happened and why; it is never
silent, because a worktree created this way is invisible to the dashboard and
the audit log and you need to know that.

It falls back when:

- `--force-git` was passed.
- `worktree-studio` isn't on `PATH`.
- The server isn't answering.
- The repo isn't registered with worktree-studio.
- The branch already exists with no worktree attached. worktree-studio can only
  create *new* branches (`git worktree add -b`), so attaching to an existing one
  is necessarily a git operation. `--base` is not applied in this case — the
  branch's own commits are what you asked for.
- **`create-worktree` reported success but git has no worktree for the branch.**
  The exit code alone is not sufficient evidence: `create-worktree` treats any
  2xx it cannot parse as success, and an older server without the
  `/api/worktrees` route answers that request with the SPA's HTML at 200. So
  `wtx` re-asks git after every creation and believes git.

That last check is an instance of the general rule below.

## Reuse is resolved by asking git

"Does a worktree for this branch already exist" is answered with
`git worktree list --porcelain`, not by querying worktree-studio's database.
That single choice is what makes reuse work identically across both creation
paths: a worktree created by the fallback while the server was down is still
found and reused once the server comes back, and a stale or unreachable server
can never cause a duplicate.

## Safety

Before it will launch anything, `wtx` asserts that the resolved worktree exists,
is **not** the main checkout, is the top level of its own working tree, is on the
requested branch, and that the process cwd *after* the `cd` is exactly that path.
Any failure exits non-zero. There is no path through this script that ends with
`claude` running in the main checkout.

A worktree that is dirty **on the requested branch** warns and proceeds — that is
the ordinary case of resuming your own half-finished task, and making it require
a flag would be bad ergonomics for the common path. `--require-clean` turns it
into a hard gate for callers that want one. Dirty on a *different* branch is
always a hard error; that is two tasks contending for one tree.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | ok |
| 2 | usage error, unknown repo, unresolvable `--base` |
| 3 | worktree conflict (occupied path, wrong branch, `--require-clean` on a dirty tree) |
| 4 | creation failed on both paths |
| 5 | a safety assertion tripped |
| 6 | a required dependency is missing |

## The repo allowlist

`wtx` resolves `<repo>` against an allowlist, not a filesystem search. An alias
not on the list is unreachable, which is what stops a typo from resolving to some
unrelated checkout — and what lets a known-bad path (a second checkout of a repo
that is already on the list under another alias) be rejected by name, with an
error that points at the right alias.

The built-in list is the five repos this harness was written for. Override it
with `WTX_REPOS`, a space-separated list of `alias=/absolute/path` pairs:

```bash
WTX_REPOS="api=$HOME/src/api web=$HOME/src/web" wtx api ool-12-thing
```

Relative paths are rejected: an alias has to mean the same directory no matter
where you invoke it from.

## Options

| Option | Effect |
|---|---|
| `--base <ref>` | base to branch from (see above) |
| `--print-path` | print the worktree path to stdout and exit; don't launch |
| `--require-clean` | fail if the reused worktree has uncommitted changes |
| `--force-git` | skip worktree-studio entirely |
| `--host-home` | run `claude` with the real host `HOME` |
| `--list` | print the allowlist |

Everything informational goes to stderr, so `--print-path`'s stdout stays a
single clean path:

```bash
cd "$(wtx backoffice ool-50-thing --print-path)"
```

`--host-home` exists for agent sandboxes that rewrite `HOME` and
`CLAUDE_CONFIG_DIR` to a per-run temp directory. `claude` launched under those
has no credentials and just reports "not logged in". `wtx` detects a
`CLAUDE_CONFIG_DIR` pointing outside your home directory and restores the real
one, announcing it; `--host-home` makes that explicit. For the same reason `wtx`
never derives a durable path from `$HOME` — it asks the system user database
(`dscl`/`getent`) instead, overridable with `WTX_HOST_HOME_DIR`.

## Install

`install/install.sh` puts `wtx` next to the `worktree-studio` binary in
`~/.worktree-studio/bin/`. Add that directory to `PATH`, or symlink `wtx` into a
directory already on it.
