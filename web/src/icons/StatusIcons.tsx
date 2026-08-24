// Sidebar worktree-row status icons — same minimal-glyph, currentColor-
// stroke convention as FileTreeIcons.tsx/SplitIcons.tsx.

// A table-lamp/spotlight glyph — a shade casting a cone of light down onto
// a base — for the "spotlight sync is active" indicator in Sidebar.tsx.
// Replaces the plain dot that used to mark this (see .sidebar-dot-
// attention in sidebar.css for where that dot look moved to instead: the
// "claude needs your input" badge, not spotlight).
export function SpotlightIcon({ size = 14 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      {/* lamp shade */}
      <path d="M5 2.5 H11 L13 7 H3 Z" />
      {/* neck */}
      <line x1="8" y1="7" x2="8" y2="9.5" />
      {/* base */}
      <line x1="5" y1="12.5" x2="11" y2="12.5" />
      <line x1="8" y1="9.5" x2="8" y2="12.5" />
      {/* light cone */}
      <path d="M4.5 12.5 L2 15.5 M11.5 12.5 L14 15.5" strokeDasharray="1.6 1.6" />
    </svg>
  );
}

// A single down-chevron, rotated 180deg via CSS when expanded (see
// .sidebar-worktree-expand-toggle in sidebar.css) — the sidebar's
// worktree-card expand/collapse affordance.
export function ChevronIcon({ size = 12 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M4 6 L8 10 L12 6" />
    </svg>
  );
}

// Settings. Replaced a plain "⚙" text glyph, which rendered at the
// button's font-size — a ~14px character floating in a 26px box, so it
// read as small and slightly off-centre no matter how the button was
// sized. An SVG scales with the button instead of with the type scale.
//
// Drawn in the same convention as the icons above: 16 viewBox,
// currentColor stroke, no fill. A ring plus eight radial teeth and a hub,
// rather than a single traced gear outline — at 16px an outline gear
// turns to mush, and this stays legible down to 12.
export function GearIcon({ size = 16 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      <circle cx="8" cy="8" r="4.55" />
      <circle cx="8" cy="8" r="1.5" />
      <path d="M12.55 8.00 L14.35 8.00 M11.22 11.22 L12.49 12.49 M8.00 12.55 L8.00 14.35 M4.78 11.22 L3.51 12.49 M3.45 8.00 L1.65 8.00 M4.78 4.78 L3.51 3.51 M8.00 3.45 L8.00 1.65 M11.22 4.78 L12.49 3.51" />
    </svg>
  );
}

// "Add" — for the sidebar's add-repo and new-worktree buttons, which were
// also bare text glyphs sized by the type scale rather than by their
// button.
export function PlusIcon({ size = 16 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M8 3.5 V12.5 M3.5 8 H12.5" />
    </svg>
  );
}
