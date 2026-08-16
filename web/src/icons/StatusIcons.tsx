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
