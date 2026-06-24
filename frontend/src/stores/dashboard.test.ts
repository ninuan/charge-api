import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useDashboardStore } from "./dashboard";

describe("dashboard store", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.unstubAllGlobals();
  });

  it("reports revoked sessions as an expired login", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: vi.fn().mockResolvedValue({ error: "login required" })
    } as unknown as Response));

    const store = useDashboardStore();

    await expect(store.fetchSnapshot()).rejects.toThrow("登录已失效，请重新登录");
  });
});
