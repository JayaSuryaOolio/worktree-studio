// FileTree.tsx's header icons — same minimal-glyph, currentColor-stroke
// convention as SplitIcons.tsx.

// A classic three-node git-branch glyph: a main line with a branch curving
// off partway down — used as the "filter to files changed in this PR"
// toggle.
export function GitBranchIcon({ size = 14 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      <circle cx="4" cy="3" r="1.5" />
      <circle cx="4" cy="13" r="1.5" />
      <circle cx="12" cy="7.5" r="1.5" />
      <line x1="4" y1="4.5" x2="4" y2="11.5" />
      <path d="M4 7.5 C 6.5 7.5, 8 7.5, 10.5 7.5" />
    </svg>
  );
}

// Two chevrons converging toward the middle — reads as "collapse
// everything inward," replacing the plain "⊟" glyph this had before.
export function CollapseAllIcon({ size = 14 }: { size?: number }) {
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
      <path d="M4 3 L8 7 L12 3" />
      <path d="M4 13 L8 9 L12 13" />
    </svg>
  );
}
