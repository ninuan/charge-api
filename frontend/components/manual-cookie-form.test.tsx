import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { expect, it, vi } from "vitest"
import { ManualCookieForm } from "@/components/manual-cookie-form"

const updateCookie = vi.fn()
vi.mock("@/lib/dashboard-context", () => ({
  useDashboard: () => ({ updateCookie }),
}))
it("keeps manual credentials on failure and clears them only after saving", async () => {
  const user = userEvent.setup(),
    saved = vi.fn()
  updateCookie
    .mockRejectedValueOnce(new Error("更新失败，请重试。"))
    .mockResolvedValueOnce({})
  render(<ManualCookieForm onSaved={saved} />)
  const input = screen.getByLabelText("手动更新 Cookie")
  await user.type(input, "test-cookie")
  await user.click(screen.getByRole("button", { name: "更新 Cookie" }))
  expect(await screen.findByText("更新失败，请重试。")).toBeVisible()
  expect(input).toHaveValue("test-cookie")
  expect(saved).not.toHaveBeenCalled()
  await user.click(screen.getByRole("button", { name: "更新 Cookie" }))
  expect(await screen.findByText("Cookie 已更新")).toBeVisible()
  expect(input).toHaveValue("")
  expect(saved).toHaveBeenCalledTimes(1)
})
