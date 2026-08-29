import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { Alert } from "@/components/ui/alert"

describe("Alert", () => {
  it("announces ordinary status updates politely", () => {
    render(<Alert>设置已保存</Alert>)

    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite")
  })

  it("uses an assertive alert only for urgent failures", () => {
    render(<Alert urgent>无法继续加载</Alert>)

    expect(screen.getByRole("alert")).toHaveAttribute(
      "aria-live",
      "assertive"
    )
  })
})
