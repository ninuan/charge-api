import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { AppearanceSettings } from "@/components/appearance-settings"

const setTheme = vi.fn()

vi.mock("next-themes", () => ({
  useTheme: () => ({ theme: "light", setTheme }),
}))

describe("AppearanceSettings", () => {
  afterEach(cleanup)
  beforeEach(() => setTheme.mockClear())

  it("switches the interface to dark mode", async () => {
    const user = userEvent.setup()
    render(<AppearanceSettings />)

    await user.click(screen.getByRole("button", { name: "使用深色模式" }))

    expect(setTheme).toHaveBeenCalledWith("dark")
  })

  it("switches the interface to light mode", async () => {
    const user = userEvent.setup()
    render(<AppearanceSettings />)

    await user.click(screen.getByRole("button", { name: "使用浅色模式" }))

    expect(setTheme).toHaveBeenCalledWith("light")
  })

  it("returns the interface to the system preference", async () => {
    const user = userEvent.setup()
    render(<AppearanceSettings />)

    await user.click(screen.getByRole("button", { name: "跟随系统外观" }))

    expect(setTheme).toHaveBeenCalledWith("system")
  })
})
