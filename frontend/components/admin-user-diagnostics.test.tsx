import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it } from "vitest"

import { AdminUserDiagnostics } from "@/components/admin-user-diagnostics"

afterEach(cleanup)

describe("AdminUserDiagnostics", () => {
  it("expands a user's sanitized diagnostics", async () => {
    const user = userEvent.setup()
    render(
      <AdminUserDiagnostics
        diagnostics={[
          {
            operation: "refresh",
            code: "refresh_failed",
            message: "刷新设备状态失败，请稍后重试",
            deviceSuffix: "0001",
            statusCode: 502,
            at: "2026-07-16T08:00:00Z",
          },
        ]}
      />
    )

    await user.click(screen.getByRole("button", { name: "查看诊断（1）" }))

    expect(screen.getByText("刷新设备状态失败，请稍后重试")).toBeVisible()
    expect(
      screen.getByText(/刷新状态.*设备尾号 0001.*状态码 502/)
    ).toBeVisible()
  })

  it("does not render successful recovery events as failures", () => {
    const { container } = render(
      <AdminUserDiagnostics
        diagnostics={[
          {
            operation: "credential_recovery",
            code: "recovery_succeeded",
            message: "登录凭据已自动恢复并校验成功",
            at: "2026-07-16T08:00:00Z",
          },
        ]}
      />
    )

    expect(container).toBeEmptyDOMElement()
  })
})

it.each([
  ["yyb_account_refresh_expired", "扫码服务未能恢复登录状态，请重新扫码"],
  ["yyb_account_refresh_unknown", "扫码服务缺少账号恢复信息，请重新扫码"],
])(
  "shows the account refresh reason between initial and final failures: %s",
  async (code, message) => {
    const user = userEvent.setup()
    render(
      <AdminUserDiagnostics
        diagnostics={[
          {
            operation: "credential_recovery",
            code: "yyb_get_code_failed",
            message: "临时凭据获取失败",
            at: "2026-09-14T14:16:44Z",
          },
          {
            operation: "credential_recovery",
            code,
            message,
            at: "2026-09-14T14:16:45Z",
          },
          {
            operation: "sync_cookie",
            code: "credential_sync_failed",
            message: "同步失败",
            at: "2026-09-14T14:16:46Z",
          },
        ]}
      />
    )
    await user.click(screen.getByRole("button", { name: "查看诊断（3）" }))
    expect(screen.getByText(message)).toBeVisible()
    expect(screen.getAllByRole("listitem")).toHaveLength(3)
  }
)
