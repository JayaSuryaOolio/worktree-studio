#!/usr/bin/env bash
# Builds worktree-studio (frontend + Go binary), installs the built binary
# to ~/.worktree-studio/bin/worktree-studio, and installs this app's
# "hooks" (every Claude Code hook registered in internal/claudehook's
# registry — currently session-tracking and worktree-context — plus the
# globally-installed worktree-studio skill) — see cmd/worktree-studio/hooks.go
# for what "install-hooks" actually does; this script is just the thin,
# repeatable wrapper around it that install/uninstall.sh reverses.
# Detects the OS + package manager and offers to install missing deps
# (git, tmux, go, bun); set WS_YES=1 to skip the prompt.
set -euo pipefail

PREFIX="worktree-studio__install: "
log() { printf '%s%s\n' "$PREFIX" "$1"; }
fail() { printf '%s%s\n' "$PREFIX" "$1" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALL_DIR="$HOME/.worktree-studio/bin"
BIN_PATH="$INSTALL_DIR/worktree-studio"

# --- OS + package manager detection -----------------------------------------
case "$(uname -s)" in
  Darwin) OS=macos ;;
  Linux)  OS=linux ;;
  *) fail "unsupported OS '$(uname -s)' — worktree-studio needs macOS or Linux (use WSL on Windows)" ;;
esac

PM=""
for pm in brew apt-get dnf pacman apk; do
  command -v "$pm" >/dev/null 2>&1 && { PM="$pm"; break; }
done

SUDO=""
[ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1 && SUDO="sudo"

# pkg_install <pkg>: install via the detected package manager. Package names
# are identical across managers for everything we need except go (golang).
pkg_install() {
  case "$PM" in
    brew)    brew install "$1" ;;
    apt-get) $SUDO apt-get install -y "$1" ;;
    dnf)     $SUDO dnf install -y "$1" ;;
    pacman)  $SUDO pacman -S --noconfirm "$1" ;;
    apk)     $SUDO apk add "$1" ;;
    *)       return 1 ;;
  esac
}

# need <cmd> <pkg> <manual-hint>: ensure <cmd> exists, offering to install it.
need() {
  command -v "$1" >/dev/null 2>&1 && return 0
  [ -n "$PM" ] && [ -n "$2" ] || fail "$1 not found — $3"
  if [ -t 0 ] && [ "${WS_YES:-}" != 1 ]; then
    printf '%s%s not found. Install it with %s? [y/N] ' "$PREFIX" "$1" "$PM"
    read -r ans; [ "$ans" = y ] || [ "$ans" = Y ] || fail "$1 is required — $3"
  fi
  log "installing $1 via $PM"
  pkg_install "$2" || fail "could not install $1 — $3"
}

log "detected $OS (package manager: ${PM:-none})"

GO_PKG=go; [ "$PM" != brew ] && [ "$PM" != pacman ] && GO_PKG=golang
need git git "install git"
need tmux tmux "install tmux (terminal tabs are tmux sessions)"
need go "$GO_PKG" "install Go from https://go.dev/dl/"
if ! command -v bun >/dev/null 2>&1; then
  log "bun not found — installing via https://bun.sh/install"
  command -v curl >/dev/null 2>&1 || fail "curl is needed to install bun — install curl or bun by hand"
  curl -fsSL https://bun.sh/install | bash
  export PATH="$HOME/.bun/bin:$PATH"
  command -v bun >/dev/null 2>&1 || fail "bun install failed — see https://bun.sh"
fi

# Optional: spotlight sync only. Warn, don't fail.
command -v fswatch >/dev/null 2>&1 || log "note: fswatch not found (optional, needed for spotlight sync) — install it with: ${PM:-your package manager} install fswatch"
command -v spotlight >/dev/null 2>&1 || [ -x "$HOME/.local/bin/spotlight" ] || log "note: spotlight CLI not found (optional) — see docs/spotlight-sync.md"

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
