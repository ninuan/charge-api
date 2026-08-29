import { describe, expect, it } from "vitest"

import {
  deliveryPresentationState,
  notificationActionLifecycle,
  notificationPresentation,
  notificationRequiresAction,
} from "@/lib/notification-semantics"

describe("notification semantic contract", () => {
  it.each([
    ["pile_available", "informational", false],
    ["pile_recovered", "informational", false],
    ["credential_expired", "action_required", true],
    ["pile_offline", "action_required", true],
  ] as const)(
    "classifies %s as %s",
    (type, expectedLifecycle, expectedActionable) => {
      expect(notificationActionLifecycle(type)).toBe(expectedLifecycle)
      expect(notificationRequiresAction(type)).toBe(expectedActionable)
    }
  )

  it("turns stored notification data into consistent user-facing copy", () => {
    expect(
      notificationPresentation({
        id: "notice-idle",
        userId: "user-1",
        type: "pile_available",
        severity: "info",
        title: "充电桩有空闲口",
        message: "松园 3 号楼南侧现在有 1 个空闲充电口：2 号。",
        deviceId: "pile-1",
        portId: 2,
        occurrenceCount: 1,
        lastOccurredAt: "2026-08-27T08:00:00Z",
        createdAt: "2026-08-27T08:00:00Z",
      })
    ).toEqual({
      title: "2 号充电口空闲了",
      message: "松园 3 号楼南侧现在有 1 个空闲充电口：2 号。",
    })

    expect(
      notificationPresentation({
        id: "notice-login",
        userId: "user-1",
        type: "credential_expired",
        severity: "warning",
        title: "登录凭据已失效",
        message: "后台刷新已暂停",
        occurrenceCount: 1,
        lastOccurredAt: "2026-08-27T08:00:00Z",
        createdAt: "2026-08-27T08:00:00Z",
      })
    ).toEqual({
      title: "需要重新登录",
      message: "登录已过期，请重新扫码后继续查看和接收提醒。",
    })
  })

  it("keeps an accepted message submitted when provider confirmation is unknown", () => {
    expect(
      deliveryPresentationState({
        id: "delivery-accepted-unknown",
        status: "uncertain",
        userState: "submitted",
        isTest: true,
        acceptedAt: "2026-08-26T13:30:00Z",
        createdAt: "2026-08-26T13:29:58Z",
        updatedAt: "2026-08-26T13:40:00Z",
        message: "消息已提交，但无法确认处理结果",
        errorCode: "ambiguous_result",
      })
    ).toBe("submitted")
  })

  it("does not claim submission when the send outcome itself is unknown", () => {
    expect(
      deliveryPresentationState({
        id: "delivery-send-unknown",
        status: "uncertain",
        userState: "unknown",
        isTest: true,
        createdAt: "2026-08-26T13:29:58Z",
        updatedAt: "2026-08-26T13:30:10Z",
        message: "状态未知",
        errorCode: "ambiguous_result",
      })
    ).toBe("unknown")
  })
})
