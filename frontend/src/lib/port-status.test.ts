import { describe, expect, it } from "vitest";
import { getPortPresentation } from "./port-status";

describe("getPortPresentation", () => {
  it("returns an explicit charging presentation", () => {
    expect(getPortPresentation("in_use")).toEqual({
      label: "充电中",
      tone: "charging",
      icon: "Zap"
    });
  });

  it("does not rely on color alone for offline ports", () => {
    expect(getPortPresentation("offline")).toEqual({
      label: "离线",
      tone: "offline",
      icon: "WifiOff"
    });
  });
});
