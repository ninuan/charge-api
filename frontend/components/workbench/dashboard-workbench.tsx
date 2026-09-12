"use client"

import {
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowUpIcon,
  BellIcon,
  ChevronRightIcon,
  Clock3Icon,
  MapPinIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlugZapIcon,
  RefreshCwIcon,
  SlidersHorizontalIcon,
  Trash2Icon,
  WifiOffIcon,
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
import { AppShell } from "@/components/app-shell"
import { AddPileDialog } from "@/components/add-pile-dialog"
import { UsageGuideDialog } from "@/components/usage-guide-dialog"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field"
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
  SectionHeading,
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
  const { rules } = useWatch()
  const { unreadCount } = useNotifications()
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
    : entries[0]?.pile
  const stale =
    !online ||
    !!error ||
    !!refreshError ||
    streamState === "error" ||
    snapshot.refresh.partial ||
    snapshot.refresh.failedDevices > 0
  const available = snapshot.piles.filter(
    (p) => p.online && portCounts(p).idle > 0
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
    const ids = snapshot.piles.map((p) => p.id),
      index = ids.indexOf(id),
      next = index + direction
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
    const rule = activeRules.find((rule) => rule.deviceId === pile.id)
    if (rule)
      router.push(`/dashboard/reminders?pile=${encodeURIComponent(pile.id)}`)
    else setWatchPileId(pile.id)
  }
  const actions = (
    <>
      <WorkbenchButton
        variant="outline"
        busy={refreshing}
        disabled={!online || !currentUser?.refreshEnabled || initialLoading}
        onClick={() => void refresh()}
      >
        <RefreshCwIcon size={15} />
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
  return (
    <AppShell
      title="常用充电桩"
      activeSection="piles"
      description={
        initialLoading
          ? "正在读取常去的地方。"
          : stale
            ? "显示最近快照，更新后再确认空闲情况。"
            : available
              ? `最近读取：${available} 处有空闲口，出发前再确认一下。`
              : "看看常去的地方，或在有空闲时提醒我。"
      }
      actions={actions}
      counts={{ reminders: activeRules.length, notifications: unreadCount }}
      searchItems={searchItems}
      guideAction={
        <UsageGuideDialog
          key={params.has("guide") ? "open" : "closed"}
          initialOpen={params.get("guide") === "1"}
        />
      }
    >
      {!!refreshError && (
        <div className="wb-state-banner wb-banner-warning" role="alert">
          <WifiOffIcon size={17} />
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
      <div
        className={`wb-piles-workspace ${selectedId ? "wb-has-selection" : ""}`}
      >
        <aside className="wb-pile-list" aria-label="常用充电桩列表">
          <div className="wb-list-toolbar">
            <h2>
              我的常用{" "}
              <span>{initialLoading ? "—" : snapshot.piles.length}</span>
            </h2>
            <WorkbenchButton
              variant="ghost"
              className="wb-icon-button"
              aria-label={ordering ? "完成排序" : "调整充电桩顺序"}
              aria-pressed={ordering}
              onClick={() => setOrdering((v) => !v)}
            >
              <SlidersHorizontalIcon size={16} />
            </WorkbenchButton>
          </div>
          <div data-slot="dashboard-search-toolbar">
            <WorkbenchSearch
              value={search}
              onChange={(value) =>
                updatePageQuery(
                  { q: value, pile: null, port: null, detail: null },
                  true
                )
              }
            />
            <div className="wb-filter-line" aria-label="筛选充电口">
              {[
                ["all", "全部"],
                ["idle", "有空闲"],
                ["charging", "使用中"],
                ["offline", "离线"],
              ].map(([value, label]) => (
                <button
                  key={value}
                  aria-pressed={query.filter === value}
                  onClick={() =>
                    updatePageQuery({
                      status: value === "all" ? null : value,
                      pile: null,
                      port: null,
                      detail: null,
                    })
                  }
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div className="wb-pile-objects">
            {initialLoading && !snapshot.piles.length ? (
              <WorkbenchLoading label="正在读取充电桩" />
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
              entries.map(({ pile }) => {
                const count = portCounts(pile),
                  active = activeRules.some(
                    (rule) => rule.deviceId === pile.id
                  ),
                  index = snapshot.piles.indexOf(pile)
                return (
                  <div
                    key={pile.id}
                    className={`wb-pile-object ${selected?.id === pile.id ? "wb-selected" : ""}`}
                  >
                    <button
                      className="wb-pile-select"
                      aria-label={`查看 ${pile.name || pile.number}`}
                      aria-current={
                        selected?.id === pile.id ? "true" : undefined
                      }
                      onClick={() =>
                        updatePageQuery({
                          pile: pile.id,
                          port: null,
                          detail: null,
                        })
                      }
                    >
                      <span className="wb-pile-object-top">
                        <span className="wb-object-icon">
                          <PlugZapIcon size={18} />
                        </span>
                        <span className="wb-pile-name">
                          {pile.name || pile.number}
                        </span>
                        <ChevronRightIcon size={14} />
                      </span>
                      <span className="wb-pile-object-meta">
                        <span className="wb-mono">
                          {pile.number || pile.id.slice(-8)}
                        </span>
                        <span>
                          {!pile.ports.length
                            ? "尚未读取"
                            : !pile.online
                              ? "无法连接"
                              : stale
                                ? "最近快照"
                                : "已读取"}
                        </span>
                      </span>
                      <span className="wb-pile-availability">
                        <span
                          className={
                            pile.online && !stale
                              ? count.idle
                                ? "wb-idle-text"
                                : "wb-busy-text"
                              : "wb-muted"
                          }
                        >
                          {!pile.ports.length ? (
                            "等待首次读取"
                          ) : !pile.online ? (
                            "暂时无法读取"
                          ) : stale ? (
                            `上次 ${count.idle} 个空闲`
                          ) : (
                            <>
                              <strong>{count.idle}</strong> /{" "}
                              {pile.ports.length} 个口空闲
                            </>
                          )}
                        </span>
                        {active && (
                          <span className="wb-watching">
                            <BellIcon size={12} />
                            提醒中
                          </span>
                        )}
                      </span>
                      <span
                        className={`wb-port-strip ${stale || !pile.online ? "wb-stale-strip" : ""}`}
                        aria-hidden="true"
                      >
                        {pile.ports.map((port) => (
                          <i
                            key={port.id}
                            className={`wb-port-mark wb-mark-${port.status}`}
                          />
                        ))}
                      </span>
                    </button>
                    {ordering && (
                      <div className="wb-order-actions">
                        <span>调整位置</span>
                        <WorkbenchButton
                          className="wb-icon-button"
                          variant="ghost"
                          aria-label={`上移 ${pile.name}`}
                          disabled={!online || reordering || index === 0}
                          onClick={() => void move(pile.id, -1)}
                        >
                          <ArrowUpIcon size={14} />
                        </WorkbenchButton>
                        <WorkbenchButton
                          className="wb-icon-button"
                          variant="ghost"
                          aria-label={`下移 ${pile.name}`}
                          disabled={
                            !online ||
                            reordering ||
                            index === snapshot.piles.length - 1
                          }
                          onClick={() => void move(pile.id, 1)}
                        >
                          <ArrowDownIcon size={14} />
                        </WorkbenchButton>
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
          <div className="wb-list-foot">
            <Clock3Icon size={13} />
            <span>
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
        <article className="wb-pile-detail" aria-label="充电桩详情">
          <div className="wb-mobile-back">
            <button
              className="wb-back"
              onClick={() =>
                updatePageQuery({ pile: null, port: null, detail: null })
              }
            >
              <ArrowLeftIcon size={15} />
              常用充电桩
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
          ) : selectedId ? (
            <WorkbenchEmpty
              title="这个充电桩不在常用列表中"
              description="可能已经被移除，请从列表重新选择或重新加载。"
              action={
                <WorkbenchButton variant="outline" onClick={() => void retry()}>
                  重新加载
                </WorkbenchButton>
              }
            />
          ) : (
            <WorkbenchEmpty
              icon={PlugZapIcon}
              title="选择一个常去的地方"
              description="这里会展示最近读取的充电口状态和使用情况。"
            />
          )}
        </article>
      </div>
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
  const { load: loadWatch } = useWatch()
  const online = useOnline()
  const [modal, setModal] = useState<"edit" | "remove" | null>(null)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState("")
  const [name, setName] = useState(pile.name)
  const [address, setAddress] = useState(pile.address)
  const count = portCounts(pile),
    unknown = pile.ports.length === 0
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
    <>
      <header className="wb-detail-header">
        <div className="wb-detail-eyebrow">
          <span className="wb-mono">NO. {pile.number || pile.id}</span>
          <StatusPill tone={!pile.online ? "offline" : "neutral"}>
            {unknown
              ? "尚未读取"
              : stale
                ? "最近快照"
                : pile.online
                  ? "已读取"
                  : "离线"}
          </StatusPill>
        </div>
        <div className="wb-detail-title">
          <h2>{pile.name || pile.number}</h2>
          <details className="wb-dropdown">
            <summary aria-label="充电桩操作">
              <MoreHorizontalIcon size={20} />
            </summary>
            <div className="wb-menu">
              <button
                disabled={!online}
                onClick={() => {
                  setName(pile.name)
                  setAddress(pile.address)
                  setFormError("")
                  setModal("edit")
                }}
              >
                <PencilIcon size={14} />
                编辑名称与位置
              </button>
              <button
                className="wb-danger-text"
                disabled={!online}
                onClick={() => {
                  setFormError("")
                  setModal("remove")
                }}
              >
                <Trash2Icon size={14} />
                移出常用桩
              </button>
            </div>
          </details>
        </div>
        <p className="wb-address">
          <MapPinIcon size={14} />
          {pile.address || "尚未填写位置"}
        </p>
      </header>
      <div
        className={`wb-availability-banner ${!pile.online ? "wb-unavailable" : ""} ${stale ? "wb-is-stale" : ""}`}
      >
        <div className="wb-availability-copy">
          <span className="wb-availability-label">
            {unknown ? "读取状态" : stale ? "上次读取" : "最近空闲"}
          </span>
          <div className="wb-availability-number">
            {unknown || !pile.online ? (
              <>
                {unknown ? <Clock3Icon size={24} /> : <WifiOffIcon size={24} />}
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
          <p>
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
            disabled={!online || unknown}
            onClick={onRemind}
          >
            <BellIcon size={16} />
            {activeReminder ? "管理这次提醒" : "有空闲时提醒我"}
          </WorkbenchButton>
          <span>找到空闲后通知一次，不预留空位</span>
        </div>
      </div>
      <section className="wb-tab-content">
        <SectionHeading
          title={unknown ? "充电口" : `全部充电口 · ${pile.ports.length}`}
        >
          {!unknown && (
            <div className="wb-status-legend">
              <span>
                <i className="wb-legend-idle" />
                空闲 {count.idle}
              </span>
              <span>
                <i className="wb-legend-busy" />
                使用中 {count.in_use}
              </span>
              <span>
                <i className="wb-legend-offline" />
                离线 {count.offline}
              </span>
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
          <div
            className="wb-ports-table"
            role="group"
            aria-label={`${pile.name}的充电口`}
          >
            <div className="wb-ports-table-head" aria-hidden="true">
              <span>充电口</span>
              <span>状态</span>
              <span>已用时间</span>
              <span>预计剩余</span>
              <span />
            </div>
            {pile.ports.map((port) => (
              <Fragment key={port.id}>
                <button
                  className={`wb-port-row ${portId === port.id ? "wb-port-selected" : ""}`}
                  aria-label={`${port.id} 号充电口，${stale ? "上次" : ""}${stateLabel[port.status]}`}
                  aria-expanded={portId === port.id}
                  onClick={() =>
                    updatePageQuery({
                      pile: pile.id,
                      port: portId === port.id ? null : port.id,
                    })
                  }
                >
                  <span className="wb-port-number">
                    <span
                      className={`wb-socket wb-socket-${port.status}`}
                      aria-hidden="true"
                    >
                      <i />
                      <i />
                      <i />
                    </span>
                    <strong>{String(port.id).padStart(2, "0")}</strong>
                    <span className="wb-port-unit">号口</span>
                  </span>
                  <StatusPill
                    tone={port.status === "in_use" ? "busy" : port.status}
                  >
                    {stale
                      ? `上次${stateLabel[port.status]}`
                      : stateLabel[port.status]}
                  </StatusPill>
                  <span className="wb-port-time">{portTime(port, "used")}</span>
                  <span className="wb-port-time wb-remaining">
                    {port.status === "in_use"
                      ? portTime(port, "remaining")
                      : port.status === "idle"
                        ? stale
                          ? "待确认"
                          : "读取时空闲"
                        : "无法读取"}
                  </span>
                  <ChevronRightIcon size={14} className="wb-port-chevron" />
                </button>
                {portId === port.id && (
                  <PortInspector port={port} stale={stale} />
                )}
              </Fragment>
            ))}
          </div>
        )}
        <p className="wb-data-note">
          <Clock3Icon size={13} />
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
    </>
  )
}
function PortInspector({ port, stale }: { port: Port; stale: boolean }) {
  return (
    <section className="wb-port-inspector" aria-label={`${port.id} 号口详情`}>
      <div className="wb-inspector-heading">
        <h3>{String(port.id).padStart(2, "0")} 号口</h3>
      </div>
      {stale && (
        <p className="wb-data-note">以下为上次读取的数据，当前状态待确认。</p>
      )}
      {port.status === "in_use" ? (
        <dl className="wb-inline-facts">
          <div>
            <dt>已用时间</dt>
            <dd>{portTime(port, "used")}</dd>
          </div>
          <div>
            <dt>{stale ? "上次功率" : "当前功率"}</dt>
            <dd>
              {Number.isFinite(port.powerKw) ? port.powerKw.toFixed(2) : "—"}
              <small> kW</small>
            </dd>
          </div>
          <div>
            <dt>累计用电</dt>
            <dd>
              {Number.isFinite(port.energyKwh)
                ? port.energyKwh.toFixed(2)
                : "—"}
              <small> kWh</small>
            </dd>
          </div>
        </dl>
      ) : (
        <p>
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
