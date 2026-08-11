// Detects a well-known interactive app running inside a terminal pane from
// the window title it sets via an OSC 0/2 escape sequence — xterm.js's
// onTitleChange fires whenever one arrives over the pty stream, the same
// mechanism a real terminal emulator uses to show "vim: file.txt" or
// "ssh: host" in a tab instead of a generic shell label.
//
// Confirmed empirically (piped a real `claude` process through a pty and
// captured its raw output) that `claude` sets its title via OSC 0 to
// "<status glyph> Claude Code" — the leading glyph is a status indicator
// that can change, so matching on the stable "Claude Code" substring is
// what survives that rather than an exact match.
//
// This is deliberately a small registry, not a single hardcoded check:
// the plan is to cover more "persisting" interactive apps the same way
// later (this is where a new entry goes), while WorktreeDetail.tsx keeps
// the actual icon components out of this framework-free module.
export type TerminalAppKind = "claude";

interface TerminalAppSignature {
  kind: TerminalAppKind;
  label: string;
  matches: (title: string) => boolean;
}

const SIGNATURES: TerminalAppSignature[] = [
  {
    kind: "claude",
    label: "Claude",
    matches: (title) => title.includes("Claude Code"),
  },
];

export interface DetectedTerminalApp {
  kind: TerminalAppKind;
  label: string;
}

export function detectTerminalApp(title: string): DetectedTerminalApp | null {
  const signature = SIGNATURES.find((s) => s.matches(title));
  return signature ? { kind: signature.kind, label: signature.label } : null;
}
