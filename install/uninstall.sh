#!/usr/bin/env bash
# Reverses install.sh: removes this app's hooks (every Claude Code hook
# registered in internal/claudehook's registry + the globally-installed
# worktree-studio skill) and removes the installed prod build
# (~/.worktree-studio/bin/worktree-studio) plus the wtx harness installed
# beside it (~/.worktree-studio/bin/wtx).
#
# Deliberately does NOT touch ~/.worktree-studio/studio.db, worktrees/,
# audit.log.jsonl, server.log, or backups/ — those are this app's actual
# data (registered repos, worktrees, audit history), not build output, and
# removing them isn't what "uninstall the prod build" asked for. Remove
# ~/.worktree-studio entirely by hand if you want a full wipe.
set -euo pipefail

PREFIX="worktree-studio__uninstall: "
log() { printf '%s%s\n' "$PREFIX" "$1"; }

INSTALL_DIR="$HOME/.worktree-studio/bin"
BIN_PATH="$INSTALL_DIR/worktree-studio"
WTX_PATH="$INSTALL_DIR/wtx"

if [ -x "$BIN_PATH" ]; then
  log "removing hooks (every registered claude hook + worktree-studio skill)"
  "$BIN_PATH" uninstall-hooks
else
  log "no installed binary found at $BIN_PATH — skipping hook removal"
  log "(if the hooks were installed some other way, remove them from the settings modal instead)"
fi

if [ -e "$BIN_PATH" ]; then
  log "removing installed binary: $BIN_PATH"
  rm -f "$BIN_PATH"
fi

if [ -e "$WTX_PATH" ]; then
  log "removing installed wtx harness: $WTX_PATH"
  rm -f "$WTX_PATH"
fi

if [ -d "$INSTALL_DIR" ] && [ -z "$(ls -A "$INSTALL_DIR")" ]; then
  rmdir "$INSTALL_DIR"
fi

log "done. Your registered repos/worktrees/audit log under ~/.worktree-studio were left untouched."
