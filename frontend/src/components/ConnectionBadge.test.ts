import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("ConnectionBadge styles", () => {
  it("uses the same sizing rhythm as shell action buttons", () => {
    const css = readFileSync(resolve(__dirname, "../assets/index.css"), "utf8");
    const match = css.match(/\.connection-badge\s*\{\s*@apply\s+([^;]+);/);

    expect(match?.[1]).toContain("h-11");
    expect(match?.[1]).toContain("px-3.5");
    expect(match?.[1]).toContain("text-sm");
    expect(match?.[1]).toContain("rounded-lg");
    expect(match?.[1]).toContain("bg-background");
  });
});
