import { describe, expect, it } from "vitest";
import { resolveLanguage } from "./CodeMirrorEditor";

describe("resolveLanguage", () => {
  it("resolves a Tiltfile (no extension) to Python highlighting", async () => {
    const ext = await resolveLanguage("/repo/Tiltfile");
    expect(ext).not.toBeNull();
  });

  it("still resolves a real .py file to Python highlighting", async () => {
    const ext = await resolveLanguage("/repo/main.py");
    expect(ext).not.toBeNull();
  });

  it("returns null for a filename with no known language", async () => {
    const ext = await resolveLanguage("/repo/some-file.unknownext");
    expect(ext).toBeNull();
  });
});
