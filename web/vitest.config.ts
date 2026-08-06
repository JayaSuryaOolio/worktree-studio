import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Separate from vite.config.ts (which is production-build/dev-server
// config) to keep that file's concerns clean. Component tests here exist
// specifically to catch real interaction bugs (event propagation into a
// parent <Link> causing a full-page navigation, a modal not actually
// rendering its content) by executing the real components against jsdom,
// rather than reasoning about them statically — the practical substitute
// for a browser in an environment with no browser-automation tool
// available. jsdom does NOT compute layout/paint, so it can't catch a
// pure-CSS invisibility bug — it catches whether the right DOM exists and
// whether the right event handlers actually ran.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    globals: true,
    // Reset vi.fn() mocks between every test automatically — without this,
    // call counts/implementations leak across tests within the same file
    // (bit us once already: a call-count assertion failed because an
    // earlier test's calls were still counted).
    clearMocks: true,
  },
});
