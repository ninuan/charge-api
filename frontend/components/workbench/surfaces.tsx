"use client"

import type { ComponentProps, ReactNode } from "react"
import {
  CircleAlertIcon,
  LoaderCircleIcon,
  SearchIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

export function WorkbenchButton({
  busy = false,
  children,
  className = "",
  ...props
}: ComponentProps<typeof Button> & { busy?: boolean }) {
  const tone =
    props.variant === "destructive"
      ? "danger"
      : props.variant === "ghost" || props.variant === "link"
        ? "quiet"
        : !props.variant || props.variant === "default"
          ? "brand"
          : "default"
  return (
    <Button
      {...props}
      className={`wb-button wb-button-${tone} ${className}`}
      aria-busy={busy || undefined}
      disabled={props.disabled || busy}
      focusableWhenDisabled={busy || props.focusableWhenDisabled}
    >
      <span className="wb-button-content">{children}</span>
      {busy && (
        <LoaderCircleIcon
          aria-hidden="true"
          className="wb-spin wb-button-spinner"
        />
      )}
    </Button>
  )
}

export function PulseDot({
  tone = "idle",
  pulse = false,
}: {
  tone?: "idle" | "busy" | "brand" | "warning" | "danger" | "offline"
  pulse?: boolean
}) {
  return (
    <span className={`wb-pulse-dot wb-pulse-${tone}`} aria-hidden="true">
      {pulse && <span className="wb-pulse-wave" />}
      <span className="wb-pulse-core" />
    </span>
  )
}

export function StatusPill({
  children,
  tone = "neutral",
  pulse = false,
}: {
  children: ReactNode
  tone?:
    | "neutral"
    | "idle"
    | "busy"
    | "offline"
    | "warning"
    | "danger"
    | "brand"
  pulse?: boolean
}) {
  return (
    <span className={`wb-badge wb-tone-${tone}`}>
      {pulse && tone !== "neutral" ? (
        <PulseDot tone={tone} pulse />
      ) : (
        <i className="wb-dot" aria-hidden="true" />
      )}
      {children}
    </span>
  )
}

export function WorkbenchSearch({
  value,
  onChange,
  label = "搜索充电桩",
  placeholder = "名称、位置、桩号或口号",
  shortcut,
}: {
  value: string
  onChange: (value: string) => void
  label?: string
  placeholder?: string
  shortcut?: string
}) {
  return (
    <div className="wb-search-field">
      <SearchIcon aria-hidden="true" />
      <Input
        className="wb-input"
        aria-label={label}
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      {value ? (
        <button
          type="button"
          aria-label="清除搜索"
          onClick={() => onChange("")}
        >
          <XIcon size={14} />
        </button>
      ) : shortcut ? (
        <kbd className="wb-search-kbd" aria-hidden="true">
          {shortcut}
        </kbd>
      ) : null}
    </div>
  )
}

export function WorkbenchEmpty({
  title,
  description,
  icon: Icon = SearchIcon,
  action,
}: {
  title: string
  description: string
  icon?: LucideIcon
  action?: ReactNode
}) {
  return (
    <section className="wb-empty">
      <div className="wb-empty-art" aria-hidden="true">
        <span />
        <Icon size={28} />
        <span />
      </div>
      <h2>{title}</h2>
      <p>{description}</p>
      {action}
    </section>
  )
}

export function StationSkeletonGrid({
  count = 4,
  label = "正在读取充电桩",
}: {
  count?: number
  label?: string
}) {
  return (
    <div className="wb-station-grid" role="status" aria-label={label}>
      {Array.from({ length: count }, (_, idx) => (
        <div key={idx} className="wb-station-card wb-skeleton-card">
          <div className="wb-skel-head">
            <Skeleton className="h-4 w-32 rounded-md" />
            <Skeleton className="h-5 w-16 rounded-full" />
          </div>
          <Skeleton className="h-3 w-44 rounded-md" />
          <div className="wb-skel-bay">
            {[0, 1, 2, 3, 4].map((j) => (
              <Skeleton key={j} className="h-12 w-full rounded-lg" />
            ))}
          </div>
          <div className="wb-skel-foot">
            <Skeleton className="h-3.5 w-28 rounded-md" />
            <Skeleton className="h-3.5 w-16 rounded-md" />
          </div>
        </div>
      ))}
    </div>
  )
}

export function WorkbenchLoading({ label = "正在加载" }: { label?: string }) {
  return (
    <div className="wb-loading" role="status" aria-label={label}>
      <div className="wb-loading-label">
        <LoaderCircleIcon className="wb-spin" size={16} />
        {label}…
      </div>
      <Skeleton className="wb-skeleton wb-skeleton-title" />
      {[0, 1, 2, 3].map((i) => (
        <Skeleton key={i} className="wb-skeleton" />
      ))}
    </div>
  )
}

export function WorkbenchError({
  message,
  retry,
}: {
  message: string
  retry: () => void
}) {
  return (
    <WorkbenchEmpty
      icon={CircleAlertIcon}
      title="暂时没有加载出来"
      description={message}
      action={
        <WorkbenchButton variant="outline" onClick={retry}>
          重新加载
        </WorkbenchButton>
      }
    />
  )
}

export function SectionHeading({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children?: ReactNode
}) {
  return (
    <div className="wb-section-heading">
      <div>
        <h2>{title}</h2>
        {description && <p>{description}</p>}
      </div>
      {children}
    </div>
  )
}
