import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { shouldAllowNativeContextMenu } from "./contextMenuPolicy";
import { applyTheme, getStoredTheme, watchSystemMode } from "./theme";
import "./style.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>
);

// index.html's inline snippet already stamped data-theme/data-mode before
// first paint (that's the whole point of it). Re-applying once here is
// still needed for one thing the snippet can't do: syncing
// <meta name="theme-color"> to the palette, which reads a computed custom
// property and therefore has to run after the stylesheet has loaded.
// Everything else about this call is a no-op re-setting the same values.
applyTheme(getStoredTheme());

// Keeps the "System" mode choice actually live — without this, picking
// System would resolve once at load and never follow the OS again until
// reload. A no-op unless the stored mode is "system": applyTheme
// re-resolves from matchMedia each time, so an explicit dark/light choice
// re-applies to the value it already had.
watchSystemMode(() => applyTheme(getStoredTheme()));

// Registering a service worker (even a no-op one) is what makes Chrome/Edge
// treat this as an installable PWA — see public/sw.js.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js");
  });
}

// This is meant to feel like a native app, not a page — the browser's own
// right-click context menu (with its "Reload"/"Inspect"/"View Page Source"
// entries) doesn't belong here on the app's own chrome (sidebar, buttons,
// dialogs, ...). App-level, not React-owned, since it's a permanent
// document-wide behavior override rather than UI state. See
// contextMenuPolicy.ts for which surfaces are exempted and why.
document.addEventListener("contextmenu", (e) => {
  if (shouldAllowNativeContextMenu(e.target)) return;
  e.preventDefault();
});
