import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { AdminUserFilters } from "@/components/admin-user-filters"

describe("AdminUserFilters", () => {
  it("applies account, credential, and health filters together", async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    render(<AdminUserFilters query={{ page: 1, pageSize: 15, search: "", account: "all", credential: "all", health: "all" }} onApply={onApply} />)

    await user.type(screen.getByLabelText("搜索用户"), "wang")
    await user.selectOptions(screen.getByLabelText("账户状态"), "enabled")
    await user.selectOptions(screen.getByLabelText("凭据状态"), "expired")
    await user.selectOptions(screen.getByLabelText("设备健康"), "risk")
    await user.click(screen.getByRole("button", { name: "应用筛选" }))

    expect(onApply).toHaveBeenCalledWith({ page: 1, pageSize: 15, search: "wang", account: "enabled", credential: "expired", health: "risk" })
  })
})
