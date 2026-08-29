"use client"

import { AlertTriangleIcon, InfoIcon, XIcon } from "lucide-react"
import { useSyncExternalStore } from "react"

import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  dismissFeedbackBanner,
  getFeedbackBanner,
  getFeedbackBannerServerSnapshot,
  subscribeFeedbackBanner,
} from "@/lib/feedback-banner"
import { cn } from "@/lib/utils"

export function GlobalFeedbackBanner() {
  const banner = useSyncExternalStore(
    subscribeFeedbackBanner,
    getFeedbackBanner,
    getFeedbackBannerServerSnapshot
  )

  if (!banner) return null

  const urgent = banner.severity === "critical"
  const Icon = urgent ? AlertTriangleIcon : InfoIcon

  return (
    <Alert
      urgent={urgent}
      variant={urgent ? "destructive" : "default"}
      className={cn(
        "feedback-banner-transition mb-4 pr-32",
        banner.severity === "warning" && "border-warning/40 bg-warning/10"
      )}
    >
      <Icon />
      <AlertTitle>{banner.title}</AlertTitle>
      <AlertDescription className="space-y-1">
        <div>{banner.description}</div>
        {banner.occurrenceCount > 1 ? (
          <div className="text-xs">
            已重复出现 {banner.occurrenceCount} 次，最近一次：
            {new Date(banner.lastOccurredAt).toLocaleTimeString("zh-CN", {
              hour: "2-digit",
              minute: "2-digit",
            })}
          </div>
        ) : null}
        {banner.details ? (
          <details className="text-xs">
            <summary className="cursor-pointer">查看详情</summary>
            <div className="mt-1 break-words">{banner.details}</div>
          </details>
        ) : null}
      </AlertDescription>
      <AlertAction className="flex gap-1">
        {banner.action ? (
          <Button size="sm" variant="outline" onClick={banner.action.onClick}>
            {banner.action.label}
          </Button>
        ) : null}
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label="关闭提示"
          onClick={() => dismissFeedbackBanner(banner.id)}
        >
          <XIcon />
        </Button>
      </AlertAction>
    </Alert>
  )
}
