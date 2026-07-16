import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { AdminUserDiagnostics } from "@/components/admin-user-diagnostics"

describe("AdminUserDiagnostics", () => {
  it("expands a user's sanitized diagnostics", async () => {
    const user = userEvent.setup()
    render(<AdminUserDiagnostics diagnostics={[{
      operation: "refresh",
      code: "refresh_failed",
      message: "刷新设备状态失败，请稍后重试",
      deviceSuffix: "0001",
      statusCode: 502,
      at: "2026-07-16T08:00:00Z",
    }]} />)

    await user.click(screen.getByRole("button", { name: "查看诊断（1）" }))

    expect(screen.getByText("刷新设备状态失败，请稍后重试")).toBeVisible()
    expect(screen.getByText(/刷新状态.*设备尾号 0001.*状态码 502/)).toBeVisible()
  })

  it("does not render successful recovery events as failures", () => {
    const { container } = render(<AdminUserDiagnostics diagnostics={[{
      operation: "credential_recovery",
      code: "recovery_succeeded",
      message: "登录凭据已自动恢复并校验成功",
      at: "2026-07-16T08:00:00Z",
    }]} />)

    expect(container).toBeEmptyDOMElement()
  })
})
