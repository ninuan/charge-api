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

export function StatusPill({
  children,
  tone = "neutral",
}: {
  children: ReactNode
  tone?:
    "neutral" | "idle" | "busy" | "offline" | "warning" | "danger" | "brand"
}) {
  return (
    <span className={`wb-badge wb-tone-${tone}`}>
      <i className="wb-dot" aria-hidden="true" />
      {children}
    </span>
  )
}

export function WorkbenchSearch({
  value,
  onChange,
  label = "搜索充电桩",
  placeholder = "名称、位置、桩号或口号",
}: {
  value: string
  onChange: (value: string) => void
  label?: string
  placeholder?: string
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
      {value && (
        <button
          type="button"
          aria-label="清除搜索"
          onClick={() => onChange("")}
        >
          <XIcon size={14} />
        </button>
      )}
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
