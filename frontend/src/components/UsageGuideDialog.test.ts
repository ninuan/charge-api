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
    expect(wrapper.findAll('[data-testid="usage-guide-step-link"]')).toHaveLength(7);
  });

  it("shows screenshots only for the steps that need visual guidance", async () => {
    const auth = useAuthStore();
    auth.currentUser = { ...userWithoutAck, usageGuideAckAt: "2026-06-24T12:00:00Z" };

    const wrapper = mountGuide();
    await wrapper.get('[data-testid="usage-guide-trigger"]').trigger("click");

    const images = wrapper.findAll('[data-testid="usage-guide-image"]');
    expect(images).toHaveLength(6);
    expect(images[0].attributes("alt")).toContain("Reqable 官网");
    expect(images[1].attributes("alt")).toContain("开启代理");
    expect(images[2].attributes("alt")).toContain("充电桩二维码");
    expect(images[4].attributes("alt")).toContain("复制按钮");
    expect(wrapper.text()).toContain("选择自己设备对应的 Reqable 版本");
    expect(wrapper.text()).toContain("切到 Cookies 面板，复制完整 Cookie");
    expect(wrapper.find("#usage-guide-step-6 [data-testid='usage-guide-image']").exists()).toBe(false);
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
