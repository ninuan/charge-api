"use client"

import {
  ActivityIcon,
  BatteryChargingIcon,
  BookOpenCheckIcon,
  PlugZapIcon,
  PlusIcon,
  QrCodeIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  SearchIcon,
  TriangleAlertIcon,
} from "lucide-react"
import dynamic from "next/dynamic"
import { useRouter } from "next/navigation"
import {
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
} from "react"
import { toast } from "sonner"

import { AppShell } from "@/components/app-shell"
import { MetricCard } from "@/components/metric-card"
import { PileCard } from "@/components/pile-card"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/lib/auth-context"
import { filterPiles, type PortFilter } from "@/lib/dashboard-filters"
import { useDashboard } from "@/lib/dashboard-context"
import {
  parseDashboardQuery,
  serializeDashboardQuery,
} from "@/lib/dashboard-query"

// 三个对话框只在点击后才需要，从首包拆出；占位按钮与真实触发按钮
// 同样式同尺寸，chunk 加载完成前后不产生布局跳动。
const UsageGuideDialog = dynamic(
  () =>
    import("@/components/usage-guide-dialog").then((m) => m.UsageGuideDialog),
  {
    ssr: false,
    loading: () => (
      <Button variant="outline" disabled>
        <BookOpenCheckIcon />
        使用说明
      </Button>
    ),
  }
)
const YybLoginDialog = dynamic(
  () => import("@/components/yyb-login-dialog").then((m) => m.YybLoginDialog),
  {
    ssr: false,
    loading: () => (
      <Button variant="outline" disabled>
        <QrCodeIcon />
        扫码登录
      </Button>
    ),
  }
)
const AddPileDialog = dynamic(
  () => import("@/components/add-pile-dialog").then((m) => m.AddPileDialog),
  {
    ssr: false,
    loading: () => (
      <Button disabled>
        <PlusIcon />
        添加充电桩
      </Button>
    ),
  }
)
const DeviceHistorySheet = dynamic(
  () =>
    import("@/components/device-history-sheet").then(
      (m) => m.DeviceHistorySheet
    ),
  { ssr: false }
)

function formatTime(value?: string) {
  return value
    ? new Date(value).toLocaleTimeString("zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      })
    : "--"
}

const portFilterOptions = [
  { value: "all", label: "全部充电口" },
  { value: "idle", label: "仅看空闲" },
  { value: "charging", label: "仅看充电中" },
  { value: "offline", label: "仅看离线" },
]

export default function DashboardPage() {
  const router = useRouter()
  const { currentUser, fetchMe, clearSession } = useAuth()
  const {
    snapshot,
    loading,
    streamState,
    fetchSnapshot,
    connectStream,
    disconnectStream,
    refreshFromCapture,
    deletePile,
    updatePile,
    reorderPiles,
  } = useDashboard()
  const [refreshing, setRefreshing] = useState(false)
  const [reordering, setReordering] = useState(false)
  const [initialLoadFinished, setInitialLoadFinished] = useState(false)
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState<PortFilter>("all")
  const [historyPileId, setHistoryPileId] = useState<string | null>(null)
  const [queryReady, setQueryReady] = useState(false)
  // 输入框即时回显，筛选计算滞后一拍，键入时不再同步重渲染整个卡片列表。
  const deferredSearch = useDeferredValue(search)

  useEffect(() => {
    let active = true
    function restoreQuery() {
      const query = parseDashboardQuery(window.location.search)
      setSearch(query.search)
      setFilter(query.filter)
    }

    window.addEventListener("popstate", restoreQuery)
    queueMicrotask(() => {
      if (!active) return
      restoreQuery()
      setQueryReady(true)
    })
    return () => {
      active = false
      window.removeEventListener("popstate", restoreQuery)
    }
  }, [])

  useEffect(() => {
    if (!queryReady) return
    const query = serializeDashboardQuery(
      { search, filter },
      window.location.search
    )
    const nextURL = `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`
    window.history.replaceState(window.history.state, "", nextURL)
  }, [filter, queryReady, search])

  const handleError = useCallback(
    (reason: unknown) => {
      const message = (reason as Error).message
      if (message.includes("登录已失效")) {
        clearSession()
        router.replace("/login")
        return
      }
      toast.error(message)
    },
    [clearSession, router]
  )

  useEffect(() => {
    let active = true
    async function load() {
      const user = currentUser ?? (await fetchMe())
      if (!user) return router.replace("/login")
      if (user.role === "admin") return router.replace("/admin")
      try {
        await fetchSnapshot()
        if (active) connectStream()
      } catch (reason) {
        handleError(reason)
      } finally {
        if (active) setInitialLoadFinished(true)
      }
    }
    void load()
    return () => {
      active = false
      disconnectStream()
    }
    // Initial page load intentionally uses the established session and snapshot only once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleRemove = useCallback(
    (id: string) => {
      void deletePile(id)
        .then(() => toast.success("充电桩已移除"))
        .catch(handleError)
    },
    [deletePile, handleError]
  )

  const handleUpdate = useCallback(
    (
      id: string,
      payload: { name: string; address: string; sortOrder: number }
    ) => {
      void updatePile(id, payload)
        .then(() => toast.success("设备资料已更新"))
        .catch(handleError)
    },
    [handleError, updatePile]
  )

  const handleMove = useCallback(
    async (id: string, direction: "up" | "down") => {
      const ids = snapshot.piles.map((pile) => pile.id)
      const currentIndex = ids.indexOf(id)
      const nextIndex = direction === "up" ? currentIndex - 1 : currentIndex + 1
      if (currentIndex < 0 || nextIndex < 0 || nextIndex >= ids.length) return

      ;[ids[currentIndex], ids[nextIndex]] = [ids[nextIndex], ids[currentIndex]]
      setReordering(true)
      try {
        await reorderPiles(ids)
        toast.success("充电桩顺序已调整")
      } catch (reason) {
        handleError(reason)
      } finally {
        setReordering(false)
      }
    },
    [handleError, reorderPiles, snapshot.piles]
  )

  const entries = useMemo(
    () => filterPiles(snapshot.piles, deferredSearch, filter),
    [filter, deferredSearch, snapshot.piles]
  )
  const historyPile = useMemo(
    () => snapshot.piles.find((pile) => pile.id === historyPileId) ?? null,
    [historyPileId, snapshot.piles]
  )
  const openHistory = useCallback((id: string) => setHistoryPileId(id), [])
  const visiblePortCount = entries.reduce(
    (total, entry) => total + entry.portIds.length,
    0
  )
  const hasActiveFilter = Boolean(deferredSearch.trim()) || filter !== "all"
  const showingInitialSkeleton = loading || !initialLoadFinished
  const streamLabel =
    streamState === "connected"
      ? "已连接"
      : streamState === "error"
        ? "重连中"
        : streamState === "connecting"
          ? "连接中"
          : "准备中"

  async function refresh() {
    setRefreshing(true)
    try {
      const next = await refreshFromCapture()
      toast.success(next.refresh.message || "设备状态已刷新")
    } catch (reason) {
      handleError(reason)
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <AppShell
      compact
      title="充电桩运营看板"
      description="端口占用、刷新状态与筛选结果一处查看；仅在主动刷新时请求远端接口。"
      actions={
        <div className="grid grid-cols-2 gap-2 md:flex md:flex-wrap md:items-center [&_button]:w-full md:[&_button]:w-auto">
          <span className="hidden items-center gap-1.5 text-xs text-muted-foreground lg:inline-flex">
            <span
              aria-hidden
              className={`size-1.5 rounded-full ${
                streamState === "connected"
                  ? "bg-success"
                  : streamState === "error"
                    ? "bg-destructive motion-safe:animate-pulse"
                    : streamState === "connecting"
                      ? "bg-warning motion-safe:animate-pulse"
                      : "bg-muted-foreground/40"
              }`}
            />
            实时连接：{streamLabel}
          </span>
          <UsageGuideDialog />
          <YybLoginDialog />
          <AddPileDialog />
          <Button
            variant="outline"
            disabled={refreshing}
            onClick={() => void refresh()}
          >
            <RefreshCwIcon className={refreshing ? "animate-spin" : ""} />
            {refreshing ? "刷新中…" : "刷新状态"}
          </Button>
        </div>
      }
    >
      <Card className="shadow-xs" aria-label="运营摘要">
        <CardContent className="grid grid-cols-2 p-0 xl:grid-cols-4">
          <MetricCard
            compact
            className="border-r border-b xl:border-b-0"
            tone="primary"
            label="充电桩"
            value={snapshot.statistics.pileCount}
            detail="当前账户添加的设备"
            icon={PlugZapIcon}
          />
          <MetricCard
            compact
            className="border-b xl:border-r xl:border-b-0"
            label="全部端口"
            value={snapshot.statistics.portCount}
            detail="所有设备端口总和"
            icon={ActivityIcon}
          />
          <MetricCard
            compact
            className="border-r"
            tone="warning"
            label="正在使用"
            value={snapshot.statistics.inUsePortCount}
            detail="当前正在充电的端口"
            icon={BatteryChargingIcon}
          />
          <MetricCard
            compact
            tone="destructive"
            label="异常端口"
            value={snapshot.statistics.offlinePorts}
            detail="离线或暂时不可访问"
            icon={TriangleAlertIcon}
          />
        </CardContent>
      </Card>
      <Card className="mt-3 shadow-xs md:sticky md:top-20 md:z-30">
        <CardContent className="grid gap-2 p-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
            <div>
              <h2 className="text-sm font-semibold">查找充电口</h2>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {entries.length} 台桩 · {visiblePortCount} 个充电口
              </p>
            </div>
            <p className="flex items-center gap-2 text-xs text-muted-foreground">
              <ActivityIcon className="size-3.5 text-foreground" />
              <span className="font-medium text-foreground">
                {snapshot.refresh.message || "等待首次刷新"}
              </span>
              <span>· 上次 {formatTime(snapshot.refresh.lastRemoteAt)}</span>
            </p>
          </div>
          <div className="grid gap-2 sm:grid-cols-[minmax(15rem,1fr)_10rem] lg:w-[32rem]">
            <label className="relative block">
              <span className="sr-only">搜索充电桩或端口号</span>
              <SearchIcon className="pointer-events-none absolute inset-y-0 left-3 my-auto size-4 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                className="pl-10 md:pl-10"
                placeholder="名称、桩号、地址或端口号"
              />
            </label>
            <Select
              items={portFilterOptions}
              value={filter}
              onValueChange={(value) => setFilter(value as PortFilter)}
            >
              <SelectTrigger aria-label="按充电口状态筛选" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {portFilterOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <Button
            className="mt-1 w-full md:hidden"
            disabled={refreshing}
            onClick={() => void refresh()}
          >
            <RefreshCwIcon className={refreshing ? "animate-spin" : ""} />
            {refreshing ? "刷新中…" : "主动刷新设备状态"}
          </Button>
        </CardContent>
      </Card>
      <section
        className="mt-4 flex flex-col gap-4"
        aria-label="充电桩列表"
        aria-busy={reordering}
      >
        {showingInitialSkeleton ? (
          <div className="grid gap-4">
            <Skeleton className="h-64 w-full" />
            <Skeleton className="h-64 w-full" />
          </div>
        ) : (
          <div className="grid gap-4">
            {entries.map((entry) => (
              <div key={entry.pile.id} className="pile-card-enter">
                <PileCard
                  pile={entry.pile}
                  visiblePortIds={entry.portIds}
                  filtering={hasActiveFilter}
                  onRemove={handleRemove}
                  onUpdate={handleUpdate}
                  canMoveUp={snapshot.piles[0]?.id !== entry.pile.id}
                  canMoveDown={snapshot.piles.at(-1)?.id !== entry.pile.id}
                  reordering={reordering}
                  onMove={handleMove}
                  onHistory={openHistory}
                />
              </div>
            ))}
          </div>
        )}
        {!showingInitialSkeleton && entries.length === 0 && (
          <Card className="border-dashed shadow-xs">
            <CardContent className="py-16 text-center">
              <PlugZapIcon className="mx-auto size-8 text-muted-foreground" />
              <h2 className="mt-4 text-lg font-semibold">
                {snapshot.piles.length ? "没有匹配的充电口" : "还没有充电桩"}
              </h2>
              <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">
                {snapshot.piles.length
                  ? "调整搜索内容或状态条件，查看其他充电口。"
                  : "建议先完成扫码登录，再通过添加入口录入桩号；系统会在添加设备时自动维护登录凭据。"}
              </p>
              {snapshot.piles.length && hasActiveFilter ? (
                <Button
                  className="mt-5"
                  variant="outline"
                  onClick={() => {
                    setSearch("")
                    setFilter("all")
                  }}
                >
                  <RotateCcwIcon />
                  清除筛选条件
                </Button>
              ) : (
                <div className="mt-5">
                  <AddPileDialog />
                </div>
              )}
            </CardContent>
          </Card>
        )}
      </section>
      {historyPile && (
        <DeviceHistorySheet
          key={historyPile.id}
          pile={historyPile}
          open
          onOpenChange={(next) => {
            if (!next) setHistoryPileId(null)
          }}
        />
      )}
    </AppShell>
  )
}
