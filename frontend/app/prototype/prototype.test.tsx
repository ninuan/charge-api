import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { ChargePrototype } from "./prototype"
import { initialData, STORAGE_KEY } from "./model"

beforeEach(() => {
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(initialData()))
  window.history.replaceState(null, "", "/prototype/#/piles")
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  vi.stubGlobal("fetch", vi.fn())
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

async function navigate(hash: string) {
  await act(async () => {
    window.location.hash = hash
    window.dispatchEvent(new HashChangeEvent("hashchange"))
  })
}
function stored() {
  return JSON.parse(window.localStorage.getItem(STORAGE_KEY)!) as ReturnType<
    typeof initialData
  >
}

describe("working prototype", () => {
  it("opens with a usable device hierarchy without any business requests", () => {
    render(<ChargePrototype />)
    expect(
      screen.getByRole("heading", { name: "常用充电桩", level: 1 })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { name: "桂园 · 北门车棚", level: 2 })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "查看 2 号口，使用中" })
    ).toBeInTheDocument()
    expect(fetch).not.toHaveBeenCalled()
  })
  it("filters by number and recovers from an empty search", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    const search = screen.getByRole("textbox", { name: "搜索充电桩" })
    await user.type(search, "not-found")
    expect(
      screen.getByRole("heading", { name: "没有找到充电桩" })
    ).toBeInTheDocument()
    await user.clear(search)
    await user.type(search, "608203")
    expect(
      screen.getByRole("button", { name: "查看 桂园 · 南门车棚" })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "查看 桂园 · 北门车棚" })
    ).not.toBeInTheDocument()
  })
  it("switches details and expands port information next to the selected row", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    await user.click(
      screen.getByRole("button", { name: "查看 2 号口，使用中" })
    )
    await waitFor(() =>
      expect(
        screen.getByRole("region", { name: "2 号口详情" })
      ).toBeInTheDocument()
    )
    const row = screen.getByRole("button", { name: "查看 2 号口，使用中" })
    expect(row.nextElementSibling).toHaveAttribute("aria-label", "2 号口详情")
  })
  it("does not schedule a reminder when a pile already has idle ports", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    await user.click(screen.getByRole("button", { name: "有空闲时提醒我" }))
    expect(screen.getByText("现在就有 4 个空闲口")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "开始等待" })
    ).not.toBeInTheDocument()
    expect(stored().watches).toHaveLength(3)
  })
  it("validates duplicate device identifiers and does not invent port readings", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    await user.click(screen.getByRole("button", { name: "添加充电桩" }))
    await user.type(screen.getByRole("textbox", { name: /桩号/ }), "608201")
    await user.click(screen.getByRole("button", { name: "添加到常用桩" }))
    expect(screen.getByRole("alert")).toHaveTextContent("已经")
    await user.clear(screen.getByRole("textbox", { name: /桩号/ }))
    await user.type(screen.getByRole("textbox", { name: /桩号/ }), "608205")
    await user.click(screen.getByRole("button", { name: "添加到常用桩" }))
    await waitFor(() => expect(stored().piles).toHaveLength(5))
    expect(stored().piles[4].ports).toEqual([])
    expect(stored().piles[4].online).toBe(false)
    expect(fetch).not.toHaveBeenCalled()
  })
  it("marking messages read does not resolve active problems", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    await navigate("#/notifications")
    await user.click(screen.getByRole("button", { name: "全部已读" }))
    await waitFor(() =>
      expect(stored().notices.every((n) => n.read)).toBe(true)
    )
    expect(
      stored().notices.find((n) => n.type === "pile_offline")?.resolved
    ).toBe(false)
  })
  it("keeps cached states explicit and blocks writes when offline", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    await user.click(screen.getByRole("button", { name: /示例数据/ }))
    await user.selectOptions(
      screen.getByRole("combobox", { name: "页面状态" }),
      "offline"
    )
    await user.click(screen.getByRole("button", { name: "应用预览" }))
    expect(screen.getByRole("button", { name: "刷新状态" })).toBeDisabled()
    expect(screen.queryByText("可以使用")).not.toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "查看 1 号口，上次空闲" })
    ).toBeInTheDocument()
  })
  it("keeps administrative pages out of the ordinary workspace", async () => {
    render(<ChargePrototype />)
    await navigate("#/users")
    expect(
      screen.getByRole("heading", { name: "这个页面属于另一类账户" })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "创建用户" })
    ).not.toBeInTheDocument()
  })
  it("uses the skip link without changing the route", async () => {
    render(<ChargePrototype />)
    await navigate("#/notifications")
    fireEvent.click(screen.getByRole("link", { name: "跳到主要内容" }))
    expect(window.location.hash).toBe("#/notifications")
    expect(document.activeElement).toBe(screen.getByRole("main"))
  })
  it("simulates the full reminder to notification to port loop", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    await user.click(screen.getByRole("button", { name: /示例数据/ }))
    await user.click(screen.getByRole("button", { name: /模拟发现空闲口/ }))
    await waitFor(() => expect(stored().watches[0].status).toBe("notified"))
    expect(stored().piles[1].ports[0].status).toBe("idle")
    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "通知", level: 1 })
      ).toBeInTheDocument()
    )
    await user.click(
      screen.getByRole("button", { name: /桂园 · 3 号楼有空闲口了/ })
    )
    await user.click(screen.getByRole("button", { name: "查看充电桩" }))
    await waitFor(() =>
      expect(
        screen.getByRole("region", { name: "1 号口详情" })
      ).toBeInTheDocument()
    )
    expect(fetch).not.toHaveBeenCalled()
  })
  it("can apply dark theme and an explicit reduced-motion preference", async () => {
    const user = userEvent.setup()
    const { container } = render(<ChargePrototype />)
    await user.click(screen.getByRole("button", { name: "切换到深色" }))
    expect(container.querySelector(".cp-root")).toHaveAttribute(
      "data-theme",
      "dark"
    )
    await navigate("#/account?tab=appearance")
    await user.click(screen.getByRole("switch", { name: "始终减少动态效果" }))
    expect(container.querySelector(".cp-root")).toHaveAttribute(
      "data-reduced",
      "true"
    )
  })
  it("can dismiss a dialog using Escape", async () => {
    const user = userEvent.setup()
    render(<ChargePrototype />)
    await user.click(screen.getByRole("button", { name: "添加充电桩" }))
    const dialog = screen.getByRole("dialog")
    expect(
      within(dialog).getByRole("heading", { name: "添加常用充电桩" })
    ).toBeInTheDocument()
    await user.keyboard("{Escape}")
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
    )
  })
})
