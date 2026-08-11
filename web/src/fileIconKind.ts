// Which icon FileTypeIcon.tsx should render for a given file name — kept
// separate from that component (same split as terminalAppDetection.ts vs.
// its icons) so the actual matching rule is plain, framework-free logic
// that's trivial to unit test.
export type FileIconKind = "go" | "md" | "js" | "ts" | "tsx" | "jsx" | "html" | "css" | "gitignore" | "generic";

const EXTENSION_KINDS: Record<string, FileIconKind> = {
  go: "go",
  md: "md",
  js: "js",
  ts: "ts",
  tsx: "tsx",
  jsx: "jsx",
  html: "html",
  css: "css",
};

export function getFileIconKind(fileName: string): FileIconKind {
  if (fileName === ".gitignore") return "gitignore";

  const dot = fileName.lastIndexOf(".");
  if (dot <= 0) return "generic"; // no extension, or a dotfile with nothing after the dot

  const ext = fileName.slice(dot + 1).toLowerCase();
  return EXTENSION_KINDS[ext] ?? "generic";
}
