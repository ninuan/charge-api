"use client"

import type { ReactNode } from "react"
import { toast, type ExternalToast } from "sonner"

import {
  dismissFeedbackBanner,
  showFeedbackBanner,
  type FeedbackBannerSeverity,
} from "@/lib/feedback-banner"

type FeedbackOptions = Omit<ExternalToast, "description"> & {
  description?: ReactNode
}

type ErrorFeedbackOptions = FeedbackOptions & {
  fallback?: string
  title?: string
  persistent?: boolean
  bannerId?: string
  details?: ReactNode
}

const fallbackErrorMessage = "操作没有完成，请稍后重试"

export function feedbackMessage(
  reason: unknown,
  fallback = fallbackErrorMessage
) {
  if (reason instanceof Error && reason.message.trim()) return reason.message
  if (typeof reason === "string" && reason.trim()) return reason
  return fallback
}

function withDuration(
  title: string,
  options: FeedbackOptions,
  duration: number
) {
  const hasAction = Boolean(options.action || options.cancel)
  return {
    duration: hasAction ? Math.max(duration, 10_000) : duration,
    ...options,
    description:
      typeof options.description === "string" &&
      options.description.trim() === title.trim()
        ? undefined
        : options.description,
  }
}

export const notify = {
  success(title: string, options: FeedbackOptions = {}) {
    return toast.success(title, withDuration(title, options, 3_500))
  },
  info(title: string, options: FeedbackOptions = {}) {
    return toast.info(title, withDuration(title, options, 4_500))
  },
  warning(title: string, options: FeedbackOptions = {}) {
    return toast.warning(title, withDuration(title, options, 5_500))
  },
  error(reason: unknown, options: ErrorFeedbackOptions = {}) {
    const {
      fallback,
      title,
      persistent,
      bannerId,
      details,
      ...toastOptions
    } = options
    const message = feedbackMessage(reason, fallback)
    if (persistent) {
      const bannerAction =
        toastOptions.action &&
        typeof toastOptions.action === "object" &&
        "label" in toastOptions.action &&
        "onClick" in toastOptions.action
          ? toastOptions.action
          : undefined
      return showFeedbackBanner({
        id: bannerId ?? `error:${title ?? message}`,
        title: title ?? "操作没有完成",
        description:
          toastOptions.description ??
          (title && title !== message ? message : "请稍后重试。"),
        severity: "critical",
        action: bannerAction
          ? {
              label: bannerAction.label,
              onClick: () => bannerAction.onClick(undefined as never),
            }
          : undefined,
        details,
      })
    }
    return toast.error(title ?? message, {
      ...withDuration(title ?? message, toastOptions, 6_500),
      description:
        toastOptions.description ??
        (title && title !== message ? message : undefined),
    })
  },
  dismiss(id?: string | number) {
    return toast.dismiss(id)
  },
  banner({
    id,
    title,
    description,
    severity = "info",
    action,
    details,
  }: {
    id: string
    title: string
    description: ReactNode
    severity?: FeedbackBannerSeverity
    action?: { label: ReactNode; onClick: () => void }
    details?: ReactNode
  }) {
    return showFeedbackBanner({
      id,
      title,
      description,
      severity,
      action,
      details,
    })
  },
  dismissBanner(id?: string) {
    dismissFeedbackBanner(id)
  },
}
