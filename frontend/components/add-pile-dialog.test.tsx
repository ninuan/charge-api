import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { AddPileDialog } from "@/components/add-pile-dialog"

const addPile = vi.fn()

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
    await user.type(screen.getByLabelText("桩号"), "invalid")
    await user.click(screen.getByRole("button", { name: "确认添加" }))

    expect(addPile).toHaveBeenCalledWith(expect.objectContaining({ number: "invalid" }))
  })
})
