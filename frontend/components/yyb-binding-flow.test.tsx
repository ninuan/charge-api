import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { StrictMode } from "react"
import { YybBindingFlow } from "@/components/yyb-binding-flow"

const requestJSON = vi.fn()
vi.mock("@/lib/http", () => ({
  requestJSON: (...args: unknown[]) => requestJSON(...args),
}))
const qr = { sessionId: "one", imageBase64: "data:image/png;base64,YQ==" }
let status = "pending"
beforeEach(() => {
  vi.clearAllMocks()
  Object.defineProperty(navigator, "onLine", {
    configurable: true,
    value: true,
  })
  status = "pending"
  requestJSON.mockImplementation((path: string) =>
    Promise.resolve(
      path.endsWith("/poll")
        ? { sessionId: "one", status }
        : path.endsWith("/confirm")
          ? { bound: true, sessionId: "one", syncState: "not_needed" }
          : qr
    )
  )
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})
async function check() {
  await userEvent.click(
    await screen.findByRole("button", { name: "检查扫码状态" })
  )
}

describe("QR binding lifecycle", () => {
  it("creates once in strict mode and enables only after authorization; requires explicit continue", async () => {
    const continued = vi.fn()
    render(
      <StrictMode>
        <YybBindingFlow continueLabel="继续添加充电桩" onContinue={continued} />
      </StrictMode>
    )
    await screen.findByRole("img")
    expect(
      requestJSON.mock.calls.filter(([path]) => path === "/api/session/yyb-qr")
    ).toHaveLength(1)
    const confirm = screen.getByRole("button", { name: "确认绑定" })
    expect(confirm).toBeDisabled()
    status = "scanned"
    await check()
    expect(confirm).toBeDisabled()
    expect(screen.getByText("已扫码，请在微信中确认授权")).toBeVisible()
    status = "authorized"
    await check()
    expect(confirm).toBeEnabled()
    await userEvent.click(confirm)
    expect(await screen.findByText("微信已绑定")).toBeVisible()
    expect(continued).not.toHaveBeenCalled()
    await userEvent.click(
      screen.getByRole("button", { name: "继续添加充电桩" })
    )
    expect(continued).toHaveBeenCalledTimes(1)
  })
  it.each([
    "pending",
    "scanned",
    "expired",
    "cancelled",
    "unknown",
    "unrecognized",
  ])("does not allow %s to confirm", async (value) => {
    render(<YybBindingFlow onContinue={vi.fn()} />)
    await screen.findByRole("img")
    status = value
    await check()
    expect(screen.getByRole("button", { name: "确认绑定" })).toBeDisabled()
  })
  it("invalidates an old poll immediately when generating another code", async () => {
    let resolve!: (value: unknown) => void
    render(<YybBindingFlow onContinue={vi.fn()} />)
    await screen.findByRole("img")
    requestJSON.mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done
        })
    )
    await check()
    requestJSON.mockResolvedValueOnce({ ...qr, sessionId: "two" })
    await userEvent.click(screen.getByRole("button", { name: "重新生成" }))
    await act(async () => resolve({ sessionId: "one", status: "authorized" }))
    expect(screen.getByRole("button", { name: "确认绑定" })).toBeDisabled()
  })
  it("reconciles the exact session after a lost confirm response", async () => {
    render(<YybBindingFlow onContinue={vi.fn()} />)
    await screen.findByRole("img")
    status = "authorized"
    await check()
    requestJSON.mockRejectedValueOnce(new Error("timeout"))
    await userEvent.click(screen.getByRole("button", { name: "确认绑定" }))
    expect(screen.getByRole("button", { name: "确认绑定" })).toBeDisabled()
    requestJSON.mockResolvedValueOnce({
      sessionId: "one",
      status: "saved",
      binding: { bound: true, sessionId: "one", syncState: "failed" },
    })
    await check()
    expect(await screen.findByText("微信已绑定")).toBeVisible()
    expect(screen.getByText(/平台连接暂未恢复/)).toBeVisible()
    expect(
      requestJSON.mock.calls.filter(([path]) => path.endsWith("/confirm"))
    ).toHaveLength(1)
  })
  it("requires a fresh check after going offline", async () => {
    render(<YybBindingFlow onContinue={vi.fn()} />)
    await screen.findByRole("img")
    status = "authorized"
    await check()
    act(() => {
      Object.defineProperty(navigator, "onLine", {
        configurable: true,
        value: false,
      })
      window.dispatchEvent(new Event("offline"))
    })
    expect(screen.getByRole("button", { name: "确认绑定" })).toBeDisabled()
    act(() => {
      Object.defineProperty(navigator, "onLine", {
        configurable: true,
        value: true,
      })
      window.dispatchEvent(new Event("online"))
    })
    expect(screen.getByRole("button", { name: "确认绑定" })).toBeDisabled()
    await check()
    expect(screen.getByRole("button", { name: "确认绑定" })).toBeEnabled()
  })
  it("stops after three consecutive errors and resumes on a manual check", async () => {
    vi.useFakeTimers()
    render(<YybBindingFlow onContinue={vi.fn()} />)
    await act(async () => {
      await Promise.resolve()
    })
    requestJSON.mockRejectedValue(new Error("network"))
    for (let i = 0; i < 3; i++)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(3000)
      })
    expect(screen.getByText(/自动检测已暂停/)).toBeVisible()
    const calls = requestJSON.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })
    expect(requestJSON).toHaveBeenCalledTimes(calls)
    requestJSON.mockResolvedValue({ sessionId: "one", status: "pending" })
    fireEvent.click(screen.getByRole("button", { name: "检查扫码状态" }))
    await act(async () => {
      await Promise.resolve()
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })
    expect(requestJSON).toHaveBeenCalledTimes(calls + 2)
  })
  it("aborts in-flight requests on close", async () => {
    let signal: AbortSignal | undefined
    requestJSON.mockImplementation((_path, options) => {
      signal = options.signal
      return new Promise(() => {})
    })
    const view = render(<YybBindingFlow onContinue={vi.fn()} />)
    await waitFor(() => expect(requestJSON).toHaveBeenCalled())
    view.unmount()
    expect(signal?.aborted).toBe(true)
  })
})

it("blocks returning to the draft during confirmation and removes that shortcut after saving", async () => {
  const back = vi.fn()
  render(<YybBindingFlow onContinue={vi.fn()} onBack={back} />)
  await screen.findByRole("img")
  status = "authorized"
  await check()
  let resolve!: (value: unknown) => void
  requestJSON.mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done
      })
  )
  await userEvent.click(screen.getByRole("button", { name: "确认绑定" }))
  expect(screen.getByRole("button", { name: "返回填写" })).toBeDisabled()
  await act(async () => resolve({ bound: true, sessionId: "one" }))
  expect(await screen.findByText("微信已绑定")).toBeVisible()
  expect(
    screen.queryByRole("button", { name: "返回填写" })
  ).not.toBeInTheDocument()
  expect(back).not.toHaveBeenCalled()
})
