import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { AddPileDialog } from "@/components/add-pile-dialog"
import { RequestError } from "@/lib/http"

const addPile = vi.fn()
const requestJSON = vi.fn()
afterEach(cleanup)
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
  it("keeps a recoverable platform error retryable without requiring a new binding", async () => {
    addPile.mockRejectedValueOnce(
      new RequestError("已保留平台绑定，请稍后重试", 502, "YYB_RECOVERY_FAILED")
    )
    const user = userEvent.setup()
    render(<AddPileDialog />)
    await user.click(screen.getByRole("button", { name: "添加充电桩" }))
    await user.type(await screen.findByLabelText("桩号"), "61034278")
    await user.click(screen.getByRole("button", { name: "确认添加" }))
    expect(await screen.findByRole("alert")).toHaveTextContent("已保留平台绑定")
    expect(screen.getByRole("button", { name: "确认添加" })).toBeEnabled()
    expect(screen.queryByRole("button", { name: "去绑定 / 重新扫码" })).not.toBeInTheDocument()
    expect(screen.getByLabelText("桩号")).toHaveValue("61034278")
    await user.click(screen.getByRole("button", { name: "取消" }))
  })

  it("requires reauthorization only after the server rejects credential recovery", async () => {
    requestJSON.mockResolvedValue({
      bound: true,
      scanEnabled: true,
      status: "expired",
    })
    addPile.mockRejectedValueOnce(
      new RequestError("平台登录需要重新确认", 409, "YYB_RESCAN_REQUIRED")
    )
    const user = userEvent.setup()
    render(<AddPileDialog />)
    await user.click(screen.getByRole("button", { name: "添加充电桩" }))
    await user.type(await screen.findByLabelText("桩号"), "61034278")
    await user.click(screen.getByRole("button", { name: "确认添加" }))
    expect(
      await screen.findByText(
        "平台登录无法自动恢复，请重新扫码授权后继续添加。"
      )
    ).toBeVisible()
    expect(screen.getByRole("button", { name: "确认添加" })).toBeDisabled()
    expect(screen.getByLabelText("桩号")).toHaveValue("61034278")
    await user.click(screen.getByRole("button", { name: "取消" }))
  })

  it.each(["alive", "expired", "unknown"])(
    "allows a saved binding with cached %s status to reach server validation",
    async (status) => {
      requestJSON.mockResolvedValue({ bound: true, scanEnabled: true, status })
      addPile.mockRejectedValueOnce(new Error("测试验证失败"))
      const user = userEvent.setup()
      render(<AddPileDialog />)
      await user.click(screen.getByRole("button", { name: "添加充电桩" }))
      await user.type(await screen.findByLabelText("桩号"), "61034278")
      await user.click(screen.getByRole("button", { name: "确认添加" }))
      expect(addPile).toHaveBeenCalledWith(
        expect.objectContaining({ number: "61034278" })
      )
      expect(screen.queryByText("添加前，先绑定微信")).not.toBeInTheDocument()
      await user.click(screen.getByRole("button", { name: "取消" }))
    }
  )

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
    await user.click(screen.getByRole("button", { name: "取消" }))
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
