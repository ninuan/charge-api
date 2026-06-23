import { describe, expect, it } from "vitest";
import { getPilePresentation } from "./pile-status";
import type { Pile } from "@/types/dashboard";

function pile(overrides: Partial<Pile> = {}): Pile {
  return {
    id: "device-1",
    number: "61034278",
    name: "测试充电桩",
    status: "在线",
    address: "",
    openNum: 2,
    online: true,
    createdAt: "",
    updatedAt: "",
    source: "remote",
    usedPortIds: [],
    ports: [
      { id: 1, status: "idle", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 0, usedSeconds: 0 },
      { id: 2, status: "idle", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 0, usedSeconds: 0 }
    ],
    ...overrides
  };
}

describe("getPilePresentation", () => {
  it("marks a pile with any active port as charging", () => {
    const result = getPilePresentation(pile({
      usedPortIds: [1],
      ports: [
        { id: 1, status: "in_use", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 1, usedSeconds: 60 },
        { id: 2, status: "idle", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 0, usedSeconds: 0 }
      ]
    }));

    expect(result).toEqual({ label: "充电中", tone: "charging" });
  });

  it("marks an online unused pile as available", () => {
    expect(getPilePresentation(pile())).toEqual({ label: "全部空闲", tone: "idle" });
  });

  it("prioritizes the offline state", () => {
    expect(getPilePresentation(pile({ online: false }))).toEqual({ label: "设备离线", tone: "offline" });
  });
});
