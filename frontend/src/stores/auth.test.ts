import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAuthStore } from "./auth";
import type { CurrentUser } from "@/types/dashboard";

const baseUser: CurrentUser = {
  id: "user-1",
  username: "alice",
  role: "user",
  enabled: true,
  createdAt: "2026-06-24T00:00:00Z",
  deviceLimit: 10,
  refreshEnabled: true
};

function jsonResponse(body: unknown) {
  return {
    ok: true,
    json: vi.fn().mockResolvedValue(body)
  } as unknown as Response;
}

describe("auth store", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.unstubAllGlobals();
  });

  it("acknowledges the usage guide and updates the current user", async () => {
    const acknowledged = { ...baseUser, usageGuideAckAt: "2026-06-24T12:00:00Z" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(acknowledged));
    vi.stubGlobal("fetch", fetchMock);

    const auth = useAuthStore();
    auth.currentUser = baseUser;

    await auth.acknowledgeUsageGuide();

    expect(fetchMock).toHaveBeenCalledWith("/api/user/usage-guide/ack", {
      method: "POST",
      credentials: "include"
    });
    expect(auth.currentUser?.usageGuideAckAt).toBe("2026-06-24T12:00:00Z");
  });
});
