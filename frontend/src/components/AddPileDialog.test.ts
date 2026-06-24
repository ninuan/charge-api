import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AddPileDialog from "./AddPileDialog.vue";
import { useAuthStore } from "@/stores/auth";
import type { CurrentUser } from "@/types/dashboard";

const user: CurrentUser = {
  id: "user-1",
  username: "alice",
  role: "user",
  enabled: true,
  createdAt: "2026-06-24T00:00:00Z",
  deviceLimit: 10,
  refreshEnabled: true,
  usageGuideAckAt: "2026-06-24T12:00:00Z"
};

const dialogStubs = {
  Dialog: { template: "<div><slot /></div>" },
  DialogTrigger: { template: "<span><slot /></span>" },
  DialogContent: { template: "<section><slot /></section>" },
  DialogDescription: { template: "<p><slot /></p>" },
  DialogFooter: { template: "<footer><slot /></footer>" },
  DialogHeader: { template: "<header><slot /></header>" },
  DialogTitle: { template: "<h2><slot /></h2>" }
};

describe("AddPileDialog", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.unstubAllGlobals();
  });

  it("submits the visible primary field as the pile number", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ id: "2601201412385560001", number: "61034278", ports: [] })
      } as unknown as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ piles: [], statistics: {}, refresh: {}, updatedAt: "2026-06-24T00:00:00Z" })
      } as unknown as Response);
    vi.stubGlobal("fetch", fetchMock);
    useAuthStore().currentUser = user;

    const wrapper = mount(AddPileDialog, {
      global: {
        stubs: dialogStubs
      }
    });

    expect(wrapper.text()).toContain("桩号");
    expect(wrapper.text()).not.toContain("设备长 ID *");

    await wrapper.get("input").setValue("61034278");
    await wrapper.get("form").trigger("submit.prevent");

    expect(fetchMock).toHaveBeenCalledWith("/api/piles", expect.objectContaining({
      method: "POST",
      body: expect.any(String)
    }));
    const body = JSON.parse((fetchMock.mock.calls[0]?.[1] as RequestInit).body as string);
    expect(body).toMatchObject({
      id: "",
      number: "61034278"
    });
  });
});
