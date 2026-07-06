import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import YybLoginDialog from "./YybLoginDialog.vue";

const dialogStubs = {
  Dialog: { template: "<div><slot /></div>" },
  DialogTrigger: { template: "<span><slot /></span>" },
  DialogContent: { template: "<section><slot /></section>" },
  DialogDescription: { template: "<p><slot /></p>" },
  DialogFooter: { template: "<footer><slot /></footer>" },
  DialogHeader: { template: "<header><slot /></header>" },
  DialogTitle: { template: "<h2><slot /></h2>" }
};

describe("YybLoginDialog", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    vi.unstubAllGlobals();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("does not automatically poll after generating a QR code", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ sessionId: "sid-1", imageBase64: "data:image/jpeg;base64,abc", status: "waiting" })
    } as unknown as Response);
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(YybLoginDialog, {
      global: {
        stubs: dialogStubs
      }
    });

    await wrapper.findAll("button").find((button) => button.text().includes("生成扫码二维码"))?.trigger("click");
    await vi.runOnlyPendingTimersAsync();
    await vi.advanceTimersByTimeAsync(5000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/api/session/yyb-qr", expect.objectContaining({ method: "POST" }));
  });

  it("shows a clear status when the QR scan has been authorized", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ sessionId: "sid-1", imageBase64: "data:image/jpeg;base64,abc", status: "waiting" })
      } as unknown as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ sessionId: "sid-1", status: "authorized" })
      } as unknown as Response);
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(YybLoginDialog, {
      global: {
        stubs: dialogStubs
      }
    });

    await wrapper.findAll("button").find((button) => button.text().includes("生成扫码二维码"))?.trigger("click");
    await wrapper.findAll("button").find((button) => button.text().includes("检查扫码状态"))?.trigger("click");

    expect(wrapper.text()).toContain("扫码已确认，可以点击确认绑定");
  });

  it("keeps the QR login dialog scrollable and closable on mobile", () => {
    const wrapper = mount(YybLoginDialog, {
      global: {
        stubs: dialogStubs
      }
    });

    const content = wrapper.find('[data-testid="yyb-login-dialog"]');
    expect(content.exists()).toBe(true);
    expect(content.classes()).toContain("max-h-[calc(100dvh-2rem)]");
    expect(content.classes()).toContain("overflow-y-auto");
    expect(content.classes()).toContain("sm:max-w-2xl");
    expect(wrapper.get('[data-testid="yyb-login-close"]').text()).toContain("关闭");
  });

});
