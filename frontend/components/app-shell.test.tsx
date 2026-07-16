import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AppShell } from "@/components/app-shell"

vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: vi.fn() }) }))
vi.mock("@/lib/auth-context", () => ({
  useAuth: () => ({
    currentUser: { username: "alice" },
    isAdmin: false,
    logout: vi.fn(),
  }),
}))

describe("AppShell", () => {
  afterEach(() => cleanup())

  it("renders the dashboard action area once", () => {
    render(
      <AppShell title="看板" description="说明" actions={<button type="button">使用说明</button>}>
        <p>主要内容</p>
      </AppShell>
    )

    expect(screen.getAllByRole("button", { name: "使用说明" })).toHaveLength(1)
    expect(screen.getByRole("link", { name: "进入账户中心" })).toHaveAttribute("href", "/account")
  })

  it("uses a bounded, scrollable account drawer on narrow screens", async () => {
    const user = userEvent.setup()
    render(
      <AppShell title="看板" description="说明" actions={<button type="button">使用说明</button>}>
        <p>主要内容</p>
      </AppShell>
    )

    await user.click(screen.getByRole("button", { name: "打开菜单" }))

    const drawer = (await screen.findByText("账户与操作")).closest("[data-slot=sheet-content]")
    expect(drawer).toHaveClass("w-[calc(100vw-1rem)]", "max-w-[22rem]", "overflow-y-auto")
  })
})
