// A small hand-built SVG for the "Open in VS Code" toolbar button —
// deliberately not pulled from an icon library (per direct request), just
// a plain inline <svg>. Not a pixel-exact reproduction of the official VS
// Code logo, just a recognizable rounded badge in VS Code's signature blue
// with a code-bracket glyph, sized to sit inline with the emoji used by
// every other toolbar button.
export default function VSCodeIcon({ size = 16 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      <rect width="16" height="16" rx="3.5" fill="#0065A9" />
      <path
        d="M5.7 4.2 3.3 6.6a.6.6 0 0 0 0 .85L5.7 9.8"
        stroke="#ffffff"
        strokeWidth="1.15"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />
      <path
        d="M10.3 4.2 12.7 6.6a.6.6 0 0 1 0 .85L10.3 9.8"
        stroke="#ffffff"
        strokeWidth="1.15"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />
      <path
        d="M9 3.4 7 12.6"
        stroke="#ffffff"
        strokeWidth="1.15"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />
    </svg>
  );
}
