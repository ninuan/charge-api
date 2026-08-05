import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminOperations } from "@/components/admin-operations"

const { adminApiMock } = vi.hoisted(() => ({
  adminApiMock: {
    operations: vi.fn(),
    audit: vi.fn(),
  },
}))

vi.mock("@/lib/admin-api", () => ({ adminApi: adminApiMock }))

describe("AdminOperations", () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("shows port history volume, retention, and time bounds", async () => {
    adminApiMock.operations.mockResolvedValue({
      databaseSizeBytes: 524288,
      metricRows: 42,
      metricRetentionDays: 30,
      portHistoryRows: 128,
      portHistoryRetentionDays: 90,
      portHistoryOldestAt: "2026-07-01T01:00:00Z",
      portHistoryNewestAt: "2026-08-05T09:30:00Z",
      integrityResult: "ok",
      checkedAt: "2026-08-05T10:00:00Z",
      lastBackupAt: "2026-08-05T08:00:00Z",
      lastBackupSizeBytes: 1024,
      backupState: "healthy",
      backupMessage: "已发现最近数据库备份。",
    })
    adminApiMock.audit.mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 20,
      total: 0,
      totalPages: 1,
    })

    render(<AdminOperations />)

    expect(await screen.findByText("端口历史")).toBeInTheDocument()
    expect(screen.getByText("128 条")).toBeInTheDocument()
    expect(screen.getByText(/保留 90 天/)).toBeInTheDocument()
    expect(screen.getByText(/07\/01/)).toBeInTheDocument()
    expect(screen.getByText(/08\/05/)).toBeInTheDocument()
  })
})
