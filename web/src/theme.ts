// Theme is two independent, explicitly-stored choices — a palette family
// and a mode — set from SettingsModal.tsx's Appearance tab and applied as
// two attributes on <html> that styles/tokens.css keys off:
//
//   data-theme = "graphite" | "ledger" | "deck"
//   data-mode  = "dark" | "light"          (always resolved, never "system")
//
// Mode can be *stored* as "system", but what reaches the DOM is always a
// concrete dark/light. Keeping the resolution here rather than in a CSS
// prefers-color-scheme query is what lets the stylesheet stay a flat set
// of palette blocks with no third code path, and keeps "follow the OS" an
// explicit choice the user made — the original reasoning for not having a
// media query at all — rather than silent OS-driven behaviour.

export type ThemeFamily = "graphite" | "ledger" | "deck";
export type ThemeMode = "dark" | "light" | "system";
export type ResolvedMode = "dark" | "light";

export interface ThemeChoice {
  family: ThemeFamily;
  mode: ThemeMode;
}

export const THEME_FAMILIES: ThemeFamily[] = ["graphite", "ledger", "deck"];

export const THEME_FAMILY_LABELS: Record<ThemeFamily, string> = {
  graphite: "Graphite",
  ledger: "Ledger",
  deck: "Command Deck",
};

export const THEME_FAMILY_BLURBS: Record<ThemeFamily, string> = {
  graphite: "Modern. Neutral greys, one warm accent, no outlines.",
  ledger: "Classic. Warm paper, square corners, deep editor blue.",
  deck: "The original. Higher contrast, more colour, boxed rows.",
};

const FAMILY_KEY = "worktree-studio-theme-family";
const MODE_KEY = "worktree-studio-theme-mode";
// Pre-redesign key, when the whole theme was just "dark" | "light". Read
// once for migration so an existing install keeps the light/dark it had
// chosen — only the family is new. Never written back to.
const LEGACY_KEY = "worktree-studio-theme";

export const DEFAULT_THEME: ThemeChoice = { family: "graphite", mode: "system" };

function isFamily(v: unknown): v is ThemeFamily {
  return v === "graphite" || v === "ledger" || v === "deck";
}

function isMode(v: unknown): v is ThemeMode {
  return v === "dark" || v === "light" || v === "system";
}

export function getStoredTheme(): ThemeChoice {
  let family: ThemeFamily = DEFAULT_THEME.family;
  let mode: ThemeMode = DEFAULT_THEME.mode;
  try {
    const storedFamily = localStorage.getItem(FAMILY_KEY);
    if (isFamily(storedFamily)) family = storedFamily;

    const storedMode = localStorage.getItem(MODE_KEY);
    if (isMode(storedMode)) {
      mode = storedMode;
    } else {
      const legacy = localStorage.getItem(LEGACY_KEY);
      if (legacy === "light" || legacy === "dark") mode = legacy;
    }
  } catch {
    // Private-mode / disabled storage: fall through to the defaults
    // rather than leaving the app unthemed.
  }
  return { family, mode };
}

export function systemMode(): ResolvedMode {
  return typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: light)").matches
    ? "light"
    : "dark";
}

export function resolveMode(mode: ThemeMode): ResolvedMode {
  return mode === "system" ? systemMode() : mode;
}

// Applies without persisting — used by the inline pre-paint snippet's
// React-side counterpart (see index.html) and by setTheme below.
export function applyTheme(choice: ThemeChoice): void {
  const root = document.documentElement;
  root.setAttribute("data-theme", choice.family);
  root.setAttribute("data-mode", resolveMode(choice.mode));
  syncThemeColor();
}

// Keeps <meta name="theme-color"> on the current palette.
//
// Installed as a PWA (which is how this is meant to be used — see
// public/manifest.webmanifest), the browser paints its own window title
// bar with this colour. A static value meant that bar stayed one theme's
// surface no matter which theme was selected, leaving a mismatched band
// across the top of the window in every theme but one.
//
// Read from the computed style rather than a duplicated table of hexes,
// so it can't drift from tokens.css. Attribute changes above are already
// reflected in getComputedStyle by the time this runs.
function syncThemeColor(): void {
  const meta = document.querySelector('meta[name="theme-color"]');
  if (!meta) return;
  const surface = getComputedStyle(document.documentElement).getPropertyValue("--surface-1").trim();
  if (surface) meta.setAttribute("content", surface);
}

export function setTheme(choice: ThemeChoice): void {
  try {
    localStorage.setItem(FAMILY_KEY, choice.family);
    localStorage.setItem(MODE_KEY, choice.mode);
  } catch {
    // Still apply it for this session even if it can't be remembered.
  }
  applyTheme(choice);
}

// Keeps mode: "system" actually live — without this, picking System would
// resolve once at load and then never follow an OS change until reload.
// Returns an unsubscribe function; a no-op where matchMedia is missing.
export function watchSystemMode(onChange: () => void): () => void {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return () => {};
  }
  const mq = window.matchMedia("(prefers-color-scheme: light)");
  const handler = () => onChange();
  // addListener is the deprecated form, kept for older Safari.
  if (typeof mq.addEventListener === "function") {
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }
  mq.addListener(handler);
  return () => mq.removeListener(handler);
}
