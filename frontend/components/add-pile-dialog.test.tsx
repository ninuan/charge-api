import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { AddPileDialog } from "@/components/add-pile-dialog"

const addPile = vi.fn()
const searchSharedPiles = vi.fn()

afterEach(cleanup)

beforeEach(() => {
  vi.clearAllMocks()
  searchSharedPiles.mockResolvedValue([
    {
      deviceId: "2601201412385560101",
      number: "61034278",
      name: "北门车棚 1 号桩",
      address: "北门东侧",
      openNum: 10,
      createdAt: "2026-11-21T00:00:00Z",
      updatedAt: "2026-11-21T00:00:00Z",
    },
  ])
})

vi.mock("@/lib/auth-context", () => ({
  useAuth: () => ({ currentUser: { deviceLimit: 10, refreshEnabled: true } }),
}))

vi.mock("@/lib/dashboard-context", () => ({
  useDashboard: () => ({
    snapshot: {
      piles: [],
      quota: {
        hasPersonalCookie: false,
        personalCookieStatus: "none",
        freeDailyLimit: 2,
        freeDailyUsed: 0,
        freeDailyRemaining: 2,
        bonusRemaining: 0,
        totalFreeRemaining: 2,
        contributedPileCount: 0,
        pileContributionBonusQuota: 2,
        freeUserWatchAllowed: false,
        publicPoolAvailable: 2,
      },
    },
    addPile,
    searchSharedPiles,
  }),
}))

describe("AddPileDialog", () => {
  it("lists shared catalog piles and allows one-click add without QR login", async () => {
    addPile.mockResolvedValueOnce({
      id: "2601201412385560101",
      number: "61034278",
      name: "北门车棚 1 号桩",
      fromSharedCatalog: true,
    })
    const user = userEvent.setup()
    render(<AddPileDialog />)

    await user.click(screen.getByRole("button", { name: "添加充电桩" }))
    expect(await screen.findByText("北门车棚 1 号桩")).toBeVisible()
    expect(screen.queryByAltText("微信扫码登录二维码")).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "添加" }))
    expect(addPile).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "2601201412385560101",
        number: "61034278",
      })
    )
  })

  it("allows contributing a new pile and keeps backend errors retryable", async () => {
    addPile.mockRejectedValueOnce(new Error("桩号格式不正确"))
    const user = userEvent.setup()
    render(<AddPileDialog />)

    await user.click(screen.getByRole("button", { name: "添加充电桩" }))
    await user.click(screen.getByRole("tab", { name: /录入新桩/ }))
    await user.type(await screen.findByLabelText("桩号（推荐）"), "invalid")
    await user.click(screen.getByRole("button", { name: "校验并收录" }))

    expect(addPile).toHaveBeenCalledWith(
      expect.objectContaining({ number: "invalid" })
    )
    expect(await screen.findByRole("alert")).toHaveTextContent("桩号格式不正确")
    expect(screen.getByRole("button", { name: "校验并收录" })).toBeEnabled()
  })
})
