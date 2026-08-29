"use client"

import type { ReactNode } from "react"

export type FeedbackBannerSeverity = "info" | "warning" | "critical"

export type FeedbackBannerAction = {
  label: ReactNode
  onClick: () => void
}

export type FeedbackBanner = {
  id: string
  title: string
  description: ReactNode
  severity: FeedbackBannerSeverity
  action?: FeedbackBannerAction
  details?: ReactNode
  occurrenceCount: number
  lastOccurredAt: number
}

type FeedbackBannerInput = Omit<
  FeedbackBanner,
  "occurrenceCount" | "lastOccurredAt"
>

const listeners = new Set<() => void>()
let currentBanner: FeedbackBanner | null = null

const severityRank: Record<FeedbackBannerSeverity, number> = {
  info: 1,
  warning: 2,
  critical: 3,
}

function emit() {
  for (const listener of listeners) listener()
}

export function showFeedbackBanner(input: FeedbackBannerInput) {
  const now = Date.now()
  if (currentBanner?.id === input.id) {
    currentBanner = {
      ...currentBanner,
      ...input,
      occurrenceCount: currentBanner.occurrenceCount + 1,
      lastOccurredAt: now,
    }
    emit()
    return input.id
  }

  if (
    currentBanner &&
    severityRank[currentBanner.severity] > severityRank[input.severity]
  ) {
    return currentBanner.id
  }

  currentBanner = {
    ...input,
    occurrenceCount: 1,
    lastOccurredAt: now,
  }
  emit()
  return input.id
}

export function dismissFeedbackBanner(id?: string) {
  if (!currentBanner || (id && currentBanner.id !== id)) return
  currentBanner = null
  emit()
}

export function subscribeFeedbackBanner(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function getFeedbackBanner() {
  return currentBanner
}

export function getFeedbackBannerServerSnapshot() {
  return null
}
