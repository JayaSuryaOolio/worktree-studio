#!/usr/bin/env bash
# Builds worktree-studio (frontend + Go binary), installs the built binary
# to ~/.worktree-studio/bin/worktree-studio, and installs this app's
# "hooks" (every Claude Code hook registered in internal/claudehook's
# registry — currently session-tracking and worktree-context — plus the
# globally-installed worktree-studio skill) — see cmd/worktree-studio/hooks.go
# for what "install-hooks" actually does; this script is just the thin,
# repeatable wrapper around it that install/uninstall.sh reverses.
set -euo pipefail

PREFIX="worktree-studio__install: "
log() { printf '%s%s\n' "$PREFIX" "$1"; }
fail() { printf '%s%s\n' "$PREFIX" "$1" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALL_DIR="$HOME/.worktree-studio/bin"
BIN_PATH="$INSTALL_DIR/worktree-studio"

command -v go >/dev/null 2>&1 || fail "go not found on PATH — install Go first (https://go.dev/dl/)"
command -v bun >/dev/null 2>&1 || fail "bun not found on PATH — install it first (curl -fsSL https://bun.sh/install | bash)"

log "installing frontend dependencies (bun install)"
( cd "$REPO_ROOT/web" && bun install )

log "building frontend (bun run build)"
( cd "$REPO_ROOT/web" && bun run build )

log "building Go binary -> $BIN_PATH"
mkdir -p "$INSTALL_DIR"
( cd "$REPO_ROOT" && go build -o "$BIN_PATH" ./cmd/worktree-studio )

log "installing hooks (every registered claude hook + worktree-studio skill)"
"$BIN_PATH" install-hooks

log "done. Run the server with: $BIN_PATH"
log "(add $INSTALL_DIR to your PATH to just run: worktree-studio)"
