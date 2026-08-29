import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  dismissFeedbackBanner,
  getFeedbackBanner,
  showFeedbackBanner,
  subscribeFeedbackBanner,
} from "@/lib/feedback-banner"

describe("feedback banner store", () => {
  beforeEach(() => dismissFeedbackBanner())

  it("keeps one highest-priority banner and merges repeated occurrences", () => {
    const listener = vi.fn()
    const unsubscribe = subscribeFeedbackBanner(listener)

    showFeedbackBanner({
      id: "load-error",
      title: "加载失败",
      description: "请重试",
      severity: "critical",
    })
    showFeedbackBanner({
      id: "load-error",
      title: "加载失败",
      description: "请重试",
      severity: "critical",
    })
    showFeedbackBanner({
      id: "quiet-warning",
      title: "提醒暂停",
      description: "稍后恢复",
      severity: "warning",
    })

    expect(getFeedbackBanner()).toMatchObject({
      id: "load-error",
      occurrenceCount: 2,
    })
    expect(listener).toHaveBeenCalledTimes(2)
    unsubscribe()
  })

  it("allows a more important banner to replace the current one", () => {
    showFeedbackBanner({
      id: "notice",
      title: "状态提示",
      description: "可以稍后处理",
      severity: "info",
    })
    showFeedbackBanner({
      id: "failure",
      title: "加载失败",
      description: "请重试",
      severity: "critical",
    })

    expect(getFeedbackBanner()?.id).toBe("failure")
  })
})
