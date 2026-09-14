import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { AddPileDialog } from "@/components/add-pile-dialog"

const addPile = vi.fn()
const requestJSON = vi.fn()
vi.mock("@/lib/http", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/http")>()),
  requestJSON: (...args: unknown[]) => requestJSON(...args),
}))
beforeEach(() => {
  vi.clearAllMocks()
  requestJSON.mockResolvedValue({ bound: true, scanEnabled: true })
})

vi.mock("@/lib/auth-context", () => ({
  useAuth: () => ({ currentUser: { deviceLimit: 10 } }),
}))

vi.mock("@/lib/dashboard-context", () => ({
  useDashboard: () => ({ addPile }),
}))

describe("AddPileDialog", () => {
  it("submits an invalid pile number to the backend validator", async () => {
    const user = userEvent.setup()
    addPile.mockRejectedValueOnce(new Error("桩号格式不正确"))
    render(<AddPileDialog />)

    await user.click(screen.getByRole("button", { name: "添加充电桩" }))
    await user.type(await screen.findByLabelText("桩号"), "invalid")
    await user.click(screen.getByRole("button", { name: "确认添加" }))

    expect(addPile).toHaveBeenCalledWith(
      expect.objectContaining({ number: "invalid" })
    )
  })
})

it("keeps the original add flow when an older backend omits scanEnabled", async () => {
  requestJSON.mockResolvedValue({ bound: false })
  const user = userEvent.setup()
  render(<AddPileDialog />)
  await user.click(screen.getByRole("button", { name: "添加充电桩" }))
  expect(await screen.findByLabelText("桩号")).toBeVisible()
  expect(screen.queryByAltText("微信扫码登录二维码")).not.toBeInTheDocument()
})
