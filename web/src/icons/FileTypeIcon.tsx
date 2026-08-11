import { getFileIconKind } from "../fileIconKind";

interface IconProps {
  size?: number;
}

// A small colored badge (rounded square + short text) — used for every
// file-type icon here except the React-flavored ones and the generic
// fallback. Conventional colors (Go's blue, JS's yellow, TS's blue, etc.)
// rather than exact reproductions of any project's official logo artwork.
function Badge({ size = 14, bg, fg, text, fontSize = 7 }: IconProps & { bg: string; fg: string; text: string; fontSize?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <rect x="0.5" y="0.5" width="15" height="15" rx="3" fill={bg} />
      <text
        x="8"
        y="11.25"
        textAnchor="middle"
        fontSize={fontSize}
        fontFamily="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
        fontWeight="700"
        fill={fg}
      >
        {text}
      </text>
    </svg>
  );
}

// The conventional "React atom" mark (three rotated ellipses + a center
// dot) — used for .jsx/.tsx, colored by which language flavor it is,
// since a plain JS/TS square badge doesn't distinguish "this file has
// JSX in it" the way file-icon themes conventionally do.
function ReactAtomIcon({ size = 14, color }: IconProps & { color: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="2.2" fill={color} />
      <g stroke={color} strokeWidth="1.3" fill="none">
        <ellipse cx="12" cy="12" rx="10" ry="4.2" />
        <ellipse cx="12" cy="12" rx="10" ry="4.2" transform="rotate(60 12 12)" />
        <ellipse cx="12" cy="12" rx="10" ry="4.2" transform="rotate(120 12 12)" />
      </g>
    </svg>
  );
}

// The fallback for anything without a more specific icon above — a plain
// dog-eared page outline, styled from the app's own text-dim token rather
// than a fixed color so it matches whichever theme is active.
function GenericFileIcon({ size = 14 }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <path
        d="M3.5 1h5l3.5 3.5V14.5a.5.5 0 0 1-.5.5h-8a.5.5 0 0 1-.5-.5v-13a.5.5 0 0 1 .5-.5Z"
        fill="none"
        style={{ stroke: "var(--text-dim)" }}
        strokeWidth="1"
      />
      <path d="M8.5 1v3.2a.3.3 0 0 0 .3.3H12" fill="none" style={{ stroke: "var(--text-dim)" }} strokeWidth="1" />
    </svg>
  );
}

/** Small per-file-type icon for the file tree — see fileIconKind.ts for the name->kind mapping. */
export default function FileTypeIcon({ name, size = 14 }: { name: string; size?: number }) {
  switch (getFileIconKind(name)) {
    case "go":
      return <Badge size={size} bg="#00ADD8" fg="#ffffff" text="Go" />;
    case "md":
      return <Badge size={size} bg="#1a1a1a" fg="#ffffff" text="M↓" />;
    case "js":
      return <Badge size={size} bg="#f0db4f" fg="#1a1a1a" text="JS" />;
    case "ts":
      return <Badge size={size} bg="#3178c6" fg="#ffffff" text="TS" />;
    case "html":
      return <Badge size={size} bg="#e34f26" fg="#ffffff" text="<>" />;
    case "css":
      return <Badge size={size} bg="#2965f1" fg="#ffffff" text="#" fontSize={9} />;
    case "gitignore":
      return <Badge size={size} bg="#6b7684" fg="#ffffff" text="⊘" fontSize={9} />;
    case "jsx":
      return <ReactAtomIcon size={size} color="#f0db4f" />;
    case "tsx":
      return <ReactAtomIcon size={size} color="#3178c6" />;
    default:
      return <GenericFileIcon size={size} />;
  }
}
