"use client"

import type { ComponentProps, ReactNode } from "react"
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CircleAlert,
  LoaderCircle,
  Search,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { usePrototype } from "./context"

export function PButton({
  children,
  className = "",
  tone = "default",
  loading = false,
  ...props
}: ComponentProps<typeof Button> & {
  tone?: "default" | "brand" | "danger" | "quiet"
  loading?: boolean
}) {
  return (
    <Button
      variant={tone === "quiet" ? "ghost" : "outline"}
      className={`cp-button cp-button-${tone} ${className}`}
      {...props}
      disabled={props.disabled || loading}
      focusableWhenDisabled={loading || props.focusableWhenDisabled}
      aria-busy={loading || undefined}
    >
      <span className="cp-button-content">{children}</span>
      {loading && (
        <LoaderCircle
          className="cp-spin cp-button-spinner"
          aria-hidden="true"
        />
      )}
    </Button>
  )
}
export function IconButton({
  icon: Icon,
  label,
  ...props
}: Omit<ComponentProps<typeof PButton>, "children"> & {
  icon: LucideIcon
  label: string
}) {
  return (
    <PButton
      tone="quiet"
      {...props}
      className={`cp-icon-button ${props.className ?? ""}`}
      aria-label={label}
      title={label}
    >
      <Icon aria-hidden="true" />
    </PButton>
  )
}
export function Badge({
  children,
  tone = "neutral",
  dot = true,
}: {
  children: ReactNode
  tone?:
    "neutral" | "idle" | "busy" | "offline" | "warning" | "danger" | "brand"
  dot?: boolean
}) {
  return (
    <span className={`cp-badge cp-tone-${tone}`}>
      {dot && <span className="cp-dot" />}
      {children}
    </span>
  )
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <label className="cp-field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  )
}
export function PInput(props: ComponentProps<typeof Input>) {
  return <Input {...props} className={`cp-input ${props.className ?? ""}`} />
}
export function SearchField({
  value,
  onChange,
  placeholder = "搜索名称、位置或桩号",
  label = "搜索充电桩",
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  label?: string
}) {
  return (
    <div className="cp-search-field">
      <Search aria-hidden="true" />
      <PInput
        aria-label={label}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {value && (
        <button
          type="button"
          aria-label="清除搜索"
          onClick={() => onChange("")}
        >
          <X size={14} />
        </button>
      )}
    </div>
  )
}
export function SettingRow({
  title,
  description,
  children,
}: {
  title: string
  description?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="cp-setting-row">
      <div>
        <h3>{title}</h3>
        {description && <p>{description}</p>}
      </div>
      <div className="cp-setting-action">{children}</div>
    </div>
  )
}
export function Toggle({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean
  onChange: (value: boolean) => void
  label: string
  disabled?: boolean
}) {
  return (
    <Switch
      className="cp-switch"
      checked={checked}
      onCheckedChange={onChange}
      aria-label={label}
      disabled={disabled}
    />
  )
}
export function Modal({
  title,
  description,
  children,
  wide = false,
}: {
  title: string
  description?: string
  children: ReactNode
  wide?: boolean
}) {
  const { modal, openModal, theme, data } = usePrototype()
  return (
    <Dialog open={!!modal} onOpenChange={(open) => !open && openModal(null)}>
      <DialogContent
        className={`cp-theme cp-dialog ${wide ? "cp-dialog-wide" : ""}`}
        data-theme={theme}
        data-reduced={data.reduceMotion || undefined}
        showCloseButton={false}
      >
        <DialogHeader className="cp-dialog-header">
          <DialogTitle className="cp-dialog-title">{title}</DialogTitle>
          <IconButton
            label="关闭对话框"
            icon={X}
            onClick={() => openModal(null)}
          />
          {description && (
            <DialogDescription className="cp-dialog-description">
              {description}
            </DialogDescription>
          )}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  )
}
export function ModalActions({ children }: { children: ReactNode }) {
  const { openModal, busy } = usePrototype()
  return (
    <div className="cp-modal-actions">
      <PButton tone="quiet" onClick={() => openModal(null)} disabled={!!busy}>
        取消
      </PButton>
      {children}
    </div>
  )
}
export function PageHeading({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow?: string
  title: string
  description?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="cp-page-heading">
      <div>
        {eyebrow && <div className="cp-eyebrow">{eyebrow}</div>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="cp-heading-actions">{actions}</div>}
    </header>
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
    <div className="cp-section-heading">
      <div>
        <h2>{title}</h2>
        {description && <p>{description}</p>}
      </div>
      {children}
    </div>
  )
}
export function EmptyState({
  icon: Icon = Search,
  title,
  description,
  action,
}: {
  icon?: LucideIcon
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <div className="cp-empty">
      <div className="cp-empty-art" aria-hidden="true">
        <span />
        <Icon size={27} />
        <span />
      </div>
      <h2>{title}</h2>
      <p>{description}</p>
      {action}
    </div>
  )
}
export function LoadingState({ label = "正在读取充电桩" }: { label?: string }) {
  return (
    <div className="cp-loading" role="status" aria-label={label}>
      <div className="cp-loading-label">
        <LoaderCircle size={16} className="cp-spin" />
        {label}…
      </div>
      <Skeleton className="cp-skeleton cp-skeleton-title" />
      {[0, 1, 2, 3, 4].map((i) => (
        <Skeleton key={i} className="cp-skeleton" />
      ))}
    </div>
  )
}
export function ErrorState({
  title = "暂时没有加载出来",
  description = "上一次保存的内容还在。请检查连接后再试一次。",
}: {
  title?: string
  description?: string
}) {
  const { setScenario, notify } = usePrototype()
  return (
    <EmptyState
      icon={CircleAlert}
      title={title}
      description={description}
      action={
        <PButton
          onClick={() => {
            setScenario("ready")
            notify("已重新加载示例数据。")
          }}
        >
          重新加载
          <ArrowRight size={15} />
        </PButton>
      }
    />
  )
}
export function FormError({ message }: { message: string }) {
  return message ? (
    <p className="cp-form-error" role="alert">
      <CircleAlert size={15} />
      {message}
    </p>
  ) : null
}
export function BackLink({
  children = "返回",
  onClick,
}: {
  children?: ReactNode
  onClick: () => void
}) {
  return (
    <button className="cp-back" onClick={onClick}>
      <ArrowLeft size={15} />
      {children}
    </button>
  )
}
export function Brand({ small = false }: { small?: boolean }) {
  return (
    <div className={`cp-brand ${small ? "cp-brand-small" : ""}`}>
      <span className="cp-brand-mark">
        <Zap
          strokeWidth={2.7}
          fill="currentColor"
          size={20}
          aria-hidden="true"
        />
      </span>
      <span>
        charge<span className="cp-brand-period">.</span>
      </span>
    </div>
  )
}
export function SaveButton({
  children = "保存更改",
}: {
  children?: ReactNode
}) {
  const { busy, offline } = usePrototype()
  return (
    <PButton type="submit" tone="brand" loading={!!busy} disabled={offline}>
      <Check size={15} />
      {children}
    </PButton>
  )
}
