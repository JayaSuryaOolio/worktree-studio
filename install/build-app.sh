#!/usr/bin/env bash
# Builds the "Worktree Studio.app" launcher bundle (macOS). Output: ./build/Worktree Studio.app
set -euo pipefail
cd "$(dirname "$0")/.."

(cd web && bun install --frozen-lockfile && bun run build)

APP="build/Worktree Studio.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
# The bundle is a thin launcher: the real (desktop-tagged) binary is the
# one install.sh puts in ~/.worktree-studio/bin. Double-click = start the
# backend if needed, then open the window.
cat > "$APP/Contents/MacOS/worktree-studio" <<'L'
#!/bin/sh
export PATH="/opt/homebrew/bin:/usr/local/bin:$HOME/.local/bin:$PATH"
W="$HOME/.worktree-studio/bin/worktree-studio"
"$W" start >/dev/null 2>&1
exec "$W" desktop-run
L
chmod +x "$APP/Contents/MacOS/worktree-studio"

# Icon: rasterise web/public/icon.svg via Quick Look; skipped if that fails.
tmp=$(mktemp -d)
if qlmanage -t -s 1024 -o "$tmp" web/public/icon.svg >/dev/null 2>&1 && [ -f "$tmp/icon.svg.png" ]; then
  set="$tmp/icon.iconset"; mkdir "$set"
  for s in 16 32 128 256 512; do
    sips -z $s $s "$tmp/icon.svg.png" --out "$set/icon_${s}x${s}.png" >/dev/null
    sips -z $((s*2)) $((s*2)) "$tmp/icon.svg.png" --out "$set/icon_${s}x${s}@2x.png" >/dev/null
  done
  iconutil -c icns "$set" -o "$APP/Contents/Resources/icon.icns"
fi
rm -rf "$tmp"

cat > "$APP/Contents/Info.plist" <<'P'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Worktree Studio</string>
  <key>CFBundleDisplayName</key><string>Worktree Studio</string>
  <key>CFBundleIdentifier</key><string>com.worktree-studio.desktop</string>
  <key>CFBundleExecutable</key><string>worktree-studio</string>
  <key>CFBundleIconFile</key><string>icon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
</dict></plist>
P

echo "Built $APP"
