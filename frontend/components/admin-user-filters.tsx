"use client"

import { useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { AdminUserListQuery } from "@/lib/types"

const credentialOptions = [
  ["all", "全部凭据"],
  ["unbound", "未绑定扫码"],
  ["waiting_device", "等待添加设备"],
  ["healthy", "凭据正常"],
  ["sync_failed", "同步失败"],
  ["expired", "凭据已失效"],
] as const

export function AdminUserFilters({ query, onApply }: { query: AdminUserListQuery; onApply: (query: AdminUserListQuery) => void }) {
  const [draft, setDraft] = useState(query)

  useEffect(() => setDraft(query), [query])

  function update<Key extends keyof AdminUserListQuery>(key: Key, value: AdminUserListQuery[Key]) {
    setDraft((current) => ({ ...current, [key]: value }))
  }

  function apply() {
    onApply({ ...draft, search: draft.search.trim(), page: 1 })
  }

  function reset() {
    const next = { page: 1, pageSize: query.pageSize, search: "", account: "all", credential: "all", health: "all" } as AdminUserListQuery
    setDraft(next)
    onApply(next)
  }

  return <section className="rounded-xl border bg-card p-4 shadow-none" aria-label="用户筛选"><div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between"><div><h2 className="text-sm font-semibold">筛选用户</h2><p className="mt-1 text-xs leading-5 text-muted-foreground">组合账户状态、扫码凭据和设备健康度，快速定位需要处理的账户。</p></div><Button variant="ghost" size="sm" onClick={reset}>重置筛选</Button></div><div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(12rem,1fr)_10rem_11rem_9rem_auto]"><label className="grid gap-1 text-xs font-medium">搜索用户<Input aria-label="搜索用户" value={draft.search} onChange={(event) => update("search", event.target.value)} placeholder="用户名" /></label><label className="grid gap-1 text-xs font-medium">账户状态<select aria-label="账户状态" className="h-8 rounded-lg border bg-background px-2.5 text-sm" value={draft.account} onChange={(event) => update("account", event.target.value as AdminUserListQuery["account"])}><option value="all">全部账户</option><option value="enabled">已启用</option><option value="disabled">已停用</option></select></label><label className="grid gap-1 text-xs font-medium">凭据状态<select aria-label="凭据状态" className="h-8 rounded-lg border bg-background px-2.5 text-sm" value={draft.credential} onChange={(event) => update("credential", event.target.value as AdminUserListQuery["credential"])}>{credentialOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label><label className="grid gap-1 text-xs font-medium">设备健康<select aria-label="设备健康" className="h-8 rounded-lg border bg-background px-2.5 text-sm" value={draft.health} onChange={(event) => update("health", event.target.value as AdminUserListQuery["health"])}><option value="all">全部状态</option><option value="healthy">状态正常</option><option value="risk">存在风险</option></select></label><Button className="self-end" onClick={apply}>应用筛选</Button></div></section>
}
