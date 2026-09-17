import { act, cleanup, render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"
import { AnnouncementStrip } from "./announcement-strip"
const mocks = vi.hoisted(() => ({ request: vi.fn(), user: { id: "one" } }))
vi.mock("@/lib/announcements", () => ({
  announcementRequest: mocks.request,
  announcementDate: () => "9/17",
}))
vi.mock("@/lib/auth-context", () => ({
  useAuth: () => ({ currentUser: mocks.user }),
}))
vi.mock("@/lib/browser-state", () => ({ useOnline: () => true }))
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams("announcement=one"),
}))
vi.mock("next/dynamic", () => ({
  default:
    () =>
    ({ onAcknowledged }: { onAcknowledged: () => void }) => (
      <button onClick={onAcknowledged}>模拟确认成功</button>
    ),
}))
const empty = {
  item: null,
  unreadCount: 0,
  serverNow: new Date().toISOString(),
  nextBoundary: null,
}
const unread = {
  ...empty,
  unreadCount: 1,
  item: {
    id: "one",
    title: "维护公告",
    level: "normal",
    startAt: empty.serverNow,
    acknowledged: false,
  },
}
beforeEach(() => {
  mocks.request.mockReset()
  mocks.user = { id: "one" }
})
afterEach(cleanup)
it("confirmation supersedes an older in-flight summary", async () => {
  let resolveOld!: (value: typeof unread) => void
  let oldSignal: AbortSignal | undefined
  mocks.request
    .mockImplementationOnce((_path, init) => {
      oldSignal = init.signal
      return new Promise((resolve) => {
        resolveOld = resolve
      })
    })
    .mockResolvedValueOnce(empty)
  render(<AnnouncementStrip />)
  await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(1))
  await act(async () => {
    screen.getByText("模拟确认成功").click()
  })
  await screen.findByText("暂无待确认公告")
  expect(oldSignal?.aborted).toBe(true)
  await act(async () => resolveOld(unread))
  expect(screen.queryByText("维护公告")).not.toBeInTheDocument()
})
it("switching accounts discards the previous account's summary request", async () => {
  let resolveOld!: (value: typeof unread) => void
  mocks.request
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve
        })
    )
    .mockResolvedValueOnce(empty)
  const view = render(<AnnouncementStrip />)
  await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(1))
  mocks.user = { id: "two" }
  view.rerender(<AnnouncementStrip />)
  await screen.findByText("暂无待确认公告")
  await act(async () => resolveOld(unread))
  expect(screen.queryByText("维护公告")).not.toBeInTheDocument()
})
