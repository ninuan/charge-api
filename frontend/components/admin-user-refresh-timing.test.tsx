import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { AdminUserRefreshTiming } from "@/components/admin-user-refresh-timing"

describe("AdminUserRefreshTiming", () => {
  it("distinguishes credential checks from device refreshes", () => {
    render(<AdminUserRefreshTiming bound lastCheckedAt="2026-07-16T08:00:00Z" lastRemoteAt="2026-07-16T08:01:00Z" />)

    expect(screen.getByText(/凭据最近检查/)).toBeVisible()
    expect(screen.getByText(/设备最近刷新/)).toBeVisible()
  })
})
