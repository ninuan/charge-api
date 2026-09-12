import { cleanup, render, screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"
import { AuthPageShell } from "@/components/auth-page-shell"

describe("AuthPageShell", () => {
  afterEach(cleanup)
  it("keeps the real form in a dedicated main landmark alongside the product introduction", () => {
    render(
      <AuthPageShell mode="login">
        <div>login fields</div>
      </AuthPageShell>
    )
    expect(screen.getByRole("main")).toHaveClass("wb-auth-form-section")
    expect(
      within(screen.getByRole("main")).getByText("login fields")
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { level: 1, name: "欢迎回来" })
    ).toBeInTheDocument()
    expect(
      within(screen.getByRole("navigation", { name: "认证方式" })).getByRole(
        "link",
        { name: "登录" }
      )
    ).toHaveAttribute("aria-current", "page")
  })
  it("marks registration as the active route without changing form semantics", () => {
    render(
      <AuthPageShell mode="register">
        <form aria-label="注册表单" />
      </AuthPageShell>
    )
    expect(
      screen.getByRole("heading", { level: 1, name: "创建你的账户" })
    ).toBeInTheDocument()
    expect(screen.getByRole("form", { name: "注册表单" })).toBeInTheDocument()
    expect(
      within(screen.getByRole("navigation", { name: "认证方式" })).getByRole(
        "link",
        { name: "注册" }
      )
    ).toHaveAttribute("aria-current", "page")
  })
})
