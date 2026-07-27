import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminIncidentPanel } from "@/components/admin-incident-panel"
import { adminApi } from "@/lib/admin-api"
import type { SystemException } from "@/lib/types"

vi.mock("@/lib/admin-api", () => ({
  adminApi: {
    incidents: vi.fn(),
    updateIncident: vi.fn(),
  },
}))

const issue: SystemException = {
  id: "offline-user-1",
  userId: "user-1",
  username: "alice",
  type: "offline",
  level: "warning",
  message: "2 个充电口处于离线状态",
  status: "open",
  occurrences: 3,
  firstSeenAt: "2026-07-27T08:00:00Z",
  time: "2026-07-27T09:00:00Z",
}

describe("AdminIncidentPanel", () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("opens the related user and records an acknowledgement note", async () => {
    const user = userEvent.setup()
    const onUser = vi.fn()
    vi.mocked(adminApi.incidents).mockResolvedValue([issue])
    vi.mocked(adminApi.updateIncident).mockImplementation(
      async (_id, payload) => ({
        ...issue,
        ...payload,
        handledBy: "admin",
        handledAt: "2026-07-27T10:00:00Z",
      })
    )

    render(<AdminIncidentPanel initialIssues={[issue]} onUser={onUser} />)
    await user.click(screen.getByRole("button", { name: "详情" }))
    expect(onUser).toHaveBeenCalledWith("user-1")

    await user.click(screen.getByRole("button", { name: "确认" }))
    await user.type(
      screen.getByPlaceholderText("处理备注（可选，最多 500 字）"),
      "已联系用户"
    )
    await user.click(screen.getByRole("button", { name: "确认更新" }))

    await waitFor(() =>
      expect(adminApi.updateIncident).toHaveBeenCalledWith(
        issue.id,
        expect.objectContaining({
          status: "acknowledged",
          note: "已联系用户",
        })
      )
    )
    expect(await screen.findByText("已联系用户")).toBeInTheDocument()
  })
})
