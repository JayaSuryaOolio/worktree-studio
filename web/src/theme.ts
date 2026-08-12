// Theme is an explicit, stored user choice — not something that follows
// the OS's prefers-color-scheme — set from SettingsModal.tsx's Appearance
// tab. Dark is the default (styles/tokens.css's bare :root block), which
// is why "dark" never needs a data-theme attribute at all; only "light"
// does, via the :root[data-theme="light"] override in that same file.
export type Theme = "dark" | "light";

const STORAGE_KEY = "worktree-studio-theme";

export function getStoredTheme(): Theme {
  return localStorage.getItem(STORAGE_KEY) === "light" ? "light" : "dark";
}

// Applies theme to the document without persisting it — used by the
// inline snippet in index.html (see its own comment) to avoid a flash of
// the wrong theme before React/main.tsx ever run, and by setTheme below.
export function applyTheme(theme: Theme): void {
  if (theme === "light") {
    document.documentElement.setAttribute("data-theme", "light");
  } else {
    document.documentElement.removeAttribute("data-theme");
  }
}

export function setTheme(theme: Theme): void {
  localStorage.setItem(STORAGE_KEY, theme);
  applyTheme(theme);
}
