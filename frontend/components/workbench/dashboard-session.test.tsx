import { cleanup, render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { DashboardSession, useDashboardSession } from "./dashboard-session"

const mocks = vi.hoisted(() => ({
  user: { id: "user", role: "user" } as { id: string; role: string } | null,
  fetchMe: vi.fn(),
  snapshot: vi.fn(),
  watch: vi.fn(),
  notices: vi.fn(),
  connect: vi.fn(),
  disconnect: vi.fn(),
  subscribe: vi.fn(() => () => {}),
  replace: vi.fn(),
  push: vi.fn(),
}))
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mocks.replace, push: mocks.push }),
}))
vi.mock("@/lib/auth-context", () => ({
  useAuth: () => ({ currentUser: mocks.user, fetchMe: mocks.fetchMe }),
}))
vi.mock("@/lib/dashboard-context", () => ({
  useDashboard: () => ({
    fetchSnapshot: mocks.snapshot,
    connectStream: mocks.connect,
    disconnectStream: mocks.disconnect,
    subscribeNotifications: mocks.subscribe,
  }),
}))
vi.mock("@/lib/watch-context", () => ({
  useWatch: () => ({ load: mocks.watch }),
}))
vi.mock("@/lib/notification-context", () => ({
  useNotifications: () => ({ load: mocks.notices }),
}))
vi.mock("@/components/app-shell", () => ({
  AppShell: ({ children }: { children: React.ReactNode }) => (
    <main>{children}</main>
  ),
}))
function Child() {
  const { initialLoading, error } = useDashboardSession()
  return <p>{initialLoading ? "读取中" : error || "已准备好"}</p>
}
beforeEach(() => {
  vi.clearAllMocks()
  mocks.user = { id: "user", role: "user" }
  mocks.snapshot.mockResolvedValue(undefined)
  mocks.watch.mockResolvedValue(undefined)
  mocks.notices.mockResolvedValue(undefined)
})
afterEach(cleanup)
describe("dashboard session boundary", () => {
  it("authorizes before loading personal resources and keeps SSE until unmount", async () => {
    const { unmount } = render(
      <DashboardSession>
        <Child />
      </DashboardSession>
    )
    await screen.findByText("已准备好")
    expect(mocks.snapshot).toHaveBeenCalledTimes(1)
    expect(mocks.watch).toHaveBeenCalledTimes(1)
    expect(mocks.connect).toHaveBeenCalledTimes(1)
    unmount()
    expect(mocks.disconnect).toHaveBeenCalledTimes(1)
  })
  it("does not read user devices or preferences for an administrator", async () => {
    mocks.user = { id: "admin", role: "admin" }
    render(
      <DashboardSession>
        <Child />
      </DashboardSession>
    )
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/admin"))
    expect(mocks.snapshot).not.toHaveBeenCalled()
    expect(mocks.watch).not.toHaveBeenCalled()
    expect(mocks.notices).not.toHaveBeenCalled()
    expect(mocks.connect).not.toHaveBeenCalled()
  })
  it("keeps the pile view usable if the optional notification service fails", async () => {
    mocks.notices.mockRejectedValue(new Error("notifications unavailable"))
    render(
      <DashboardSession>
        <Child />
      </DashboardSession>
    )
    await screen.findByText("已准备好")
    expect(mocks.connect).toHaveBeenCalledTimes(1)
  })
  it("exposes a snapshot failure instead of pretending there are no devices", async () => {
    mocks.snapshot.mockRejectedValue(new Error("snapshot unavailable"))
    render(
      <DashboardSession>
        <Child />
      </DashboardSession>
    )
    await screen.findByText("snapshot unavailable")
  })
})
