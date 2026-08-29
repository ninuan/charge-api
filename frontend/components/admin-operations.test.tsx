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
      notificationRows: 36,
      resolvedNotificationRows: 12,
      notificationRetentionDays: 90,
      reminders: {
        state: "healthy",
        message: "后台提醒调度运行正常。",
        enabled: true,
        schedulerRunning: true,
        scheduledPowerOffActive: false,
        activeTemporaryRules: 3,
        completedNotified24Hours: 5,
        completedExpired24Hours: 2,
        averageTemporaryMinutes: 78,
        trackedPiles: 4,
        duePiles: 1,
        inFlightPiles: 0,
        nextAttemptAt: "2026-08-05T10:10:00Z",
        remoteAttempts24Hours: 20,
        remoteSuccesses24Hours: 19,
        remoteFailures24Hours: 1,
        remoteSuccessRate24Hours: 95,
        cacheHits24Hours: 8,
        coalesced24Hours: 2,
        quotaSkips24Hours: 0,
        schedulerErrors24Hours: 0,
        maxConsecutiveFailures: 0,
      },
      wxPusher: {
        state: "healthy",
        message: "通道运行正常；1 位用户的接收绑定需要单独处理。",
        configured: true,
        dispatcherRunning: true,
        activeBindings: 6,
        pendingDeliveries: 1,
        sendingDeliveries: 0,
        acceptedPendingDeliveries: 1,
        retryingDeliveries: 0,
        uncertainDeliveries: 1,
        failedDeliveries: 2,
        oldestPendingAt: "2026-08-05T09:55:00Z",
        attempts24Hours: 10,
        accepted24Hours: 9,
        providerSucceeded24Hours: 8,
        acceptanceRate24Hours: 90,
        providerSuccessRate24Hours: 88.9,
        systemFailures24Hours: 1,
        bindingFailures24Hours: 2,
        affectedBindingUsers: 1,
        consecutiveSystemFailures: 0,
        lastAcceptedAt: "2026-08-05T09:50:00Z",
        lastProviderSuccessAt: "2026-08-05T09:52:00Z",
        lastFailureAt: "2026-08-05T09:45:00Z",
        lastErrorCategory: "binding_invalid",
      },
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
    expect(screen.getAllByText(/保留 90 天/)).toHaveLength(2)
    expect(screen.getByText(/07\/01.*08\/05/)).toBeInTheDocument()
    expect(screen.getByText("提醒调度")).toBeInTheDocument()
    expect(screen.getByText("活动提醒")).toBeInTheDocument()
    expect(screen.getByText("3 条")).toBeInTheDocument()
    expect(screen.getByText("95.0%")).toBeInTheDocument()
    expect(screen.getByText("WxPusher 投递")).toBeInTheDocument()
    expect(screen.getByText("90.0%")).toBeInTheDocument()
    expect(screen.getByText(/绑定异常 2 次/)).toBeInTheDocument()
    expect(screen.getByText("36 条")).toBeInTheDocument()
  })
})
