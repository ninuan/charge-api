import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminSettings } from "@/components/admin-settings"

vi.mock("@/components/appearance-settings", () => ({
  AppearanceSettings: () => <div>界面外观</div>,
}))

const { adminApiMock } = vi.hoisted(() => ({
  adminApiMock: {
    createInvite: vi.fn(),
    removeInvite: vi.fn(),
    saveSettings: vi.fn(),
  },
}))

vi.mock("@/lib/admin-api", () => ({ adminApi: adminApiMock }))

const settings = {
  openRegistration: true,
  inviteRequired: false,
  defaultRefreshEnabled: true,
  defaultDeviceLimit: 10,
  statsRetentionDays: 90,
}

const invitePage = {
  items: Array.from({ length: 20 }, (_, index) => ({
    id: `invite-${index + 1}`,
    code: `CHG-${index + 1}`,
    usedCount: 0,
    enabled: true,
    createdAt: "2026-07-23T00:00:00Z",
    expiresAt: "",
  })),
  page: 1,
  pageSize: 20,
  total: 41,
  totalPages: 3,
}

describe("AdminSettings", () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("renders a bounded invite page and requests the next page", async () => {
    const user = userEvent.setup()
    const reload = vi.fn().mockResolvedValue(undefined)

    render(
      <AdminSettings
        settings={settings}
        setSettings={vi.fn()}
        invitePage={invitePage}
        reload={reload}
      />
    )

    expect(screen.getByText("CHG-20")).toBeInTheDocument()
    expect(screen.queryByText("CHG-21")).not.toBeInTheDocument()
    expect(screen.getByText("第 1/3 页 · 共 41 个")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "下一页" }))

    expect(reload).toHaveBeenCalledWith(2)
  })

  it("uses the shadcn checkbox to update registration settings", async () => {
    const user = userEvent.setup()
    const setSettings = vi.fn()

    render(
      <AdminSettings
        settings={settings}
        setSettings={setSettings}
        invitePage={null}
        reload={vi.fn().mockResolvedValue(undefined)}
      />
    )

    await user.click(screen.getByRole("checkbox", { name: "开放自助注册" }))

    expect(setSettings).toHaveBeenCalledWith({
      ...settings,
      openRegistration: false,
    })
  })
})
