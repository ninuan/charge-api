import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import PileCard from "./PileCard.vue";
import type { Pile } from "@/types/dashboard";

const pile: Pile = {
  id: "2601201412385560001",
  number: "61034278",
  name: "松园北侧",
  status: "在线",
  address: "3 号楼",
  openNum: 3,
  online: true,
  createdAt: "2026-06-23T10:00:00Z",
  updatedAt: "2026-06-23T10:10:00Z",
  source: "remote",
  usedPortIds: [2],
  ports: [
    { id: 1, status: "idle", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 0, usedSeconds: 0 },
    { id: 2, status: "in_use", powerKw: 2, energyKwh: 1, updatedAt: "", sessionMin: 12, usedSeconds: 720, usedText: "12 分钟", remainingText: "48 分钟" },
    { id: 3, status: "offline", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 0, usedSeconds: 0 }
  ]
};

describe("PileCard", () => {
  it("renders explicit status and timing text for every port state", () => {
    const wrapper = mount(PileCard, { props: { pile } });

    expect(wrapper.text()).toContain("空闲");
    expect(wrapper.text()).toContain("充电中");
    expect(wrapper.text()).toContain("离线");
    expect(wrapper.text()).toContain("已用 12 分钟");
    expect(wrapper.text()).toContain("剩余 48 分钟");
    expect(wrapper.text()).toContain("充电中");
    expect(wrapper.get("article").attributes("data-pile-state")).toBe("charging");
  });

  it("renders only matching ports while preserving whole-pile totals", () => {
    const wrapper = mount(PileCard, {
      props: { pile, filtering: true, visiblePortIds: [2] }
    });

    expect(wrapper.text()).toContain("当前显示 1 / 3 个匹配端口");
    expect(wrapper.text()).toContain("使用中");
    expect(wrapper.text()).toContain("空闲");
    expect(wrapper.findAll("[data-port-state]")).toHaveLength(1);
    expect(wrapper.get("[data-port-state]").attributes("data-port-state")).toBe("charging");
  });
});
