import { act, cleanup, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"
import { AdminAnnouncements } from "./admin-announcements"

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  push: vi.fn(),
  query: "announcement=one",
}))
vi.mock("@/lib/announcements", () => ({
  announcementRequest: mocks.request,
  announcementDate: () => "9/17",
}))
vi.mock("@/lib/browser-state", () => ({ useOnline: () => true }))
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(mocks.query),
  useRouter: () => ({ push: mocks.push }),
}))
beforeEach(() => {
  mocks.request.mockReset()
  mocks.push.mockReset()
  mocks.query = "announcement=one"
})
afterEach(cleanup)

it.each([false, true])(
  "ignores a copied draft response after navigation (failure=%s)",
  async (failure) => {
    let finish!: () => void
    mocks.request
      .mockResolvedValueOnce({
        id: "one",
        title: "旧公告",
        body: "正文",
        level: "normal",
        status: "withdrawn",
        startAt: "2026-09-17T00:00:00Z",
        endAt: null,
        version: 2,
      })
      .mockImplementationOnce(
        () =>
          new Promise((resolve, reject) => {
            finish = () =>
              failure
                ? reject(new Error("旧页面复制失败"))
                : resolve({ id: "copied" })
          })
      )
      .mockResolvedValue({ items: [], total: 0, page: 1 })
    const view = render(<AdminAnnouncements />)
    const copy = await screen.findByRole("button", { name: "复制为新草稿" })
    await act(async () => {
      copy.click()
    })
    expect(mocks.request).toHaveBeenCalledTimes(2)
    mocks.query = ""
    view.rerender(<AdminAnnouncements />)
    await screen.findByRole("button", { name: "新建公告" })
    await act(async () => {
      finish()
    })
    expect(mocks.push).not.toHaveBeenCalled()
    expect(screen.queryByText("旧页面复制失败")).not.toBeInTheDocument()
  }
)
