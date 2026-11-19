"use client"

import {
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  ArrowUpIcon,
  BellIcon,
  BellRingIcon,
  ChevronRightIcon,
  Clock3Icon,
  MapPinIcon,
  PencilIcon,
  PlugZapIcon,
  RefreshCwIcon,
  SlidersHorizontalIcon,
  Trash2Icon,
  WifiOffIcon,
  XIcon,
  ZapIcon,
} from "lucide-react"
import dynamic from "next/dynamic"
import { useRouter, useSearchParams } from "next/navigation"
import {
  Fragment,
  Suspense,
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
} from "react"
import type { Pile, Port } from "@/lib/types"
import { AnnouncementStrip } from "@/components/announcement-strip"
import { AppShell } from "@/components/app-shell"
import { AddPileDialog } from "@/components/add-pile-dialog"
import { UsageGuideDialog } from "@/components/usage-guide-dialog"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { useDashboard } from "@/lib/dashboard-context"
import { useAuth } from "@/lib/auth-context"
import { useWatch } from "@/lib/watch-context"
import { useNotifications } from "@/lib/notification-context"
import { useOnline } from "@/lib/browser-state"
import { RequestError } from "@/lib/http"
import { notify } from "@/lib/feedback"
import { parseDashboardQuery } from "@/lib/dashboard-query"
import {
  dashboardHref,
  formatSnapshotTime,
  portCounts,
  portTime,
  selectWorkbenchPiles,
  updatePageQuery,
} from "@/lib/workbench"
import { useDashboardSession } from "./dashboard-session"
import {
  PulseDot,
  SectionHeading,
  StationSkeletonGrid,
  StatusPill,
  WorkbenchButton,
  WorkbenchEmpty,
  WorkbenchError,
  WorkbenchLoading,
  WorkbenchSearch,
} from "./surfaces"

const WatchRuleDialog = dynamic(
  () =>
    import("@/components/watch-rule-dialog").then(
      (module) => module.WatchRuleDialog
    ),
  { ssr: false }
)

const stateLabel = { idle: "空闲", in_use: "使用中", offline: "离线" } as const

function shortRemaining(port: Port): string {
  if (port.status !== "in_use") return ""
  if (port.sessionMin && port.sessionMin > 0) {
    const remMin = Math.max(
      1,
      Math.round(port.sessionMin - (port.usedSeconds || 0) / 60)
    )
    if (remMin < 60) return `${remMin}m`
    const h = Math.floor(remMin / 60)
    const m = remMin % 60
    return m > 0 ? `${h}h${m}m` : `${h}h`
  }
  if (port.remainingText) return port.remainingText.replace(/\s+/g, "")
  return "充电中"
}

function portProgressPct(port: Port): number {
  if (port.status !== "in_use" || !port.sessionMin || port.sessionMin <= 0) {
    return 0
  }
  return Math.min(
    100,
    Math.round(((port.usedSeconds || 0) / (port.sessionMin * 60)) * 100)
  )
}

function soonestPort(pile: Pile): Port | null {
  let best: { port: Port; rem: number } | null = null
  for (const port of pile.ports) {
    if (port.status !== "in_use" || !port.sessionMin) continue
    const rem = Math.max(1, port.sessionMin - (port.usedSeconds || 0) / 60)
    if (!best || rem < best.rem) best = { port, rem }
  }
  return best?.port ?? null
}

function SocketGlyph({ status }: { status: Port["status"] }) {
  return (
    <svg
      viewBox="0 0 20 20"
      width="14"
      height="14"
      fill="none"
      className="wb-socket-pins"
      aria-hidden="true"
    >
      <rect
        x="1.5"
        y="1.5"
        width="17"
        height="17"
        rx="4.5"
        stroke="currentColor"
        strokeWidth="1.25"
        strokeOpacity="0.4"
        strokeDasharray={status === "offline" ? "2.2 2" : undefined}
      />
      <line
        x1="10"
        y1="5.2"
        x2="10"
        y2="8.1"
        stroke="currentColor"
        strokeWidth="1.65"
        strokeLinecap="round"
      />
      <line
        x1="7.3"
        y1="10.8"
        x2="6.2"
        y2="13.8"
        stroke="currentColor"
        strokeWidth="1.65"
        strokeLinecap="round"
      />
      <line
        x1="12.7"
        y1="10.8"
        x2="13.8"
        y2="13.8"
        stroke="currentColor"
        strokeWidth="1.65"
        strokeLinecap="round"
      />
    </svg>
  )
}

export function DashboardWorkbench() {
  return (
    <Suspense fallback={<WorkbenchLoading />}>
      <WorkbenchContent />
    </Suspense>
  )
}

function WorkbenchContent() {
  const router = useRouter()
  const params = useSearchParams()
  const query = parseDashboardQuery(params.toString())
  const search = params.get("q") ?? ""
  const selectedId = params.get("pile")
  const selectedPortId =
    Number(params.get("port")) ||
    (/^\d{1,2}$/.test(search) ? Number(search) : undefined)
  const deferredSearch = useDeferredValue(search)

  const { currentUser, clearSession } = useAuth()
  const {
    snapshot,
    streamState,
    refreshFromCapture,
    reorderPiles,
    fetchSnapshot,
  } = useDashboard()
  const { initialLoading, error, retry } = useDashboardSession()
  const { rules, loaded: watchLoaded } = useWatch()
  const { unreadCount, loaded: notificationsLoaded } = useNotifications()
  const online = useOnline()

  const [refreshing, setRefreshing] = useState(false)
  const [ordering, setOrdering] = useState(false)
  const [reordering, setReordering] = useState(false)
  const [refreshError, setRefreshError] = useState("")
  const [watchPileId, setWatchPileId] = useState<string | null>(null)

  const entries = useMemo(
    () => selectWorkbenchPiles(snapshot.piles, deferredSearch, query.filter),
    [snapshot.piles, deferredSearch, query.filter]
  )

  const selected = selectedId
    ? snapshot.piles.find((p) => p.id === selectedId)
    : undefined

  const stale =
    !online ||
    !!error ||
    !!refreshError ||
    streamState === "error" ||
    snapshot.refresh.partial ||
    snapshot.refresh.failedDevices > 0

  const availablePiles = snapshot.piles.filter(
    (p) => p.online && portCounts(p).idle > 0
  )
  const totalIdlePorts = availablePiles.reduce(
    (sum, p) => sum + portCounts(p).idle,
    0
  )
  const chargingPiles = snapshot.piles.filter(
    (p) => p.online && portCounts(p).in_use > 0
  ).length
  const offlinePiles = snapshot.piles.filter(
    (p) => !p.online || p.ports.length === 0 || portCounts(p).offline > 0
  ).length

  const activeRules = rules.filter(
    (rule) => rule.mode === "temporary" && rule.enabled && !rule.completedAt
  )

  const searchItems = snapshot.piles.map((p) => ({
    title: p.name || p.number,
    description: `${p.number} · ${p.address}`,
    href: dashboardHref(p.id),
  }))

  useEffect(() => {
    if (!selectedId) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return
      if (document.querySelector('[role="dialog"]')) return
      updatePageQuery({ pile: null, port: null, detail: null })
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [selectedId])

  useEffect(() => {
    const focus = (event: KeyboardEvent) => {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey)
        return
      if (
        event.target instanceof HTMLElement &&
        (event.target.matches("input,textarea,select") ||
          event.target.isContentEditable)
      )
        return
      event.preventDefault()
      ;(
        document.querySelector(
          '[aria-label="搜索充电桩"]'
        ) as HTMLInputElement | null
      )?.focus()
    }
    window.addEventListener("keydown", focus)
    return () => window.removeEventListener("keydown", focus)
  }, [])

  async function refresh() {
    setRefreshing(true)
    setRefreshError("")
    try {
      const result = await refreshFromCapture()
      if (result.refresh.partial || result.refresh.failedDevices > 0)
        notify.warning("部分设备未能更新", {
          description: result.refresh.message || "未成功的设备保留最近快照。",
        })
      else
        notify.success(
          result.refresh.cached ? "正在使用最近一次快照" : "设备状态已更新",
          { description: result.refresh.message || undefined }
        )
    } catch (reason) {
      if (reason instanceof RequestError && reason.status === 401) {
        clearSession()
        router.replace("/login")
        return
      }
      setRefreshError(
        reason instanceof Error ? reason.message : "暂时无法刷新设备。"
      )
      notify.error(reason, { title: "刷新失败，已保留最近快照" })
    } finally {
      setRefreshing(false)
    }
  }

  async function move(id: string, direction: number) {
    const ids = snapshot.piles.map((p) => p.id)
    const index = ids.indexOf(id)
    const next = index + direction
    if (index < 0 || next < 0 || next >= ids.length) return
    const target = ids[index]
    ids[index] = ids[next]
    ids[next] = target
    setReordering(true)
    try {
      await reorderPiles(ids)
      notify.success("常用桩顺序已保存")
    } catch (reason) {
      notify.error(reason, { title: "排序未保存" })
    } finally {
      setReordering(false)
    }
  }

  function remind(pile: Pile) {
    const rule = activeRules.find((r) => r.deviceId === pile.id)
    if (rule) {
      router.push(`/dashboard/reminders?pile=${encodeURIComponent(pile.id)}`)
    } else {
      setWatchPileId(pile.id)
    }
  }

  const actions = (
    <>
      <WorkbenchButton
        variant="outline"
        busy={refreshing}
        disabled={!online || !currentUser?.refreshEnabled || initialLoading}
        onClick={() => void refresh()}
      >
        <RefreshCwIcon size={14} />
        刷新状态
      </WorkbenchButton>
      <AddPileDialog
        disabled={!online || initialLoading}
        onAdded={(pile) => {
          updatePageQuery({
            pile: pile.id,
            port: null,
            detail: null,
            q: null,
            status: null,
          })
          void fetchSnapshot().catch(() => undefined)
        }}
      />
    </>
  )

  const heroTitle = initialLoading
    ? "常用充电桩"
    : !snapshot.piles.length
      ? "常用充电桩"
      : stale
        ? "常用充电桩 · 最近快照"
        : availablePiles.length > 0
          ? `${availablePiles.length} 处有空位，共 ${totalIdlePorts} 个空闲口`
          : "常用充电桩 · 暂无空位"

  const heroDesc = initialLoading
    ? "正在读取常去的地方。"
    : stale
      ? "显示最近快照，更新后再确认空闲情况。"
      : availablePiles.length > 0
        ? "直接看绿色插口编号前往现场，点击任意插口可查看已用时长与功率。"
        : "看看常去的地方，或在有空闲时提醒我。"

  return (
    <AppShell
      title={heroTitle}
      activeSection="piles"
      description={heroDesc}
      hidePageHeader
      connectionState={
        !online
          ? "offline"
          : streamState === "error"
            ? "reconnecting"
            : "connected"
      }
      counts={{
        reminders: watchLoaded ? activeRules.length : undefined,
        notifications: notificationsLoaded ? unreadCount : undefined,
      }}
      searchItems={searchItems}
      guideAction={
        <UsageGuideDialog
          key={params.has("guide") ? "open" : "closed"}
          initialOpen={params.get("guide") === "1"}
          compact
        />
      }
    >
      <AnnouncementStrip />

      {/* Stage Hero Header: Live Status Pulse + Instant Answer + Primary Actions */}
      <header className="wb-page-heading wb-stage-hero">
        <div className="wb-hero-copy">
          <div className="wb-hero-status-line">
            <PulseDot
              tone={
                stale
                  ? "warning"
                  : availablePiles.length > 0
                    ? "idle"
                    : snapshot.piles.length > 0
                      ? "busy"
                      : "offline"
              }
              pulse={!stale && availablePiles.length > 0}
            />
            <span className="wb-mono wb-small wb-muted">
              {snapshot.piles.length
                ? `快照 ${formatSnapshotTime(snapshot.refresh.lastRemoteAt || snapshot.updatedAt)}`
                : "等待首次读取"}
              {stale && snapshot.piles.length ? " · 待更新" : ""}
            </span>
          </div>
          <h1>{heroTitle}</h1>
          <p>{heroDesc}</p>
        </div>
        <div className="wb-heading-actions">{actions}</div>
      </header>

      {!!refreshError && (
        <div className="wb-state-banner wb-banner-warning" role="alert">
          <WifiOffIcon size={16} />
          <div>
            <strong>这次刷新未完成</strong>
            <span>{refreshError}</span>
          </div>
          <WorkbenchButton
            variant="ghost"
            onClick={() => router.push("/account?tab=connection")}
          >
            检查平台连接
            <ChevronRightIcon size={14} />
          </WorkbenchButton>
        </div>
      )}

      {(snapshot.refresh.partial || snapshot.refresh.failedDevices > 0) &&
        !refreshError && (
          <div className="wb-state-banner wb-banner-warning">
            <WifiOffIcon size={16} />
            <div>
              <strong>部分设备未更新</strong>
              <span>
                {snapshot.refresh.message ||
                  "未成功的设备仍显示最近快照，请以时间标记为准。"}
              </span>
            </div>
          </div>
        )}

      {/* Filter, Search & Snapshot Ribbon */}
      <aside className="wb-station-canvas" aria-label="常用充电桩列表">
        <div
          className="wb-canvas-ribbon"
          data-slot="dashboard-search-toolbar"
        >
          <div className="wb-seg-rail" role="group" aria-label="筛选充电口">
            {[
              ["all", "全部", snapshot.piles.length],
              ["idle", "有空闲", availablePiles.length],
              ["charging", "使用中", chargingPiles],
              ["offline", "离线", offlinePiles],
            ].map(([value, label, num]) => (
              <button
                key={String(value)}
                type="button"
                className={`wb-seg-btn ${
                  query.filter === value ? "wb-seg-active" : ""
                }`}
                aria-pressed={query.filter === value}
                onClick={() =>
                  updatePageQuery({
                    status: value === "all" ? null : String(value),
                    pile: null,
                    port: null,
                    detail: null,
                  })
                }
              >
                <span>{label}</span>
                {!initialLoading && (
                  <span className="wb-seg-cnt">{num}</span>
                )}
              </button>
            ))}
          </div>

          <div className="wb-ribbon-right">
            <WorkbenchSearch
              value={search}
              shortcut="/"
              onChange={(value) =>
                updatePageQuery(
                  { q: value, pile: null, port: null, detail: null },
                  true
                )
              }
            />
            <WorkbenchButton
              variant={ordering ? "secondary" : "ghost"}
              className="wb-order-toggle"
              aria-label={ordering ? "完成排序" : "调整充电桩顺序"}
              aria-pressed={ordering}
              disabled={snapshot.piles.length < 2 || !online}
              onClick={() => setOrdering((v) => !v)}
            >
              <SlidersHorizontalIcon size={14} />
              <span>{ordering ? "完成排序" : "排序"}</span>
            </WorkbenchButton>
          </div>
        </div>

        {/* Spatial Station Module Gallery */}
        {initialLoading && !snapshot.piles.length ? (
          <StationSkeletonGrid count={4} label="正在读取充电桩" />
        ) : error && !snapshot.piles.length ? (
          <WorkbenchError message={error} retry={() => void retry()} />
        ) : !entries.length ? (
          <WorkbenchEmpty
            title={
              snapshot.piles.length
                ? "没有找到充电桩"
                : "添加第一个常用充电桩"
            }
            description={
              snapshot.piles.length
                ? "换个名称、桩号或口号，或清除筛选。"
                : "添加常去的地方，以后不用反复扫码查找。"
            }
            icon={PlugZapIcon}
            action={
              snapshot.piles.length ? (
                <WorkbenchButton
                  variant="ghost"
                  onClick={() =>
                    updatePageQuery({
                      q: null,
                      status: null,
                      pile: null,
                      port: null,
                    })
                  }
                >
                  清除筛选
                </WorkbenchButton>
              ) : (
                <AddPileDialog
                  disabled={!online}
                  onAdded={(pile) => {
                    updatePageQuery({ pile: pile.id })
                    void fetchSnapshot().catch(() => undefined)
                  }}
                />
              )
            }
          />
        ) : (
          <div
            className="wb-station-grid"
            aria-hidden={Boolean(selectedId) || undefined}
            inert={Boolean(selectedId) || undefined}
          >
            {entries.map(({ pile }) => {
              const count = portCounts(pile)
              const unknown = pile.ports.length === 0
              const cardStale = stale || !pile.online
              const isWatching = activeRules.some((r) => r.deviceId === pile.id)
              const index = snapshot.piles.indexOf(pile)
              const idlePorts = pile.ports.filter((p) => p.status === "idle")
              const soonest = soonestPort(pile)

              return (
                <section
                  key={pile.id}
                  aria-label={`${pile.name || pile.number} 充电桩`}
                  className={`wb-station-card ${
                    isWatching
                      ? "wb-station-watching"
                      : unknown || !pile.online
                        ? "wb-station-offline"
                        : cardStale
                          ? "wb-station-stale"
                          : count.idle > 0
                            ? "wb-station-idle"
                            : "wb-station-busy"
                  }`}
                >
                  {/* Card Head */}
                  <div className="wb-card-head">
                    <div className="wb-card-id">
                      <div className="wb-card-title-row">
                        <button
                          type="button"
                          className="wb-card-title-btn"
                          aria-label={`查看 ${pile.name || pile.number}`}
                          onClick={() =>
                            updatePageQuery({
                              pile: pile.id,
                              port: null,
                              detail: null,
                            })
                          }
                        >
                          <h2>{pile.name || pile.number}</h2>
                        </button>
                        <span className="wb-mono wb-pile-code">
                          {pile.number || pile.id.slice(-8)}
                        </span>
                      </div>
                      <p className="wb-card-addr">
                        <MapPinIcon size={12} aria-hidden="true" />
                        <span>{pile.address || "尚未填写位置"}</span>
                      </p>
                    </div>

                    <div className="wb-card-readout">
                      {ordering ? (
                        <div className="wb-reorder-btns">
                          <WorkbenchButton
                            variant="outline"
                            className="wb-icon-button"
                            aria-label={`上移 ${pile.name}`}
                            disabled={!online || reordering || index === 0}
                            onClick={() => void move(pile.id, -1)}
                          >
                            <ArrowUpIcon size={13} />
                          </WorkbenchButton>
                          <WorkbenchButton
                            variant="outline"
                            className="wb-icon-button"
                            aria-label={`下移 ${pile.name}`}
                            disabled={
                              !online ||
                              reordering ||
                              index === snapshot.piles.length - 1
                            }
                            onClick={() => void move(pile.id, 1)}
                          >
                            <ArrowDownIcon size={13} />
                          </WorkbenchButton>
                        </div>
                      ) : unknown ? (
                        <StatusPill tone="neutral">尚未读取</StatusPill>
                      ) : !pile.online ? (
                        <StatusPill tone="offline">离线</StatusPill>
                      ) : cardStale ? (
                        <StatusPill tone="warning">
                          上次 {count.idle} 空
                        </StatusPill>
                      ) : count.idle > 0 ? (
                        <div className="wb-readout-big wb-idle-text">
                          <strong>{String(count.idle).padStart(2, "0")}</strong>
                          <span>
                            / {String(pile.ports.length).padStart(2, "0")} 空闲
                          </span>
                        </div>
                      ) : (
                        <StatusPill tone="busy">
                          全满 {pile.ports.length}/{pile.ports.length}
                        </StatusPill>
                      )}
                    </div>
                  </div>

                  {/* Recessed Hardware Socket Bay */}
                  <div className="wb-socket-bay">
                    {unknown ? (
                      <div className="wb-bay-empty">
                        <Clock3Icon size={15} />
                        <span>等待首次读取</span>
                      </div>
                    ) : (
                      <div
                        className="wb-socket-grid"
                        role="group"
                        aria-label={`${pile.name || pile.number}的充电口`}
                      >
                        {pile.ports.map((port) => {
                          const pct = portProgressPct(port)
                          const rem = shortRemaining(port)
                          return (
                            <button
                              key={port.id}
                              type="button"
                              className={`wb-socket-chip wb-socket-chip-${port.status} ${
                                cardStale ? "wb-chip-stale" : ""
                              }`}
                              aria-label={`${port.id} 号充电口，${
                                cardStale ? "上次" : ""
                              }${stateLabel[port.status]}`}
                              onClick={() =>
                                updatePageQuery({
                                  pile: pile.id,
                                  port: port.id,
                                  detail: null,
                                })
                              }
                            >
                              <span className="wb-socket-chip-top">
                                <span className="wb-socket-chip-num">
                                  {String(port.id).padStart(2, "0")}
                                </span>
                                <SocketGlyph status={port.status} />
                              </span>
                              <span className="wb-socket-chip-sub">
                                {port.status === "idle"
                                  ? cardStale
                                    ? "待确认"
                                    : "空闲"
                                  : port.status === "in_use"
                                    ? rem
                                    : "离线"}
                              </span>
                              {port.status === "in_use" && (
                                <span
                                  className="wb-socket-chip-meter"
                                  aria-hidden="true"
                                >
                                  <i style={{ width: `${pct || 28}%` }} />
                                </span>
                              )}
                            </button>
                          )
                        })}
                      </div>
                    )}
                  </div>

                  {/* Contextual Footer */}
                  <div className="wb-card-foot">
                    {unknown ? (
                      <>
                        <span className="wb-foot-note">
                          成功读取后展示端口状态
                        </span>
                        <button
                          type="button"
                          className="wb-foot-link"
                          onClick={() =>
                            updatePageQuery({
                              pile: pile.id,
                              port: null,
                              detail: null,
                            })
                          }
                        >
                          查看详情
                          <ChevronRightIcon size={13} />
                        </button>
                      </>
                    ) : !pile.online ? (
                      <>
                        <span className="wb-foot-note">
                          <WifiOffIcon size={12} />
                          暂时无法读取 · 上次{" "}
                          {formatSnapshotTime(pile.updatedAt)}
                        </span>
                        <button
                          type="button"
                          className="wb-foot-link"
                          onClick={() =>
                            updatePageQuery({
                              pile: pile.id,
                              port: null,
                              detail: null,
                            })
                          }
                        >
                          查看详情
                          <ChevronRightIcon size={13} />
                        </button>
                      </>
                    ) : cardStale ? (
                      <>
                        <span className="wb-foot-note wb-warning-text">
                          上次快照（{formatSnapshotTime(pile.updatedAt)}
                          ），请刷新确认
                        </span>
                        <button
                          type="button"
                          className="wb-foot-link"
                          onClick={() =>
                            updatePageQuery({
                              pile: pile.id,
                              port: null,
                              detail: null,
                            })
                          }
                        >
                          查看详情
                          <ChevronRightIcon size={13} />
                        </button>
                      </>
                    ) : isWatching ? (
                      <>
                        <span className="wb-foot-note wb-brand-text">
                          <PulseDot tone="brand" pulse />
                          <BellRingIcon size={13} />
                          <strong>提醒中</strong> · 发现空闲后通知一次
                        </span>
                        <WorkbenchButton
                          variant="outline"
                          disabled={!online || !watchLoaded}
                          onClick={() => remind(pile)}
                        >
                          管理提醒
                        </WorkbenchButton>
                      </>
                    ) : count.idle > 0 ? (
                      <>
                        <span className="wb-foot-note wb-idle-text">
                          <strong>
                            {idlePorts
                              .map((p) => String(p.id).padStart(2, "0"))
                              .join("、")}{" "}
                            号口
                          </strong>
                          读取时空闲
                        </span>
                        <button
                          type="button"
                          className="wb-foot-link"
                          onClick={() =>
                            updatePageQuery({
                              pile: pile.id,
                              port: null,
                              detail: null,
                            })
                          }
                        >
                          全部明细
                          <ArrowRightIcon size={13} />
                        </button>
                      </>
                    ) : (
                      <>
                        <span className="wb-foot-note">
                          {soonest?.remainingText
                            ? `最快预计 ${String(soonest.id).padStart(2, "0")} 号口（${soonest.remainingText}）`
                            : "当前全部使用中"}
                        </span>
                        <WorkbenchButton
                          disabled={!online || !watchLoaded}
                          onClick={() => remind(pile)}
                        >
                          <BellIcon size={13} />
                          有空闲时提醒我
                        </WorkbenchButton>
                      </>
                    )}
                  </div>
                </section>
              )
            })}
          </div>
        )}

        {/* Snapshot & SSE Status Footer */}
        <div className="wb-canvas-foot">
          <span className="wb-foot-clock">
            <Clock3Icon size={12} />
            {snapshot.piles.length
              ? `快照 ${formatSnapshotTime(snapshot.refresh.lastRemoteAt || snapshot.updatedAt)}`
              : "尚未读取"}
          </span>
          <span
            className="wb-stream-state"
            title="只表示网页与服务的连接，不代表设备在线"
          >
            {streamState === "connected"
              ? "实时通道已连接"
              : streamState === "error"
                ? "实时通道重连中"
                : "正在连接"}
          </span>
        </div>
      </aside>

      {/* FLOATING RIGHT-SIDE STATION INSPECTOR SHEET (URL-backed ?pile=&port=) */}
      {selectedId && (
        <div className="wb-inspector-layer">
          <div
            className="wb-inspector-backdrop"
            aria-hidden="true"
            onClick={() =>
              updatePageQuery({ pile: null, port: null, detail: null })
            }
          />
          <div className="wb-inspector-sheet">
            <article className="wb-inspector-article" aria-label="充电桩详情">
              <div className="wb-inspector-top-back">
                <button
                  type="button"
                  className="wb-back"
                  onClick={() =>
                    updatePageQuery({ pile: null, port: null, detail: null })
                  }
                >
                  <ArrowLeftIcon size={14} />
                  常用充电桩
                </button>
                <button
                  type="button"
                  className="wb-inspector-close"
                  aria-label="关闭详情"
                  onClick={() =>
                    updatePageQuery({ pile: null, port: null, detail: null })
                  }
                >
                  <XIcon size={16} />
                </button>
              </div>

              {initialLoading && !snapshot.piles.length ? (
                <WorkbenchLoading />
              ) : error && !snapshot.piles.length ? (
                <WorkbenchError message={error} retry={() => void retry()} />
              ) : selected ? (
                <PileDetail
                  key={selected.id}
                  pile={selected}
                  portId={selectedPortId}
                  stale={stale || !selected.online}
                  onRemind={() => remind(selected)}
                  activeReminder={activeRules.some(
                    (rule) => rule.deviceId === selected.id
                  )}
                />
              ) : (
                <WorkbenchEmpty
                  title="这个充电桩不在常用列表中"
                  description="可能已经被移除，请从列表重新选择或重新加载。"
                  action={
                    <WorkbenchButton
                      variant="outline"
                      onClick={() => void retry()}
                    >
                      重新加载
                    </WorkbenchButton>
                  }
                />
              )}
            </article>
          </div>
        </div>
      )}

      {watchPileId && (
        <WatchRuleDialog
          key={watchPileId}
          piles={snapshot.piles}
          target={{ pileId: watchPileId }}
          open
          onOpenChange={(open) => !open && setWatchPileId(null)}
        />
      )}
    </AppShell>
  )
}

function PileDetail({
  pile,
  portId,
  stale,
  onRemind,
  activeReminder,
}: {
  pile: Pile
  portId?: number
  stale: boolean
  onRemind: () => void
  activeReminder: boolean
}) {
  const { deletePile, updatePile, fetchSnapshot } = useDashboard()
  const { load: loadWatch, loaded: watchLoaded, error: watchError } = useWatch()
  const online = useOnline()

  const [modal, setModal] = useState<"edit" | "remove" | null>(null)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState("")
  const [name, setName] = useState(pile.name)
  const [address, setAddress] = useState(pile.address)
  const [portFilter, setPortFilter] = useState<"all" | "idle" | "in_use" | "offline">("all")

  const count = portCounts(pile)
  const unknown = pile.ports.length === 0
  const visiblePorts = pile.ports.filter(
    (p) => portFilter === "all" || p.status === portFilter
  )

  async function save(event: React.FormEvent) {
    event.preventDefault()
    setSaving(true)
    setFormError("")
    try {
      await updatePile(pile.id, {
        name: name.trim(),
        address: address.trim(),
        sortOrder: pile.sortOrder ?? 0,
      })
      setModal(null)
      notify.success("充电桩资料已保存")
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "保存失败，请重试。"
      )
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    setSaving(true)
    setFormError("")
    try {
      await deletePile(pile.id)
      await Promise.allSettled([fetchSnapshot(), loadWatch()])
      setModal(null)
      updatePageQuery({ pile: null, port: null, detail: null })
      notify.success("已移出常用桩，关联提醒已结束")
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "移除失败，请重试。"
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="wb-inspector-body">
      <header className="wb-insp-head">
        <div className="wb-insp-id">
          <div className="wb-insp-eyebrow">
            <span className="wb-mono wb-pile-code">
              NO. {pile.number || pile.id}
            </span>
            <StatusPill
              tone={
                unknown
                  ? "neutral"
                  : !pile.online
                    ? "offline"
                    : stale
                      ? "warning"
                      : "idle"
              }
            >
              {unknown
                ? "尚未读取"
                : stale
                  ? "最近快照"
                  : pile.online
                    ? "已读取"
                    : "离线"}
            </StatusPill>
          </div>
          <h2>{pile.name || pile.number}</h2>
          <p className="wb-card-addr">
            <MapPinIcon size={12} />
            {pile.address || "尚未填写位置"}
          </p>
        </div>

        <div className="wb-insp-head-ops">
          <WorkbenchButton
            variant="ghost"
            disabled={!online}
            onClick={() => {
              setName(pile.name)
              setAddress(pile.address)
              setFormError("")
              setModal("edit")
            }}
          >
            <PencilIcon size={13} />
            编辑
          </WorkbenchButton>
          <WorkbenchButton
            variant="ghost"
            className="wb-danger-text"
            disabled={!online}
            onClick={() => {
              setFormError("")
              setModal("remove")
            }}
          >
            <Trash2Icon size={13} />
            移出
          </WorkbenchButton>
        </div>
      </header>

      <div
        className={`wb-insp-watch-card ${!pile.online ? "wb-unavailable" : ""} ${
          stale ? "wb-is-stale" : ""
        }`}
      >
        <div className="wb-availability-copy">
          <span className="wb-availability-label">
            {unknown ? "读取状态" : stale ? "上次读取" : "最近空闲"}
          </span>
          <div className="wb-availability-number">
            {unknown || !pile.online ? (
              <>
                {unknown ? <Clock3Icon size={20} /> : <WifiOffIcon size={20} />}
                <strong>{unknown ? "尚未读取" : "暂时离线"}</strong>
              </>
            ) : (
              <>
                <strong>{String(count.idle).padStart(2, "0")}</strong>
                <span>
                  / {String(pile.ports.length).padStart(2, "0")} 个充电口
                </span>
              </>
            )}
          </div>
          <p className="wb-small wb-muted">
            {unknown
              ? "成功读取后才能判断是否有空闲口。"
              : !pile.online
                ? "当前无法确认状态，请稍后刷新或检查现场供电。"
                : stale
                  ? "快照可能已过期，更新后再确认是否可用。"
                  : count.idle
                    ? `${pile.ports
                        .filter((port) => port.status === "idle")
                        .map((port) => String(port.id).padStart(2, "0"))
                        .join("、")} 号口在读取时空闲`
                    : "没有空闲口，可以开启一次临时提醒。"}
          </p>
        </div>

        <div className="wb-availability-action">
          <WorkbenchButton
            variant={count.idle && !activeReminder ? "outline" : "default"}
            disabled={!online || unknown || !watchLoaded || !!watchError}
            onClick={onRemind}
          >
            <BellIcon size={14} />
            {activeReminder ? "管理这次提醒" : "有空闲时提醒我"}
          </WorkbenchButton>
          {!watchLoaded || watchError ? (
            <span className="wb-small wb-muted">
              {watchError || "正在读取提醒状态…"}
              {watchError && (
                <button
                  className="ml-1 underline"
                  onClick={() => void loadWatch().catch(() => undefined)}
                >
                  重试
                </button>
              )}
            </span>
          ) : (
            <span className="wb-small wb-muted">
              找到空闲后通知一次，不预留空位
            </span>
          )}
        </div>
      </div>

      <section className="wb-insp-ports">
        <SectionHeading
          title={unknown ? "充电口" : `全部充电口 · ${pile.ports.length}`}
        >
          {!unknown && (
            <div className="wb-seg-rail wb-seg-sm" role="group" aria-label="筛选端口状态">
              {[
                ["all", "全部"],
                ["idle", `空闲 ${count.idle}`],
                ["in_use", `使用中 ${count.in_use}`],
                ...(count.offline > 0
                  ? [["offline", `离线 ${count.offline}`]]
                  : []),
              ].map(([val, lbl]) => (
                <button
                  key={val}
                  type="button"
                  className={`wb-seg-btn ${
                    portFilter === val ? "wb-seg-active" : ""
                  }`}
                  aria-pressed={portFilter === val}
                  onClick={() =>
                    setPortFilter(val as "all" | "idle" | "in_use" | "offline")
                  }
                >
                  {lbl}
                </button>
              ))}
            </div>
          )}
        </SectionHeading>

        {unknown ? (
          <WorkbenchEmpty
            title="还没有端口状态"
            description="成功读取设备后，端口信息会出现在这里。"
            icon={PlugZapIcon}
          />
        ) : (
          <div className="wb-insp-port-list" role="group" aria-label={`${pile.name}的充电口明细`}>
            {visiblePorts.map((port) => {
              const expanded = portId === port.id
              return (
                <div
                  key={port.id}
                  className={`wb-insp-port-item ${
                    expanded ? "wb-insp-port-focused" : ""
                  }`}
                >
                  <button
                    type="button"
                    className="wb-insp-port-btn"
                    aria-label={`${port.id} 号充电口，${
                      stale ? "上次" : ""
                    }${stateLabel[port.status]}`}
                    aria-expanded={expanded}
                    onClick={() =>
                      updatePageQuery({
                        pile: pile.id,
                        port: expanded ? null : port.id,
                      })
                    }
                  >
                    <span className="wb-mono wb-insp-port-num">
                      {String(port.id).padStart(2, "0")} 号口
                    </span>
                    <StatusPill
                      tone={
                        port.status === "in_use"
                          ? "busy"
                          : port.status === "idle"
                            ? stale
                              ? "neutral"
                              : "idle"
                            : "offline"
                      }
                    >
                      {stale
                        ? `上次${stateLabel[port.status]}`
                        : stateLabel[port.status]}
                    </StatusPill>
                    <span className="wb-insp-port-time">
                      {port.status === "in_use"
                        ? expanded
                          ? "收起"
                          : portTime(port, "remaining")
                        : port.status === "idle"
                          ? stale
                            ? "待确认"
                            : "读取时空闲"
                          : "无法读取"}
                    </span>
                  </button>

                  {expanded && <PortInspector port={port} stale={stale} />}
                </div>
              )
            })}
          </div>
        )}

        <p className="wb-data-note">
          <Clock3Icon size={12} />
          {unknown
            ? "尚无成功读取的快照。"
            : `${formatSnapshotTime(pile.updatedAt, true)} 读取。剩余时间来自平台，请以现场状态为准。`}
        </p>
      </section>

      <Dialog
        open={modal !== null}
        onOpenChange={(open) => !open && !saving && setModal(null)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>
              {modal === "edit" ? "编辑充电桩" : `移除 ${pile.name}？`}
            </DialogTitle>
            <DialogDescription>
              {modal === "edit"
                ? "名称与位置备注只影响你的常用列表。"
                : "移除后关联提醒会结束，不会操作现场设备。"}
            </DialogDescription>
          </DialogHeader>
          {modal === "edit" ? (
            <form onSubmit={save}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="wb-pile-name">名称</FieldLabel>
                  <Input
                    id="wb-pile-name"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    maxLength={40}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="wb-pile-address">位置备注</FieldLabel>
                  <Input
                    id="wb-pile-address"
                    value={address}
                    onChange={(event) => setAddress(event.target.value)}
                    maxLength={80}
                  />
                </Field>
                {formError && (
                  <p role="alert" className="wb-form-error">
                    {formError}
                  </p>
                )}
                <DialogFooter>
                  <WorkbenchButton
                    type="button"
                    variant="outline"
                    disabled={saving}
                    onClick={() => setModal(null)}
                  >
                    取消
                  </WorkbenchButton>
                  <WorkbenchButton
                    type="submit"
                    busy={saving}
                    disabled={!online}
                  >
                    保存更改
                  </WorkbenchButton>
                </DialogFooter>
              </FieldGroup>
            </form>
          ) : (
            <>
              {formError && (
                <p role="alert" className="wb-form-error">
                  {formError}
                </p>
              )}
              <DialogFooter>
                <WorkbenchButton
                  variant="outline"
                  disabled={saving}
                  onClick={() => setModal(null)}
                >
                  取消
                </WorkbenchButton>
                <WorkbenchButton
                  variant="destructive"
                  busy={saving}
                  disabled={!online}
                  onClick={() => void remove()}
                >
                  确认移除
                </WorkbenchButton>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

function PortInspector({ port, stale }: { port: Port; stale: boolean }) {
  return (
    <section className="wb-port-inspector" aria-label={`${port.id} 号口详情`}>
      {stale && (
        <p className="wb-data-note">以下为上次读取的数据，当前状态待确认。</p>
      )}
      {port.status === "in_use" ? (
        <dl className="wb-tele-grid">
          <div>
            <dt>已用时间</dt>
            <dd>{portTime(port, "used")}</dd>
          </div>
          <div>
            <dt>预计剩余</dt>
            <dd>{portTime(port, "remaining")}</dd>
          </div>
          <div>
            <dt>{stale ? "上次功率" : "当前功率"}</dt>
            <dd className="wb-mono">
              <ZapIcon size={12} className="wb-busy-text" />
              {Number.isFinite(port.powerKw) ? port.powerKw.toFixed(2) : "—"} kW
            </dd>
          </div>
          <div>
            <dt>累计用电</dt>
            <dd className="wb-mono">
              {Number.isFinite(port.energyKwh)
                ? port.energyKwh.toFixed(2)
                : "—"}{" "}
              kWh
            </dd>
          </div>
        </dl>
      ) : (
        <p className="wb-small wb-muted">
          {port.status === "idle"
            ? stale
              ? "上次读取为空闲，当前是否可用尚待确认。"
              : "最近读取为空闲，使用前请在现场确认。"
            : "暂时无法读取这个充电口。"}
        </p>
      )}
    </section>
  )
}
