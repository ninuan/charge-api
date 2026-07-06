import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import UsageGuideDialog from "./UsageGuideDialog.vue";
import { useAuthStore } from "@/stores/auth";
import type { CurrentUser } from "@/types/dashboard";

const userWithoutAck: CurrentUser = {
  id: "user-1",
  username: "alice",
  role: "user",
  enabled: true,
  createdAt: "2026-06-24T00:00:00Z",
  deviceLimit: 10,
  refreshEnabled: true
};

const dialogStubs = {
  Dialog: { template: "<div><slot /></div>" },
  DialogTrigger: { template: "<span><slot /></span>" },
  DialogContent: { props: ["showCloseButton"], template: "<section><slot /></section>" },
  DialogDescription: { template: "<p><slot /></p>" },
  DialogFooter: { template: "<footer><slot /></footer>" },
  DialogHeader: { template: "<header><slot /></header>" },
  DialogTitle: { template: "<h2><slot /></h2>" }
};

function mountGuide() {
  return mount(UsageGuideDialog, {
    global: {
      stubs: dialogStubs
    }
  });
}

describe("UsageGuideDialog", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.unstubAllGlobals();
  });

  it("requires first-time users to scroll to the end before closing", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ ...userWithoutAck, usageGuideAckAt: "2026-06-24T12:00:00Z" })
    } as unknown as Response);
    vi.stubGlobal("fetch", fetchMock);
    const auth = useAuthStore();
    auth.currentUser = userWithoutAck;

    const wrapper = mountGuide();
    await nextTick();

    const primary = wrapper.get('[data-testid="usage-guide-primary"]');
    expect(primary.attributes("disabled")).toBeDefined();

    const scroller = wrapper.get('[data-testid="usage-guide-scroll"]');
    Object.defineProperty(scroller.element, "clientHeight", { configurable: true, value: 100 });
    Object.defineProperty(scroller.element, "scrollHeight", { configurable: true, value: 200 });
    Object.defineProperty(scroller.element, "scrollTop", { configurable: true, value: 100 });
    await scroller.trigger("scroll");

    expect(wrapper.get('[data-testid="usage-guide-primary"]').attributes("disabled")).toBeUndefined();
    await wrapper.get('[data-testid="usage-guide-primary"]').trigger("click");

    expect(fetchMock).toHaveBeenCalledWith("/api/user/usage-guide/ack", {
      method: "POST",
      credentials: "include"
    });
    expect(auth.currentUser?.usageGuideAckAt).toBe("2026-06-24T12:00:00Z");
  });

  it("allows acknowledged users to reopen and close the guide without scrolling", async () => {
    const auth = useAuthStore();
    auth.currentUser = { ...userWithoutAck, usageGuideAckAt: "2026-06-24T12:00:00Z" };

    const wrapper = mountGuide();
    await wrapper.get('[data-testid="usage-guide-trigger"]').trigger("click");

    const primary = wrapper.get('[data-testid="usage-guide-primary"]');
    expect(primary.attributes("disabled")).toBeUndefined();
    expect(primary.text()).toContain("关闭");
    expect(wrapper.text()).not.toContain("这里可以随时回来看，不会再次强制阅读。");
    expect(wrapper.find(".usage-guide-status").exists()).toBe(false);
  });

  it("uses a wider two-column guide layout on desktop", async () => {
    const auth = useAuthStore();
    auth.currentUser = { ...userWithoutAck, usageGuideAckAt: "2026-06-24T12:00:00Z" };

    const wrapper = mountGuide();
    await wrapper.get('[data-testid="usage-guide-trigger"]').trigger("click");

    const dialogClasses = wrapper.get('[data-testid="usage-guide-dialog"]').classes();
    expect(dialogClasses).toContain("usage-guide-dialog--wide");
    expect(dialogClasses).toContain("sm:max-w-[min(1180px,calc(100vw-2rem))]");
    expect(wrapper.get('[data-testid="usage-guide-layout"]').classes()).toContain("usage-guide-layout");
    expect(wrapper.get('[data-testid="usage-guide-sidebar"]').classes()).toContain("usage-guide-sidebar");
    expect(wrapper.findAll('[data-testid="usage-guide-step-link"]')).toHaveLength(6);
  });

  it("explains the new scan-login first run flow without capture-tool instructions", async () => {
    const auth = useAuthStore();
    auth.currentUser = { ...userWithoutAck, usageGuideAckAt: "2026-06-24T12:00:00Z" };

    const wrapper = mountGuide();
    await wrapper.get('[data-testid="usage-guide-trigger"]').trigger("click");

    expect(wrapper.text()).toContain("扫码登录与充电桩添加说明");
    expect(wrapper.text()).toContain("打开扫码登录");
    expect(wrapper.text()).toContain("使用微信扫码");
    expect(wrapper.text()).toContain("确认绑定状态");
    expect(wrapper.text()).toContain("添加充电桩");
    expect(wrapper.text()).toContain("刷新查看状态");
    expect(wrapper.text()).toContain("如果扫码登录暂时不可用，仍可以在高级设置中手动更新 Cookie");
    expect(wrapper.text()).not.toContain("Reqable");
    expect(wrapper.text()).not.toContain("抓包");
    expect(wrapper.text()).not.toContain("前端只会请求 Charge 后端接口");
    expect(wrapper.findAll('[data-testid="usage-guide-image"]')).toHaveLength(0);
  });

  it("keeps the footer visible while only the guide body scrolls", async () => {
    const auth = useAuthStore();
    auth.currentUser = { ...userWithoutAck, usageGuideAckAt: "2026-06-24T12:00:00Z" };

    const wrapper = mountGuide();
    await wrapper.get('[data-testid="usage-guide-trigger"]').trigger("click");

    const dialogClasses = wrapper.get('[data-testid="usage-guide-dialog"]').classes();
    expect(dialogClasses).toContain("h-[min(820px,calc(100dvh-4.5rem))]");
    expect(dialogClasses).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(wrapper.get('[data-testid="usage-guide-scroll"]').classes()).toContain("min-h-0");
    expect(wrapper.get('[data-testid="usage-guide-footer"]').classes()).toContain("pb-[calc(2rem+env(safe-area-inset-bottom))]");
    expect(wrapper.get('[data-testid="usage-guide-footer-inner"]').classes()).toContain("usage-guide-footer-inner");
  });
});
