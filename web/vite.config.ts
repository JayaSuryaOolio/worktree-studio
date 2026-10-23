import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// worktree-studio's Vite dev config: proxies /api to the Go server so the
// dev server and the API server can run side by side (see
// docs/running-locally.md). `bun run build` outputs to dist/, which
// web/embed.go go:embeds into the production Go binary.
// WORKTREE_STUDIO_ADDR (e.g. localhost:8788, set by ./wts dev) picks the API server.
const api = process.env.WORKTREE_STUDIO_ADDR?.replace(/^:/, "localhost:") ?? "localhost:8787";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: `http://${api}`,
        changeOrigin: true,
      },
      "/ws": {
        target: `ws://${api}`,
        ws: true,
      },
    },
  },
  build: {
    outDir: "dist",
    // NOT emptyOutDir: true — that would delete the checked-in
    // web/dist/.gitkeep placeholder on every build (main.go's
    // mountFrontend/go:embed comments rely on web/dist existing, with or
    // without a real build, so `go build ./...` works on a fresh checkout
    // before the frontend is ever built). .gitignore's `web/dist/*` +
    // `!web/dist/.gitkeep` already keeps built assets out of git, so Vite
    // doesn't need to clean the directory itself.
    emptyOutDir: false,
  },
});
