"use client"

import { ChevronDownIcon, SlidersHorizontalIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { AdminUserListQuery } from "@/lib/types"
import { cn } from "@/lib/utils"

const accountOptions = [
  { value: "all", label: "全部账户" },
  { value: "enabled", label: "已启用" },
  { value: "disabled", label: "已停用" },
]

const credentialOptions = [
  { value: "all", label: "全部凭据" },
  { value: "unbound", label: "未绑定扫码" },
  { value: "waiting_device", label: "等待添加设备" },
  { value: "healthy", label: "凭据正常" },
  { value: "sync_failed", label: "同步失败" },
  { value: "expired", label: "凭据已失效" },
]

const healthOptions = [
  { value: "all", label: "全部状态" },
  { value: "healthy", label: "状态正常" },
  { value: "risk", label: "存在风险" },
]

export function AdminUserFilters({
  query,
  onApply,
}: {
  query: AdminUserListQuery
  onApply: (query: AdminUserListQuery) => void
}) {
  const [draft, setDraft] = useState(query)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const advancedCount = [
    draft.account !== "all",
    draft.credential !== "all",
    draft.health !== "all",
  ].filter(Boolean).length

  function update<Key extends keyof AdminUserListQuery>(
    key: Key,
    value: AdminUserListQuery[Key]
  ) {
    setDraft((current) => ({ ...current, [key]: value }))
  }

  function apply(event?: React.FormEvent) {
    event?.preventDefault()
    onApply({ ...draft, search: draft.search.trim(), page: 1 })
  }

  function reset() {
    const next = {
      page: 1,
      pageSize: query.pageSize,
      search: "",
      account: "all",
      credential: "all",
      health: "all",
    } as AdminUserListQuery
    setDraft(next)
    onApply(next)
  }

  return (
    <section
      className="rounded-xl border bg-card p-4 shadow-xs"
      aria-label="用户筛选"
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">筛选用户</h2>
          <p className="mt-1 hidden text-xs leading-5 text-muted-foreground sm:block">
            组合账户状态、扫码凭据和设备健康度，快速定位需要处理的账户。
          </p>
        </div>
        <Button variant="ghost" size="sm" onClick={reset}>
          重置筛选
        </Button>
      </div>
      <form
        className="mt-3 grid gap-3 md:grid-cols-[minmax(12rem,1fr)_10rem_11rem_9rem_auto]"
        onSubmit={apply}
      >
        <Field>
          <FieldLabel htmlFor="user-search">搜索用户</FieldLabel>
          <Input
            id="user-search"
            aria-label="搜索用户"
            value={draft.search}
            onChange={(event) => update("search", event.target.value)}
            placeholder="用户名"
          />
        </Field>
        <Button
          type="button"
          variant="outline"
          className="justify-between md:hidden"
          aria-expanded={advancedOpen}
          aria-controls="advanced-user-filters"
          onClick={() => setAdvancedOpen((current) => !current)}
        >
          <span className="flex items-center gap-2">
            <SlidersHorizontalIcon />
            高级筛选
            {advancedCount > 0 && (
              <span className="rounded-full bg-primary px-1.5 py-0.5 text-[0.6875rem] leading-none text-primary-foreground">
                {advancedCount}
              </span>
            )}
          </span>
          <ChevronDownIcon
            className={cn("transition-transform", advancedOpen && "rotate-180")}
          />
        </Button>
        <div
          id="advanced-user-filters"
          className={cn(
            "col-span-full grid gap-3 sm:grid-cols-3 md:contents",
            !advancedOpen && "hidden md:contents"
          )}
        >
          <Field>
            <FieldLabel htmlFor="user-account">账户状态</FieldLabel>
            <Select
              items={accountOptions}
              value={draft.account}
              onValueChange={(value) =>
                update("account", value as AdminUserListQuery["account"])
              }
            >
              <SelectTrigger
                id="user-account"
                aria-label="账户状态"
                className="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {accountOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="user-credential">凭据状态</FieldLabel>
            <Select
              items={credentialOptions}
              value={draft.credential}
              onValueChange={(value) =>
                update("credential", value as AdminUserListQuery["credential"])
              }
            >
              <SelectTrigger
                id="user-credential"
                aria-label="凭据状态"
                className="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {credentialOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="user-health">设备健康</FieldLabel>
            <Select
              items={healthOptions}
              value={draft.health}
              onValueChange={(value) =>
                update("health", value as AdminUserListQuery["health"])
              }
            >
              <SelectTrigger
                id="user-health"
                aria-label="设备健康"
                className="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {healthOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
        </div>
        <Button className="self-end" type="submit">
          应用筛选
        </Button>
      </form>
    </section>
  )
}
