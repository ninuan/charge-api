import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"
import { useState } from "react"

import { AppShell, useCloseAppShellMenu } from "@/components/app-shell"
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"

const { authState, routerMock } = vi.hoisted(() => ({
  routerMock: { replace: vi.fn(), push: vi.fn() },
  authState: {
    currentUser: { username: "alice" } as {
      username: string
      mustChangePassword?: boolean
    } | null,
    isAdmin: false,
    ready: true,
    logout: vi.fn(),
    clearSession: vi.fn(),
  },
}))

vi.mock("next/navigation", () => ({ useRouter: () => routerMock }))
vi.mock("@/lib/auth-context", () => ({
  useAuth: () => authState,
}))

function DialogAction() {
  const [open, setOpen] = useState(false)
  const closeMenu = useCloseAppShellMenu()
  const handleOpenChange = (next: boolean) => {
    setOpen(next)
    if (!next) closeMenu()
  }
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<button type="button" />}>使用说明</DialogTrigger>
      <DialogContent>
        <DialogTitle>说明内容</DialogTitle>
      </DialogContent>
    </Dialog>
  )
}

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
        notificationAction={<button type="button">通知</button>}
      >
        <p>主要内容</p>
      </AppShell>
    )

    expect(screen.getAllByRole("button", { name: "使用说明" })).toHaveLength(1)
    expect(screen.getAllByRole("button", { name: "通知" })).toHaveLength(1)
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

    expect(screen.getByText("当前使用的是管理员生成的临时密码")).toBeVisible()
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
      within(drawer as HTMLElement).getByRole("link", { name: "空闲提醒" })
    ).toHaveAttribute("href", "/dashboard/reminders")
    expect(
      within(drawer as HTMLElement).getByRole("link", { name: "通知" })
    ).toHaveAttribute("href", "/dashboard/notifications")
    expect(
      within(drawer as HTMLElement).getByRole("link", { name: "账户安全" })
    ).toHaveAttribute("href", "/account?tab=security")
    expect(
      screen.queryByRole("button", { name: "退出登录" })
    ).not.toBeInTheDocument()

    await user.keyboard("{Escape}")
    await waitFor(() => expect(drawer).not.toBeVisible())
  })

  it("does not duplicate page actions inside navigation and restores their focus", async () => {
    authState.currentUser = { username: "alice" }
    authState.isAdmin = false
    authState.ready = true
    const user = userEvent.setup()
    render(
      <AppShell title="看板" description="说明" actions={<DialogAction />}>
        <p>主要内容</p>
      </AppShell>
    )

    await user.click(screen.getByRole("button", { name: "打开菜单" }))
    const drawer = screen
      .getByText("账户与操作")
      .closest("[data-slot=sheet-content]")
    expect(
      within(drawer as HTMLElement).queryByRole("button", { name: "使用说明" })
    ).not.toBeInTheDocument()
    expect(
      screen.getAllByRole("button", { name: "使用说明", hidden: true })
    ).toHaveLength(1)
    await user.keyboard("{Escape}")
    await waitFor(() => expect(drawer).not.toBeVisible())
    const trigger = screen.getByRole("button", { name: "使用说明" })
    await user.click(trigger)

    expect(screen.getByRole("dialog", { name: "说明内容" })).toHaveTextContent(
      "说明内容"
    )

    await user.click(
      within(screen.getByRole("dialog", { name: "说明内容" })).getByRole(
        "button",
        { name: "关闭" }
      )
    )
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "说明内容" })
      ).not.toBeInTheDocument()
    )
    await waitFor(() => expect(trigger).toHaveFocus())
  })

  it("uses the current workspace navigation and sends administrator searches to the user directory", async () => {
    authState.currentUser = { username: "admin" }
    authState.isAdmin = true
    authState.ready = true
    const user = userEvent.setup()
    render(
      <AppShell title="运行概览" activeSection="overview" description="说明">
        <p>内容</p>
      </AppShell>
    )
    const nav = screen.getByRole("navigation", { name: "主导航" })
    expect(
      within(nav).queryByRole("link", { name: "常用充电桩" })
    ).not.toBeInTheDocument()
    expect(within(nav).getByRole("link", { name: "运行概览" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    await user.click(screen.getByRole("button", { name: "打开快速搜索" }))
    await user.type(screen.getByRole("textbox", { name: "快速搜索" }), "alice")
    await user.click(screen.getByRole("button", { name: /查找用户“alice”/ }))
    expect(routerMock.push).toHaveBeenCalledWith(
      "/admin?tab=users&search=alice"
    )
  })
})
