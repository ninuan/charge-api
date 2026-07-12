import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAdminAutoRefresh } from "./useAdminAutoRefresh";

function setVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value
  });
  document.dispatchEvent(new Event("visibilitychange"));
}

describe("useAdminAutoRefresh", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible"
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("refreshes every minute while visible", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const controller = useAdminAutoRefresh(refresh, 60_000);
    controller.start();

    await vi.advanceTimersByTimeAsync(60_000);

    expect(refresh).toHaveBeenCalledTimes(1);
    controller.stop();
  });

  it("pauses while hidden and refreshes immediately when visible again", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const controller = useAdminAutoRefresh(refresh, 60_000);
    controller.start();

    setVisibility("hidden");
    await vi.advanceTimersByTimeAsync(120_000);
    expect(refresh).not.toHaveBeenCalled();

    setVisibility("visible");
    await Promise.resolve();
    expect(refresh).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(60_000);
    expect(refresh).toHaveBeenCalledTimes(2);
    controller.stop();
  });
});
