"use client"

import { Fragment, useState } from "react"
import {
  ArrowDown,
  ArrowRight,
  ArrowUp,
  Bell,
  Check,
  ChevronRight,
  Clock3,
  History,
  MapPin,
  MoreHorizontal,
  Pencil,
  PlugZap,
  Plus,
  RefreshCw,
  SlidersHorizontal,
  Trash2,
  WifiOff,
} from "lucide-react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Pile, Port } from "@/lib/types"
import { usePrototype } from "./context"
import {
  counts,
  filterPiles,
  historyBuckets,
  SNAPSHOT_AT,
  statusLabel,
  timeLabel,
} from "./model"
import {
  BackLink,
  Badge,
  EmptyState,
  ErrorState,
  IconButton,
  LoadingState,
  PageHeading,
  PButton,
  SearchField,
  SectionHeading,
} from "./ui"

export function PilesPage() {
  const {
    data,
    route,
    go,
    openModal,
    scenario,
    setScenario,
    offline,
    commit,
    busy,
    notify,
  } = usePrototype()
  const [query, setQuery] = useState("")
  const [filter, setFilter] = useState("all")
  const [ordering, setOrdering] = useState(false)
  const [refreshAt, setRefreshAt] = useState(0)
  const piles = scenario === "empty" ? [] : data.piles
  const filtered = filterPiles(piles, query, filter)
  const selected = route.id ? piles.find((p) => p.id === route.id) : filtered[0]
  const available = piles.filter((p) => p.online && counts(p).idle > 0).length
  const stale =
    offline ||
    scenario === "expired" ||
    scenario === "error" ||
    data.credential === "expired"

  async function refresh() {
    if (
      scenario === "expired" ||
      data.credential === "expired" ||
      data.credential === "unbound"
    ) {
      openModal({ kind: "scan" })
      return
    }
    if (scenario === "quota") {
      notify("今天的远端检查额度已用完，明天会恢复。", "info")
      return
    }
    if (scenario === "power-off") {
      notify("当前处于计划断电时段，供电恢复后再刷新。", "info")
      return
    }
    if (Date.now() - refreshAt < 15_000) {
      notify("刚刚已更新，正在使用最近一次快照。", "info")
      return
    }
    const ok = await commit((d) => {
      // Keep all mock observations on the same explicit sample clock.
      d.snapshotAt = new Date(
        new Date(d.snapshotAt).getTime() + 60_000
      ).toISOString()
      d.piles.forEach((p) => {
        if (p.online) p.updatedAt = d.snapshotAt
      })
    }, "已更新示例快照；离线设备仍保留上次状态。")
    if (ok) setRefreshAt(Date.now())
  }
  function move(index: number, delta: number) {
    void commit((d) => {
      const [item] = d.piles.splice(index, 1)
      d.piles.splice(index + delta, 0, item)
      d.piles.forEach((p, i) => {
        p.sortOrder = i
      })
    }, "常用桩顺序已保存。")
  }

  return (
    <>
      <PageHeading
        title="常用充电桩"
        description={
          stale ? (
            "正在展示最近快照，更新后再确认是否有空闲口。"
          ) : available ? (
            <>
              <span className="cp-inline-status" />
              {available} 处有空闲口，出发前再确认一下。
            </>
          ) : (
            "先看看常去的地方，或让我们在有空闲时提醒你。"
          )
        }
        actions={
          <>
            <PButton onClick={refresh} loading={!!busy} disabled={offline}>
              <RefreshCw size={15} />
              刷新状态
            </PButton>
            <PButton
              tone="brand"
              onClick={() => openModal({ kind: "add" })}
              disabled={offline}
            >
              <Plus size={16} />
              添加充电桩
            </PButton>
          </>
        }
      />
      <div
        className={`cp-piles-workspace ${route.id ? "cp-has-selection" : ""}`}
      >
        <aside className="cp-pile-list" aria-label="常用充电桩列表">
          <div className="cp-list-toolbar">
            <h2>
              我的常用 <span>{piles.length}</span>
            </h2>
            <IconButton
              icon={SlidersHorizontal}
              label={ordering ? "完成排序" : "调整充电桩顺序"}
              onClick={() => setOrdering(!ordering)}
              aria-pressed={ordering}
            />
          </div>
          <SearchField value={query} onChange={setQuery} />
          <div className="cp-filter-line" aria-label="筛选充电桩">
            {[
              ["all", "全部"],
              ["idle", "有空闲"],
              ["busy", "已满"],
              ["offline", "离线"],
            ].map(([value, label]) => (
              <button
                key={value}
                aria-pressed={filter === value}
                onClick={() => {
                  setFilter(value)
                  if (route.id) go({ view: "piles" })
                }}
              >
                {label}
              </button>
            ))}
          </div>
          <div className="cp-pile-objects">
            {scenario === "loading" ? (
              <LoadingState />
            ) : !filtered.length ? (
              <EmptyState
                title={piles.length ? "没有找到充电桩" : "把常去的地方加进来"}
                description={
                  piles.length
                    ? "换个名称、位置或桩号试试。"
                    : "添加第一个充电桩，之后不用再反复扫码查找。"
                }
                action={
                  piles.length ? (
                    <PButton
                      tone="quiet"
                      onClick={() => {
                        setQuery("")
                        setFilter("all")
                      }}
                    >
                      清除筛选
                    </PButton>
                  ) : (
                    <PButton
                      onClick={() => {
                        setScenario("ready")
                        openModal({ kind: "add" })
                      }}
                    >
                      <Plus size={15} />
                      添加充电桩
                    </PButton>
                  )
                }
              />
            ) : (
              filtered.map((p) => {
                const stats = counts(p)
                const activeWatch = data.watches.some(
                  (w) => w.deviceId === p.id && w.status === "active"
                )
                return (
                  <div
                    key={p.id}
                    className={`cp-pile-object ${selected?.id === p.id ? "cp-selected" : ""}`}
                  >
                    <button
                      className="cp-pile-select"
                      onClick={() => go({ view: "piles", id: p.id })}
                      aria-label={`查看 ${p.name}`}
                      aria-current={selected?.id === p.id ? "true" : undefined}
                    >
                      <div className="cp-pile-object-top">
                        <span className="cp-object-icon">
                          <PlugZap size={18} />
                        </span>
                        <span className="cp-pile-name">{p.name}</span>
                        <ChevronRight size={15} />
                      </div>
                      <div className="cp-pile-object-meta">
                        <span className="cp-mono">{p.number}</span>
                        <span>
                          {!p.ports.length
                            ? "尚未读取"
                            : !p.online
                              ? "连接中断"
                              : stale
                                ? "最近快照"
                                : "已连接"}
                        </span>
                      </div>
                      <div className="cp-pile-availability">
                        <span
                          className={
                            !p.online || stale
                              ? "cp-muted"
                              : stats.idle
                                ? "cp-idle-text"
                                : "cp-busy-text"
                          }
                        >
                          {!p.ports.length ? (
                            "等待首次读取"
                          ) : !p.online ? (
                            "暂时无法读取"
                          ) : stale ? (
                            `上次 ${stats.idle} 个空闲`
                          ) : stats.idle ? (
                            <>
                              <strong>{stats.idle}</strong> / {p.ports.length}{" "}
                              个口空闲
                            </>
                          ) : (
                            "全部使用中"
                          )}
                        </span>
                        {activeWatch && (
                          <span className="cp-watching">
                            <Bell size={12} />
                            提醒中
                          </span>
                        )}
                      </div>
                      <div
                        className={`cp-port-strip ${stale ? "cp-stale-strip" : ""}`}
                        aria-hidden="true"
                      >
                        {p.ports.map((port) => (
                          <span
                            className={`cp-port-mark cp-mark-${port.status}`}
                            key={port.id}
                          />
                        ))}
                      </div>
                    </button>
                    {ordering && (
                      <div className="cp-order-actions">
                        <span>调整位置</span>
                        <IconButton
                          label={`上移 ${p.name}`}
                          icon={ArrowUp}
                          disabled={
                            data.piles.indexOf(p) === 0 || !!busy || offline
                          }
                          onClick={() => move(data.piles.indexOf(p), -1)}
                        />
                        <IconButton
                          label={`下移 ${p.name}`}
                          icon={ArrowDown}
                          disabled={
                            data.piles.indexOf(p) === data.piles.length - 1 ||
                            !!busy ||
                            offline
                          }
                          onClick={() => move(data.piles.indexOf(p), 1)}
                        />
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
          <div className="cp-list-foot">
            <Clock3 size={13} />
            <span>快照更新于 {timeLabel(data.snapshotAt)}</span>
            <span className="cp-example-caption">示例</span>
          </div>
        </aside>
        <article className="cp-pile-detail" aria-label="充电桩详情">
          <div className="cp-mobile-back">
            <BackLink onClick={() => go({ view: "piles" })}>
              常用充电桩
            </BackLink>
          </div>
          {scenario === "loading" ? (
            <LoadingState />
          ) : scenario === "error" ? (
            <ErrorState />
          ) : selected ? (
            <PileDetail key={selected.id} pile={selected} stale={stale} />
          ) : route.id ? (
            <EmptyState
              title="这个充电桩不在常用列表中"
              description="可能已经被移除。请从列表重新选择。"
              action={
                <PButton onClick={() => go({ view: "piles" })}>
                  返回列表
                </PButton>
              }
            />
          ) : (
            <EmptyState
              icon={PlugZap}
              title="选一个常去的地方"
              description="这里会显示每个充电口的状态和历史。"
            />
          )}
        </article>
      </div>
    </>
  )
}

function PileDetail({ pile, stale }: { pile: Pile; stale: boolean }) {
  const { route, data, go, openModal, offline } = usePrototype()
  const stats = counts(pile)
  const unobserved = pile.ports.length === 0
  const tab = route.tab === "history" ? "history" : "current"
  const watch = data.watches.find(
    (w) => w.deviceId === pile.id && w.status === "active"
  )
  const selectedPort = pile.ports.find((p) => p.id === route.port)
  return (
    <>
      <header className="cp-detail-header">
        <div className="cp-detail-eyebrow">
          <span className="cp-mono">NO. {pile.number}</span>
          <Badge tone={pile.online ? "neutral" : "offline"}>
            {unobserved
              ? "等待首次读取"
              : stale
                ? "最近快照"
                : pile.online
                  ? "已连接"
                  : "离线"}
          </Badge>
        </div>
        <div className="cp-detail-title">
          <h2>{pile.name}</h2>
          <details className="cp-dropdown">
            <summary aria-label="充电桩操作">
              <MoreHorizontal size={20} />
            </summary>
            <div className="cp-menu">
              <button onClick={() => openModal({ kind: "edit", id: pile.id })}>
                <Pencil size={14} />
                编辑名称与位置
              </button>
              <button
                className="cp-danger-text"
                onClick={() => openModal({ kind: "remove", id: pile.id })}
              >
                <Trash2 size={14} />
                移出常用桩
              </button>
            </div>
          </details>
        </div>
        <p className="cp-address">
          <MapPin size={14} />
          {pile.address || "尚未填写位置"}
        </p>
      </header>
      <div
        className={`cp-availability-banner ${!pile.online ? "cp-unavailable" : ""} ${stale ? "cp-is-stale" : ""}`}
      >
        <div className="cp-availability-copy">
          <span className="cp-availability-label">
            {unobserved
              ? "读取状态"
              : stale
                ? "上次读取"
                : !pile.online
                  ? "连接状态"
                  : "当前空闲"}
          </span>
          <div className="cp-availability-number">
            {!pile.online ? (
              <>
                {unobserved ? <Clock3 size={25} /> : <WifiOff size={25} />}
                <strong>{unobserved ? "尚未读取" : "暂时离线"}</strong>
              </>
            ) : (
              <>
                <strong>{stats.idle.toString().padStart(2, "0")}</strong>
                <span>
                  / {pile.ports.length.toString().padStart(2, "0")} 个充电口
                </span>
              </>
            )}
          </div>
          <p>
            {unobserved
              ? "先成功读取一次，再判断是否有空闲口。"
              : !pile.online
                ? "状态无法确认，请稍后刷新或查看现场供电。"
                : stale
                  ? "快照可能已过期，当前空闲情况尚未确认。"
                  : stats.idle
                    ? `${pile.ports
                        .filter((p) => p.status === "idle")
                        .map((p) => String(p.id).padStart(2, "0"))
                        .join("、")} 号口可以使用`
                    : "还没有空闲口，可以开启一次空闲提醒。"}
          </p>
        </div>
        <div className="cp-availability-action">
          <PButton
            tone={stats.idle && !watch ? "default" : "brand"}
            disabled={offline || !pile.ports.length}
            onClick={() => openModal({ kind: "watch", id: pile.id })}
          >
            <Bell size={16} />
            {watch ? "管理这次提醒" : "有空闲时提醒我"}
          </PButton>
          {watch ? (
            <span>提醒至 {timeLabel(watch.expiresAt)}</span>
          ) : (
            <span>找到空闲后通知一次</span>
          )}
        </div>
      </div>
      <Tabs
        value={tab}
        onValueChange={(value) =>
          go({
            view: "piles",
            id: pile.id,
            tab: String(value),
            port: route.port,
          })
        }
        className="cp-tabs"
      >
        <TabsList
          variant="line"
          className="cp-tabs-list"
          aria-label="充电桩详情"
        >
          <TabsTrigger value="current" className="cp-tab">
            <PlugZap size={15} />
            充电口
          </TabsTrigger>
          <TabsTrigger value="history" className="cp-tab">
            <History size={15} />
            使用历史
          </TabsTrigger>
        </TabsList>
        <TabsContent value="current" className="cp-tab-content">
          <SectionHeading
            title={unobserved ? "充电口" : `全部充电口 · ${pile.ports.length}`}
          >
            {!unobserved && (
              <div className="cp-status-legend">
                <span>
                  <i className="cp-legend-idle" />
                  空闲 {stats.idle}
                </span>
                <span>
                  <i className="cp-legend-busy" />
                  使用中 {stats.in_use}
                </span>
                <span>
                  <i className="cp-legend-offline" />
                  离线 {stats.offline}
                </span>
              </div>
            )}
          </SectionHeading>
          {!pile.ports.length ? (
            <EmptyState
              icon={PlugZap}
              title="还没有端口状态"
              description="新增设备需要首次成功读取。原型不会替未读取的设备生成可用端口。"
            />
          ) : (
            <div
              className="cp-ports-table"
              role="group"
              aria-label={`${pile.name}的充电口`}
            >
              <div className="cp-ports-table-head" aria-hidden="true">
                <span>充电口</span>
                <span>当前状态</span>
                <span>已用时间</span>
                <span>预计剩余</span>
                <span className="cp-sr-only">操作</span>
              </div>
              {pile.ports.map((port) => (
                <Fragment key={port.id}>
                  <button
                    className={`cp-port-row ${selectedPort?.id === port.id ? "cp-port-selected" : ""}`}
                    aria-label={`查看 ${port.id} 号口，${stale ? "上次" : ""}${statusLabel[port.status]}`}
                    aria-description={
                      port.status === "in_use"
                        ? `已用 ${port.usedText}，预计剩余 ${port.remainingText}${stale ? "，以上为过期快照" : ""}`
                        : undefined
                    }
                    onClick={() =>
                      go({
                        view: "piles",
                        id: pile.id,
                        tab: "current",
                        port:
                          selectedPort?.id === port.id ? undefined : port.id,
                      })
                    }
                    aria-expanded={selectedPort?.id === port.id}
                  >
                    <span className="cp-port-number">
                      <span
                        className={`cp-socket cp-socket-${port.status}`}
                        aria-hidden="true"
                      >
                        <i />
                        <i />
                        <i />
                      </span>
                      <strong>{String(port.id).padStart(2, "0")}</strong>
                      <span className="cp-port-unit">号口</span>
                    </span>
                    <Badge
                      tone={port.status === "in_use" ? "busy" : port.status}
                    >
                      {stale
                        ? `上次${statusLabel[port.status]}`
                        : statusLabel[port.status]}
                    </Badge>
                    <span className="cp-port-time">
                      {port.status === "in_use" ? port.usedText : "—"}
                    </span>
                    <span className="cp-port-time cp-remaining">
                      {port.status === "in_use" ? (
                        port.remainingText
                      ) : port.status === "idle" ? (
                        <span className={stale ? "cp-muted" : "cp-idle-text"}>
                          {stale ? "待确认" : "可以使用"}
                        </span>
                      ) : (
                        "无法读取"
                      )}
                    </span>
                    <ChevronRight className="cp-port-chevron" size={14} />
                  </button>
                  {selectedPort?.id === port.id && (
                    <PortDetail port={port} pile={pile} stale={stale} />
                  )}
                </Fragment>
              ))}
            </div>
          )}
          <p className="cp-data-note">
            <Clock3 size={13} />
            {unobserved
              ? "尚无成功读取的快照。"
              : `${timeLabel(pile.updatedAt, true)} 的示例快照。剩余时间来自平台，不保证结束后仍有空位。`}
          </p>
        </TabsContent>
        <TabsContent value="history" className="cp-tab-content">
          <HistoryPanel pile={pile} />
        </TabsContent>
      </Tabs>
    </>
  )
}

function PortDetail({
  port,
  pile,
  stale,
}: {
  port: Port
  pile: Pile
  stale: boolean
}) {
  const { go } = usePrototype()
  return (
    <section className="cp-port-inspector" aria-label={`${port.id} 号口详情`}>
      <div className="cp-inspector-heading">
        <h3>
          {String(port.id).padStart(2, "0")} 号口{" "}
          <Badge tone={port.status === "in_use" ? "busy" : port.status}>
            {stale ? "最近快照" : statusLabel[port.status]}
          </Badge>
        </h3>
        <IconButton
          icon={History}
          label="查看这个充电口的历史"
          onClick={() =>
            go({ view: "piles", id: pile.id, tab: "history", port: port.id })
          }
        />
      </div>
      {port.status === "in_use" ? (
        <dl className="cp-inline-facts">
          <div>
            <dt>已用时间</dt>
            <dd>{port.usedText}</dd>
          </div>
          <div>
            <dt>当前功率</dt>
            <dd>
              {port.powerKw.toFixed(2)} <small>kW</small>
            </dd>
          </div>
          <div>
            <dt>累计用电</dt>
            <dd>
              {port.energyKwh.toFixed(2)} <small>kWh</small>
            </dd>
          </div>
        </dl>
      ) : (
        <p>
          {port.status === "idle"
            ? "最近一次读取为空闲，使用前请在现场确认。"
            : "尚无法读取这个充电口，请稍后刷新。"}
        </p>
      )}
      <button
        className="cp-text-link"
        onClick={() =>
          go({ view: "piles", id: pile.id, tab: "history", port: port.id })
        }
      >
        查看这个口的历史
        <ArrowRight size={14} />
      </button>
    </section>
  )
}

export function HistoryPanel({ pile }: { pile: Pile }) {
  const { route, go } = usePrototype()
  const [range, setRange] = useState("7d")
  const [focused, setFocused] = useState<number | null>(null)
  const points = historyBuckets(range, route.port)
  const selected = focused === null ? null : points[focused]
  const port = pile.ports.find((p) => p.id === route.port)
  const eventPort =
    port ?? pile.ports.find((p) => p.status === "idle") ?? pile.ports[0]
  const idleHours =
    (points.reduce((sum, p) => sum + p.idle * p.observed, 0) / points.length) *
    24
  return (
    <div className="cp-history">
      <div className="cp-history-controls">
        <label className="cp-inline-select">
          <span>查看范围</span>
          <select
            value={route.port ?? "all"}
            onChange={(e) =>
              go({
                view: "piles",
                id: pile.id,
                tab: "history",
                port:
                  e.target.value === "all" ? undefined : Number(e.target.value),
              })
            }
          >
            <option value="all">整桩概览</option>
            {pile.ports.map((p) => (
              <option key={p.id} value={p.id}>
                {String(p.id).padStart(2, "0")} 号口
              </option>
            ))}
          </select>
        </label>
        <div className="cp-segmented" aria-label="历史时间范围">
          {[
            ["24h", "24 小时"],
            ["7d", "7 天"],
            ["30d", "30 天"],
          ].map(([v, label]) => (
            <button
              key={v}
              aria-pressed={range === v}
              onClick={() => {
                setRange(v)
                setFocused(null)
              }}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      {!pile.ports.length ? (
        <EmptyState
          icon={History}
          title="还没有观察记录"
          description="读取到端口状态后，历史记录会从那一刻开始积累。"
        />
      ) : (
        <>
          <div className="cp-history-summary">
            <div>
              <span>
                {port ? `${port.id} 号口` : "全部充电口"} · 日均观察到空闲
              </span>
              <strong>
                {idleHours.toFixed(1)}
                <small> 小时</small>
              </strong>
            </div>
            <p>
              记录覆盖约{" "}
              {Math.round(
                (points.reduce((s, p) => s + p.observed, 0) / points.length) *
                  100
              )}
              %<br />
              <span>未观察到的时段不计为空闲</span>
            </p>
          </div>
          <SectionHeading title="空闲与使用分布">
            <span className="cp-example-caption">示例历史</span>
          </SectionHeading>
          <div
            className="cp-history-chart"
            role="group"
            aria-label={`${range}历史示例，绿为空闲，蓝为使用中，灰为未观察`}
          >
            <div className="cp-chart-y">
              <span>100%</span>
              <span>50%</span>
              <span>0</span>
            </div>
            <div className="cp-chart-plot">
              {points.map((point, i) => (
                <button
                  key={i}
                  className="cp-history-bar"
                  onMouseEnter={() => setFocused(i)}
                  onFocus={() => setFocused(i)}
                  onClick={() => setFocused(i)}
                  aria-label={`${point.label}，空闲占比 ${Math.round(point.idle * point.observed * 100)}%，观察覆盖 ${Math.round(point.observed * 100)}%`}
                  aria-pressed={focused === i}
                >
                  <span className="cp-bar-stack">
                    <i
                      className="cp-bar-unknown"
                      style={{ height: `${point.unknown * 100}%` }}
                    />
                    <i
                      className="cp-bar-busy"
                      style={{ height: `${point.busy * 100}%` }}
                    />
                    <i
                      className="cp-bar-idle"
                      style={{
                        height: `${point.idle * point.observed * 100}%`,
                      }}
                    />
                  </span>
                  <span className="cp-chart-x">
                    {range === "7d" ||
                    i % (range === "24h" ? 6 : 7) === 0 ||
                    i === points.length - 1
                      ? point.label
                      : ""}
                  </span>
                </button>
              ))}
            </div>
          </div>
          <div className="cp-history-legend">
            <div className="cp-status-legend">
              <span>
                <i className="cp-legend-idle" />
                空闲
              </span>
              <span>
                <i className="cp-legend-busy" />
                使用中
              </span>
              <span>
                <i className="cp-legend-offline" />
                未观察
              </span>
            </div>
            <span aria-live="polite">
              {selected
                ? `${selected.label} · 空闲 ${Math.round(selected.idle * selected.observed * 100)}%`
                : "选择柱形查看详情"}
            </span>
          </div>
          <section className="cp-history-observations">
            <SectionHeading
              title={port ? `${port.id} 号口 · 最近状态变化` : "最近状态变化"}
            />
            <div className="cp-timeline-row">
              <span
                className={`cp-timeline-node ${eventPort?.status === "idle" ? "cp-idle-text" : eventPort?.status === "in_use" ? "cp-busy-text" : "cp-muted"}`}
              >
                {eventPort?.status === "idle" ? (
                  <Check size={13} />
                ) : eventPort?.status === "in_use" ? (
                  <PlugZap size={13} />
                ) : (
                  <WifiOff size={13} />
                )}
              </span>
              <div>
                <strong>
                  {eventPort.id} 号口{" "}
                  {eventPort.status === "in_use"
                    ? "开始使用"
                    : eventPort.status === "offline" || !pile.online
                      ? "暂时无法连接"
                      : "变为空闲"}
                </strong>
                <p>
                  {eventPort.status === "offline" || !pile.online
                    ? "当前状态未知，不推断为空闲"
                    : "已记录平台返回的状态变化"}
                </p>
              </div>
              <time>
                {eventPort.status === "in_use"
                  ? timeLabel(
                      new Date(
                        new Date(SNAPSHOT_AT).getTime() -
                          eventPort.usedSeconds * 1000
                      ).toISOString(),
                      true
                    )
                  : "9/10 18:35"}
              </time>
            </div>
            <div className="cp-timeline-row">
              <span className="cp-timeline-node cp-busy-text">
                <PlugZap size={13} />
              </span>
              <div>
                <strong>
                  {eventPort.id} 号口{" "}
                  {eventPort.status === "in_use" ? "此前空闲" : "开始使用"}
                </strong>
                <p>平台返回的上一段状态</p>
              </div>
              <time>9/10 12:10</time>
            </div>
            <div className="cp-timeline-row">
              <span className="cp-timeline-node">
                <Clock3 size={13} />
              </span>
              <div>
                <strong>一段观察从这里开始</strong>
                <p>间隔之外未观测的时段，不推断为空闲</p>
              </div>
              <time>9 月 4 日 09:10</time>
            </div>
          </section>
          <p className="cp-data-note">
            历史只反映已观察的状态，不预测当前或未来空位。时区：亚洲 / 上海。
          </p>
        </>
      )}
    </div>
  )
}
