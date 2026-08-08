// Minimal service worker — its only job is to exist with a fetch handler,
// which is what makes Chrome/Edge consider this installable as a PWA.
// worktree-studio is a local-only tool talking to its own Go server, so
// there's no offline caching story worth building; every request just
// passes straight through to the network.
self.addEventListener("fetch", () => {});
