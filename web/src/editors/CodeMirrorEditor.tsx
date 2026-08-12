import { useEffect, useRef } from "react";
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import { basicSetup } from "codemirror";
import { LanguageDescription } from "@codemirror/language";
import { languages } from "@codemirror/language-data";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";
import type { EditorProps } from "./EditorAdapter";

// The one CodeMirror-specific adapter for v1 (see docs/editor-plan.md).
// Everything CodeMirror-shaped — its extensions, the vscodeDark theme, the
// language-grammar registry — lives entirely in this file. EditorPanel.tsx
// only ever imports EditorProps/the registry, never anything from here or
// from "codemirror"/"@codemirror/*" directly.
export default function CodeMirrorEditor({ content, path, onChange, onSaveRequested }: EditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    let view: EditorView | null = null;

    async function mount() {
      const langExtension = await resolveLanguage(path);
      if (cancelled || !containerRef.current) return;

      const extensions: Extension[] = [
        basicSetup,
        vscodeDark,
        saveKeymap(onSaveRequested),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChange(update.state.doc.toString());
          }
        }),
      ];
      if (langExtension) extensions.push(langExtension);

      view = new EditorView({
        state: EditorState.create({ doc: content, extensions }),
        parent: containerRef.current,
      });
    }

    mount();

    return () => {
      cancelled = true;
      view?.destroy();
    };
    // Mount-once per instance, deliberately: this component is uncontrolled
    // after mount (see EditorProps.content's doc comment) — the caller
    // remounts it via a changed `key` to reset content, rather than this
    // effect re-running on every `content`/`path` prop change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return <div ref={containerRef} className="codemirror-editor-container" />;
}

function saveKeymap(onSaveRequested: () => void): Extension {
  return keymap.of([
    {
      key: "Mod-s",
      run: () => {
        onSaveRequested();
        return true; // must return true, not just call preventDefault, so
        // CodeMirror itself marks the key handled and the browser's native
        // "Save Page As" never fires — see docs/editor-plan.md pitfall #4.
      },
      preventDefault: true,
    },
  ]);
}

// Filenames @codemirror/language-data's own filename/extension matching
// doesn't recognize on its own, mapped to a filename whose language it
// does — so `LanguageDescription.matchFilename` gets a name it can
// actually match against, rather than teaching this file a second
// language-detection scheme. Tiltfile (Tilt's build-config format,
// Python-like/Starlark syntax, conventionally named exactly "Tiltfile"
// with no extension) is the concrete case that motivated this: it fell
// through as unrecognized (plain text, no highlighting) before this
// existed. Add more filename aliases here as they come up, same as
// terminalAppDetection.ts's small SIGNATURES registry.
const FILENAME_LANGUAGE_ALIASES: Record<string, string> = {
  Tiltfile: "Tiltfile.py",
};

export async function resolveLanguage(path: string): Promise<Extension | null> {
  const filename = path.split("/").pop() ?? path;
  const aliasedFilename = FILENAME_LANGUAGE_ALIASES[filename] ?? filename;
  const desc = LanguageDescription.matchFilename(languages, aliasedFilename);
  if (!desc) return null;
  try {
    return await desc.load();
  } catch {
    return null; // a failed dynamic import of one language's grammar shouldn't break the whole editor
  }
}
