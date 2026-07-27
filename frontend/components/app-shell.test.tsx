import { cleanup, render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AppShell } from "@/components/app-shell"

const { authState } = vi.hoisted(() => ({
  authState: {
    currentUser: { username: "alice" } as {
      username: string
      mustChangePassword?: boolean
    } | null,
    isAdmin: false,
    ready: true,
    logout: vi.fn(),
  },
}))

vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: vi.fn() }) }))
vi.mock("@/lib/auth-context", () => ({
  useAuth: () => authState,
}))

describe("AppShell", () => {
  afterEach(() => cleanup())

  it("shows an identity skeleton until authentication is ready", () => {
    authState.currentUser = null
    authState.isAdmin = false
    authState.ready = false

    render(
      <AppShell title="看板" description="说明">
        <p>主要内容</p>
      </AppShell>
    )

    expect(screen.getByTestId("identity-skeleton")).toBeInTheDocument()
    expect(screen.queryByText("普通用户")).not.toBeInTheDocument()
  })

  it("renders the dashboard action area once", () => {
    authState.currentUser = { username: "alice" }
    authState.isAdmin = false
    authState.ready = true
    render(
      <AppShell
        title="看板"
        description="说明"
        actions={<button type="button">使用说明</button>}
      >
        <p>主要内容</p>
      </AppShell>
    )

    expect(screen.getAllByRole("button", { name: "使用说明" })).toHaveLength(1)
    expect(screen.getByRole("link", { name: "进入账户中心" })).toHaveAttribute(
      "href",
      "/account"
    )
  })

  it("prompts a user logged in with a temporary password to change it", () => {
    authState.currentUser = {
      username: "alice",
      mustChangePassword: true,
    }
    authState.isAdmin = false
    authState.ready = true

    render(
      <AppShell title="看板" description="说明">
        <p>主要内容</p>
      </AppShell>
    )

    expect(
      screen.getByText("当前使用的是管理员生成的临时密码")
    ).toBeVisible()
    expect(screen.getByRole("button", { name: "修改密码" })).toBeVisible()
  })

  it("uses a bounded, scrollable account drawer on narrow screens", async () => {
    authState.currentUser = { username: "alice" }
    authState.isAdmin = false
    authState.ready = true
    const user = userEvent.setup()
    render(
      <AppShell
        title="看板"
        description="说明"
        actions={<button type="button">使用说明</button>}
      >
        <p>主要内容</p>
      </AppShell>
    )

    await user.click(screen.getByRole("button", { name: "打开菜单" }))

    const drawer = (await screen.findByText("账户与操作")).closest(
      "[data-slot=sheet-content]"
    )
    expect(drawer).toHaveClass(
      "w-[calc(100vw-1rem)]",
      "max-w-[22rem]",
      "overflow-y-auto"
    )
    expect(
      within(drawer as HTMLElement).getByRole("button", { name: "使用说明" })
    ).toBeVisible()
    expect(within(drawer as HTMLElement).getByText("当前页面")).toBeVisible()
    expect(within(drawer as HTMLElement).getByText("账户操作")).toBeVisible()
    expect(
      within(drawer as HTMLElement).getByRole("button", { name: "退出登录" })
    ).toHaveClass("text-destructive")
  })
})
