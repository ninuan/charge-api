import { describe, expect, it } from "vitest";
import { hasTrafficData } from "./traffic";

describe("hasTrafficData", () => {
  it("returns false when every traffic category is zero", () => {
    expect(hasTrafficData(0, 0, 0)).toBe(false);
  });

  it("returns true when at least one category has requests", () => {
    expect(hasTrafficData(3, 0, 0)).toBe(true);
  });
});
