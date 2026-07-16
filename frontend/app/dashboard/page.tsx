"use client"

import { ActivityIcon, BatteryChargingIcon, Clock3Icon, PlugZapIcon, RotateCcwIcon, SearchIcon, TriangleAlertIcon } from "lucide-react"
import { useRouter } from "next/navigation"
import { useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import { AddPileDialog } from "@/components/add-pile-dialog"
import { AppShell } from "@/components/app-shell"
import { MetricCard } from "@/components/metric-card"
import { PileCard } from "@/components/pile-card"
import { UsageGuideDialog } from "@/components/usage-guide-dialog"
import { YybLoginDialog } from "@/components/yyb-login-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/lib/auth-context"
import { filterPiles, type PortFilter } from "@/lib/dashboard-filters"
import { useDashboard } from "@/lib/dashboard-context"

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "--"
}

export default function DashboardPage() {
  const router = useRouter()
  const { currentUser, fetchMe, clearSession } = useAuth()
  const { snapshot, loading, streamState, fetchSnapshot, connectStream, disconnectStream, refreshFromCapture, deletePile, updatePile } = useDashboard()
  const [refreshing, setRefreshing] = useState(false)
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState<PortFilter>("all")

  useEffect(() => {
    let active = true
    async function load() {
      const user = currentUser ?? await fetchMe()
      if (!user) return router.replace("/login")
      if (user.role === "admin") return router.replace("/admin")
      try { await fetchSnapshot(); if (active) connectStream() } catch (reason) { handleError(reason) }
    }
    void load()
    return () => { active = false; disconnectStream() }
  // Initial page load intentionally uses the established session and snapshot only once.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const entries = useMemo(() => filterPiles(snapshot.piles, search, filter), [filter, search, snapshot.piles])
  const visiblePortCount = entries.reduce((total, entry) => total + entry.portIds.length, 0)
  const hasActiveFilter = Boolean(search.trim()) || filter !== "all"
  const streamLabel = streamState === "connected" ? "已连接" : streamState === "error" ? "重连中" : streamState === "connecting" ? "连接中" : "准备中"

  function handleError(reason: unknown) {
    const message = (reason as Error).message
    if (message.includes("登录已失效")) { clearSession(); router.replace("/login"); return }
    toast.error(message)
  }

  async function refresh() {
    setRefreshing(true)
    try { await refreshFromCapture(); toast.success(snapshot.refresh.message || "设备状态已刷新") } catch (reason) { handleError(reason) } finally { setRefreshing(false) }
  }

  return <AppShell compact title="充电桩运营看板" description="端口占用、刷新状态与筛选结果一处查看；仅在主动刷新时请求远端接口。" actions={<div className="grid grid-cols-2 gap-2 [&_button]:w-full md:flex md:flex-wrap md:items-center md:[&_button]:w-auto"><span className="hidden text-xs text-muted-foreground lg:inline">实时连接：{streamLabel}</span><UsageGuideDialog /><YybLoginDialog /><AddPileDialog /><Button variant="outline" disabled={refreshing} onClick={() => void refresh()}><Clock3Icon className={refreshing ? "animate-spin" : ""} />{refreshing ? "刷新中…" : "刷新状态"}</Button></div>}>
    <Card className="shadow-none" aria-label="运营摘要"><CardContent className="grid divide-y p-0 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4"><MetricCard compact label="充电桩" value={snapshot.statistics.pileCount} detail="当前账户添加的设备" icon={PlugZapIcon} /><MetricCard compact label="全部端口" value={snapshot.statistics.portCount} detail="所有设备端口总和" icon={ActivityIcon} /><MetricCard compact label="正在使用" value={snapshot.statistics.inUsePortCount} detail="当前正在充电的端口" icon={BatteryChargingIcon} /><MetricCard compact label="异常端口" value={snapshot.statistics.offlinePorts} detail="离线或暂时不可访问" icon={TriangleAlertIcon} /></CardContent></Card>
    <Card className="mt-3 shadow-none"><CardContent className="grid gap-2 p-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center"><div className="flex flex-wrap items-center gap-x-4 gap-y-1"><div><h2 className="text-sm font-semibold">查找充电口</h2><p className="mt-0.5 text-xs text-muted-foreground">{entries.length} 台桩 · {visiblePortCount} 个充电口</p></div><p className="flex items-center gap-2 text-xs text-muted-foreground"><ActivityIcon className="size-3.5 text-foreground" /><span className="font-medium text-foreground">{snapshot.refresh.message || "等待首次刷新"}</span><span>· 上次 {formatTime(snapshot.refresh.lastRemoteAt)}</span></p></div><div className="grid gap-2 sm:grid-cols-[minmax(15rem,1fr)_10rem] lg:w-[32rem]"><label className="relative block h-8"><span className="sr-only">搜索充电桩或端口号</span><SearchIcon className="pointer-events-none absolute inset-y-0 left-3 my-auto size-4 text-muted-foreground" /><Input value={search} onChange={(event) => setSearch(event.target.value)} className="pl-10" placeholder="名称、桩号、地址或端口号" /></label><select aria-label="按充电口状态筛选" className="h-8 rounded-lg border bg-background px-2.5 text-sm" value={filter} onChange={(event) => setFilter(event.target.value as PortFilter)}><option value="all">全部充电口</option><option value="idle">仅看空闲</option><option value="charging">仅看充电中</option><option value="offline">仅看离线</option></select></div></CardContent></Card>
    <section className="mt-4 space-y-4" aria-label="充电桩列表">{loading ? <div className="grid gap-4"><Skeleton className="h-64 w-full" /><Skeleton className="h-64 w-full" /></div> : entries.map((entry) => <PileCard key={entry.pile.id} pile={entry.pile} visiblePortIds={entry.portIds} filtering={hasActiveFilter} onRemove={(id) => void deletePile(id).then(() => toast.success("充电桩已移除")).catch(handleError)} onUpdate={(id, payload) => void updatePile(id, payload).then(() => toast.success("设备资料已更新")).catch(handleError)} />)}{!loading && entries.length === 0 && <Card className="border-dashed shadow-none"><CardContent className="py-16 text-center"><PlugZapIcon className="mx-auto size-8 text-muted-foreground" /><h2 className="mt-4 text-lg font-semibold">{snapshot.piles.length ? "没有匹配的充电口" : "还没有充电桩"}</h2><p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">{snapshot.piles.length ? "调整搜索内容或状态条件，查看其他充电口。" : "建议先完成扫码登录，再通过添加入口录入桩号；系统会在添加设备时自动维护登录凭据。"}</p>{snapshot.piles.length && hasActiveFilter ? <Button className="mt-5" variant="outline" onClick={() => { setSearch(""); setFilter("all") }}><RotateCcwIcon />清除筛选条件</Button> : <div className="mt-5"><AddPileDialog /></div>}</CardContent></Card>}</section>
    <div className="sticky bottom-3 mt-5 md:hidden"><Button className="w-full shadow-lg" disabled={refreshing} onClick={() => void refresh()}><Clock3Icon className={refreshing ? "animate-spin" : ""} />{refreshing ? "刷新中…" : "主动刷新设备状态"}</Button></div>
  </AppShell>
}
