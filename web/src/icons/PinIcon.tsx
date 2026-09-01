// A thumbtack — round head, angled point — for the sidebar's "pin this
// worktree" toggle and its small always-visible indicator on a pinned
// row. Same minimal-glyph, currentColor-stroke convention as
// StatusIcons.tsx/FileTreeIcons.tsx: built from a circle and a line
// rather than a hand-traced compound path, so it stays legible (and easy
// to get right without a browser to check it in) at 12-14px.
//
// Unlike SpotlightIcon (identical glyph in both states, distinguished
// only by a CSS class on the button), the head fills solid when pinned —
// an outline-only pin at this size reads as "just an icon," not
// "currently on." The `filled` prop drives that; the point (a <line>,
// which has no fill area regardless) is unaffected either way.
export default function PinIcon({ size = 14, filled = false }: { size?: number; filled?: boolean }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth="1.3"
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      <circle cx="9.5" cy="6" r="3.2" />
      <line x1="7.2" y1="8.2" x2="2.5" y2="13.5" />
    </svg>
  );
}
