import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// worktree-studio's Vite dev config: proxies /api to the Go server so the
// dev server and the API server can run side by side (see
// docs/running-locally.md). `npm run build` outputs to dist/, which
// web/embed.go go:embeds into the production Go binary.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8787",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
