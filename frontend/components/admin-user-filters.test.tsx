import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminUserFilters } from "@/components/admin-user-filters"

describe("AdminUserFilters", () => {
  afterEach(cleanup)


  it("applies account, credential, and health filters together", async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    render(
      <AdminUserFilters
        query={{
          page: 1,
          pageSize: 15,
          search: "",
          account: "all",
          credential: "all",
          health: "all",
        }}
        onApply={onApply}
      />
    )

    await user.type(screen.getByLabelText("搜索用户"), "wang")
    await user.click(screen.getByRole("combobox", { name: "账户状态" }))
    await user.click(await screen.findByRole("option", { name: "已启用" }))
    await user.click(screen.getByRole("combobox", { name: "凭据状态" }))
    await user.click(await screen.findByRole("option", { name: "凭据已失效" }))
    await user.click(screen.getByRole("combobox", { name: "设备健康" }))
    await user.click(await screen.findByRole("option", { name: "存在风险" }))
    await user.click(screen.getByRole("button", { name: "应用筛选" }))

    expect(onApply).toHaveBeenCalledWith({
      page: 1,
      pageSize: 15,
      search: "wang",
      account: "enabled",
      credential: "expired",
      health: "risk",
    })
  })

  it("applies the filters when pressing Enter in the search box", async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    render(
      <AdminUserFilters
        query={{
          page: 3,
          pageSize: 15,
          search: "",
          account: "all",
          credential: "all",
          health: "all",
        }}
        onApply={onApply}
      />
    )

    await user.type(screen.getByLabelText("搜索用户"), "wang{Enter}")

    expect(onApply).toHaveBeenCalledWith(
      expect.objectContaining({ search: "wang", page: 1 })
    )
  })
})
