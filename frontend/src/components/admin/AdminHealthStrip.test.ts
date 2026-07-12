import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import AdminHealthStrip from "./AdminHealthStrip.vue";
import type { AdminHealth } from "@/types/dashboard";

const health: AdminHealth = {
  checkedAt: "2026-07-10T02:30:00Z",
  charge: { state: "healthy", message: "服务正常" },
  database: { state: "healthy", message: "存储正常" },
  yyb: { state: "degraded", message: "扫码服务连接异常" }
};

describe("AdminHealthStrip", () => {
  it("renders compact service chips with short state labels", () => {
    const wrapper = mount(AdminHealthStrip, {
      props: { health, loading: false, error: "" }
    });

    expect(wrapper.findAll(".admin-health-chip")).toHaveLength(3);
    expect(wrapper.text()).toContain("Charge");
    expect(wrapper.text()).toContain("SQLite");
    expect(wrapper.text()).toContain("扫码服务");
    expect(wrapper.text()).toContain("异常");
    expect(wrapper.find(".admin-health-item").exists()).toBe(false);
  });
});
