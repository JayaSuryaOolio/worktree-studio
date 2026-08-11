import { describe, expect, it } from "vitest";
import { getFileIconKind } from "./fileIconKind";

describe("getFileIconKind", () => {
  it.each([
    ["main.go", "go"],
    ["README.md", "md"],
    ["index.js", "js"],
    ["index.ts", "ts"],
    ["App.tsx", "tsx"],
    ["App.jsx", "jsx"],
    ["index.html", "html"],
    ["style.css", "css"],
  ] as const)("maps %s to %s", (name, kind) => {
    expect(getFileIconKind(name)).toBe(kind);
  });

  it("maps .gitignore specifically, not by extension", () => {
    expect(getFileIconKind(".gitignore")).toBe("gitignore");
  });

  it("is case-insensitive on the extension", () => {
    expect(getFileIconKind("Main.GO")).toBe("go");
    expect(getFileIconKind("Notes.MD")).toBe("md");
  });

  it("falls back to generic for an unknown extension", () => {
    expect(getFileIconKind("archive.zip")).toBe("generic");
    expect(getFileIconKind("data.json")).toBe("generic");
  });

  it("falls back to generic for a file with no extension", () => {
    expect(getFileIconKind("Makefile")).toBe("generic");
    expect(getFileIconKind("LICENSE")).toBe("generic");
  });

  it("falls back to generic for a dotfile that isn't .gitignore", () => {
    expect(getFileIconKind(".env")).toBe("generic");
  });
});
