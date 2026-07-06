import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useYYBStore } from "./yyb";

describe("yyb store", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.unstubAllGlobals();
  });

  it("proxies QR login only through Charge API endpoints", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ bound: false })
      } as unknown as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ sessionId: "sid-1", imageBase64: "data:image/png;base64,abc", status: "waiting" })
      } as unknown as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ sessionId: "sid-1", status: "confirmed", message: "ready" })
      } as unknown as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ bound: true, nickname: "Alice", openidSuffix: "id-1", cookieSynced: false, message: "添加充电桩后自动生效" })
      } as unknown as Response);
    vi.stubGlobal("fetch", fetchMock);

    const store = useYYBStore();
    await store.fetchBinding();
    await store.createQR();
    await store.pollQR();
    const result = await store.confirmQR();

    expect(result.cookieSynced).toBe(false);
    expect(store.binding.bound).toBe(true);
    expect(store.qr).toBeNull();
    expect(store.poll).toBeNull();
    const urls = fetchMock.mock.calls.map((call) => call[0]);
    expect(urls).toEqual([
      "/api/session/yyb-binding",
      "/api/session/yyb-qr",
      "/api/session/yyb-qr/sid-1/poll",
      "/api/session/yyb-qr/sid-1/confirm"
    ]);
    expect(urls.join(" ")).not.toContain("127.0.0.1:8000");
  });

  it("does not start overlapping QR poll requests", async () => {
    let resolvePoll!: (value: Response) => void;
    const pendingPoll = new Promise<Response>((resolve) => {
      resolvePoll = resolve;
    });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ sessionId: "sid-1", imageBase64: "data:image/png;base64,abc", status: "waiting" })
      } as unknown as Response)
      .mockReturnValueOnce(pendingPoll);
    vi.stubGlobal("fetch", fetchMock);

    const store = useYYBStore();
    await store.createQR();
    const firstPoll = store.pollQR();
    const secondPoll = store.pollQR();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    resolvePoll({
      ok: true,
      json: vi.fn().mockResolvedValue({ sessionId: "sid-1", status: "pending" })
    } as unknown as Response);
    await Promise.all([firstPoll, secondPoll]);
  });

  it("turns missing QR sessions into a friendly recovery message", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ sessionId: "sid-1", imageBase64: "data:image/png;base64,abc", status: "waiting" })
      } as unknown as Response)
      .mockResolvedValueOnce({
        ok: false,
        status: 502,
        json: vi.fn().mockResolvedValue({ error: `yyb request failed: status=404 body={"code":404,"msg":"qr session not found","data":null}` })
      } as unknown as Response);
    vi.stubGlobal("fetch", fetchMock);

    const store = useYYBStore();
    await store.createQR();

    await expect(store.pollQR()).rejects.toThrow("二维码会话已失效，请重新生成二维码");
    expect(store.qr).toBeNull();
  });
});
