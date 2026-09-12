"use client"

import { useState, type FormEvent } from "react"
import {
  ArrowRight,
  Bell,
  Check,
  CheckCheck,
  CircleAlert,
  Clock3,
  Copy,
  Link2,
  QrCode,
  RefreshCw,
  Search,
  ShieldCheck,
  Trash2,
} from "lucide-react"
import { usePrototype } from "./context"
import {
  adminViews,
  counts,
  durationLabel,
  initialData,
  timeLabel,
  validateIdentifier,
  viewLabel,
  watchEnd,
  type Duration,
  type Role,
  type Route,
  type Scenario,
  type View,
} from "./model"
import {
  EmptyState,
  Field,
  FormError,
  Modal,
  ModalActions,
  PButton,
  PInput,
  SaveButton,
} from "./ui"

export function PrototypeDialogs() {
  const { modal } = usePrototype()
  if (!modal) return null
  if (modal.kind === "add" || modal.kind === "edit")
    return <PileForm key={`${modal.kind}-${modal.id}`} />
  if (modal.kind === "watch") return <WatchForm key={modal.id || "new-watch"} />
  if (modal.kind === "preview") return <PreviewDialog />
  if (modal.kind === "search") return <SearchDialog />
  if (modal.kind === "scan" || modal.kind === "wx-bind")
    return <ScanDialog key={modal.kind} />
  if (modal.kind === "cookie") return <CookieDialog />
  if (modal.kind === "password") return <PasswordDialog />
  if (modal.kind === "create-user") return <CreateUserDialog />
  if (modal.kind === "reset-password") return <ResetPasswordDialog />
  if (modal.kind === "invite") return <InviteDialog />
  if (modal.kind === "guide") return <GuideDialog />
  return <ConfirmationDialog />
}

function PileForm() {
  const { data, modal, openModal, scenario, setScenario, commit, go } =
    usePrototype()
  const existing = data.piles.find((p) => p.id === modal?.id)
  const editing = modal?.kind === "edit"
  const [identifier, setIdentifier] = useState(existing?.number || "")
  const [idType, setIdType] = useState("number")
  const [name, setName] = useState(existing?.name || "")
  const [address, setAddress] = useState(existing?.address || "")
  const [error, setError] = useState("")
  const unbound =
    data.credential === "unbound" ||
    data.credential === "expired" ||
    scenario === "expired"
  async function submit(e: FormEvent) {
    e.preventDefault()
    const message = editing
      ? ""
      : validateIdentifier(identifier, scenario === "empty" ? [] : data.piles)
    if (message) {
      setError(message)
      return
    }
    if (
      !editing &&
      scenario !== "empty" &&
      data.piles.length >= data.settings.defaultDeviceLimit
    ) {
      setError(
        `已达到 ${data.settings.defaultDeviceLimit} 台设备上限，请先移除不需要的充电桩。`
      )
      return
    }
    if (name.trim().length > 40 || address.trim().length > 80) {
      setError("名称不超过 40 字，位置不超过 80 字。")
      return
    }
    const id = existing?.id || identifier.trim()
    const success = await commit(
      (d) => {
        if (editing && existing) {
          const p = d.piles.find((p) => p.id === existing.id)
          if (p) {
            p.name = name.trim() || `充电桩 ${identifier}`
            p.address = address.trim()
          }
        } else {
          if (scenario === "empty") {
            d.piles = []
            d.watches = []
            d.notices = []
          }
          d.piles.push({
            id,
            number: identifier.trim(),
            name: name.trim() || `充电桩 ${identifier.trim()}`,
            address: address.trim(),
            status: "waiting",
            openNum: 0,
            online: false,
            createdAt: d.snapshotAt,
            updatedAt: d.snapshotAt,
            source: "prototype-fixture",
            ports: [],
            usedPortIds: [],
            sortOrder: d.piles.length,
          })
        }
      },
      editing
        ? "充电桩信息已保存。"
        : "已加入常用桩。首次读取前不会显示空闲数量。"
    )
    if (success) {
      setScenario("ready")
      go({ view: "piles", id })
      openModal(null)
    }
  }
  return (
    <Modal
      title={editing ? "编辑充电桩" : "添加常用充电桩"}
      description={
        editing
          ? "名称与位置只用于帮助你识别这个充电桩。"
          : "输入设备上的桩号，把常去的地方留在列表里。"
      }
    >
      {unbound && !editing ? (
        <div className="cp-modal-body">
          <EmptyState
            icon={Link2}
            title="先连接充电平台"
            description="完成扫码绑定后，才能验证并读取这个充电桩。"
            action={
              <PButton
                tone="brand"
                onClick={() => openModal({ kind: "scan", returnTo: "add" })}
              >
                <QrCode size={15} />
                去扫码连接
              </PButton>
            }
          />
        </div>
      ) : (
        <form onSubmit={submit} className="cp-modal-form">
          {!editing && (
            <div className="cp-segmented">
              <button
                type="button"
                aria-pressed={idType === "number"}
                onClick={() => setIdType("number")}
              >
                使用桩号
              </button>
              <button
                type="button"
                aria-pressed={idType === "id"}
                onClick={() => setIdType("id")}
              >
                使用设备 ID
              </button>
            </div>
          )}
          <Field
            label={idType === "number" ? "桩号" : "设备 ID"}
            hint="6–64 位数字；原型不会向远端查询。"
          >
            <PInput
              value={identifier}
              disabled={editing}
              onChange={(e) => setIdentifier(e.target.value)}
              inputMode="numeric"
              placeholder="例如 608205"
              required
              autoComplete="off"
            />
          </Field>
          <Field label="名称" hint="可选，起一个你容易记住的名字。">
            <PInput
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如 桂园 · 西门车棚"
              maxLength={40}
            />
          </Field>
          <Field label="位置备注">
            <PInput
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              placeholder="例如 西门内侧第二排"
              maxLength={80}
            />
          </Field>
          <FormError message={error} />
          <p className="cp-form-hint">
            <ShieldCheck size={14} />
            仅添加你有权访问的设备。
          </p>
          <ModalActions>
            <SaveButton>{editing ? "保存更改" : "添加到常用桩"}</SaveButton>
          </ModalActions>
        </form>
      )}
    </Modal>
  )
}

function WatchForm() {
  const { data, modal, openModal, commit, go, scenario, offline, busy } =
    usePrototype()
  const [deviceId, setDeviceId] = useState(
    modal?.id ||
      data.piles.find((p) => p.online && counts(p).idle === 0)?.id ||
      data.piles[0]?.id ||
      ""
  )
  const [duration, setDuration] = useState<Duration>(
    data.watches.find((w) => w.deviceId === deviceId && w.status === "active")
      ?.duration || "2h"
  )
  const [error, setError] = useState("")
  const pile = data.piles.find((p) => p.id === deviceId)
  const active = data.watches.find(
    (w) => w.deviceId === deviceId && w.status === "active"
  )
  const knownAvailable =
    pile?.online &&
    counts(pile).idle > 0 &&
    !offline &&
    scenario !== "expired" &&
    data.credential === "healthy"
  const activeCount = data.watches.filter((w) => w.status === "active").length
  const blockedReason = !data.settings.backgroundRemindersEnabled
    ? "管理员暂时关闭了后台提醒。"
    : data.credential === "unbound" ||
        data.credential === "expired" ||
        scenario === "expired"
      ? "平台登录需要恢复，请先重新扫码。"
      : scenario === "power-off"
        ? "当前处于计划断电时段，供电恢复后再开启提醒。"
        : scenario === "quota"
          ? "今日远端检查额度已用完，明天会恢复。"
          : !active && activeCount >= data.settings.watchPileLimitPerUser
            ? `你已经在等待 ${activeCount} 台充电桩，先结束一条提醒再继续。`
            : !pile?.ports.length
              ? "这个充电桩还没有成功读取过端口状态。"
              : ""
  const end = watchEnd(
    duration,
    data.snapshotAt,
    data.settings.scheduledPowerOffEnabled,
    data.settings.scheduledPowerOffStartMinute
  )
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!pile || blockedReason) {
      setError(blockedReason || "请先选择充电桩。")
      return
    }
    if (new Date(end).getTime() <= new Date(data.snapshotAt).getTime()) {
      setError("已到当天计划断电时间，供电恢复后再开启提醒。")
      return
    }
    const ok = await commit(
      (d) => {
        const found = d.watches.find(
          (w) => w.deviceId === deviceId && w.status === "active"
        )
        if (found) {
          found.duration = duration
          found.expiresAt = end
        } else
          d.watches.unshift({
            id: `watch-${Date.now()}`,
            deviceId,
            duration,
            status: "active",
            createdAt: d.snapshotAt,
            expiresAt: end,
            nextCheckAt: new Date(
              new Date(d.snapshotAt).getTime() +
                d.settings.watchRefreshIntervalMinutes * 60_000
            ).toISOString(),
          })
      },
      active ? "提醒时长已更新。" : `已开始等待${pile.name}，有空闲时通知一次。`
    )
    if (ok) openModal(null)
  }
  return (
    <Modal
      title={active ? "管理这次提醒" : "有空闲时，提醒我"}
      description="关注整台充电桩，任意一个口空闲就通知你。"
    >
      <form className="cp-modal-form" onSubmit={submit}>
        <Field label="充电桩">
          <select
            className="cp-input"
            value={deviceId}
            onChange={(e) => {
              setDeviceId(e.target.value)
              setError("")
            }}
            disabled={!!modal?.id}
          >
            {!data.piles.length && <option value="">请先添加充电桩</option>}
            {data.piles.map((p) => (
              <option value={p.id} key={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </Field>
        {knownAvailable && !active ? (
          <div className="cp-already-available">
            <span>
              <CheckCheck size={23} />
            </span>
            <h3>现在就有 {counts(pile!).idle} 个空闲口</h3>
            <p>可以直接查看状态，不需要再开启后台等待。</p>
            <PButton
              tone="brand"
              onClick={() => go({ view: "piles", id: deviceId })}
            >
              查看空闲口
              <ArrowRight size={15} />
            </PButton>
          </div>
        ) : (
          <>
            <fieldset className="cp-duration-field">
              <legend>这次等多久？</legend>
              <div className="cp-duration-options">
                {(Object.entries(durationLabel) as [Duration, string][])
                  .filter(
                    ([v]) =>
                      v !== "until_power_off" ||
                      data.settings.scheduledPowerOffEnabled
                  )
                  .map(([v, label]) => (
                    <label key={v}>
                      <input
                        type="radio"
                        name="duration"
                        value={v}
                        checked={duration === v}
                        onChange={() => setDuration(v)}
                      />
                      <span>
                        {label}
                        {v === "2h" && <small>常用</small>}
                      </span>
                    </label>
                  ))}
              </div>
            </fieldset>
            <div className="cp-watch-estimate">
              <Clock3 size={16} />
              <p>
                持续到 {timeLabel(end)}
                <span>
                  约每 {data.settings.watchRefreshIntervalMinutes} 分钟检查一次
                  · 空闲后自动结束
                </span>
              </p>
            </div>
            <div className="cp-watch-channels">
              <Check size={14} />
              站内通知{data.wxBound && data.wxEnabled ? " + 微信提醒" : ""}
              <button
                type="button"
                className="cp-text-link"
                onClick={() => go({ view: "account", tab: "notifications" })}
              >
                接收设置
                <ArrowRight size={12} />
              </button>
            </div>
            {blockedReason && <FormError message={blockedReason} />}
            <FormError message={error} />
            <p className="cp-form-hint">提醒不会预留空位；请以现场状态为准。</p>
            <ModalActions>
              {active && (
                <PButton
                  tone="quiet"
                  disabled={offline || !!busy}
                  onClick={async () => {
                    if (
                      await commit((d) => {
                        const w = d.watches.find((x) => x.id === active.id)
                        if (w) w.status = "cancelled"
                      }, "这次提醒已取消。")
                    )
                      openModal(null)
                  }}
                >
                  结束提醒
                </PButton>
              )}
              <PButton
                type="submit"
                tone="brand"
                disabled={!!blockedReason || offline}
                loading={!!busy}
              >
                <Bell size={15} />
                {active ? "更新提醒" : "开始等待"}
              </PButton>
            </ModalActions>
          </>
        )}
      </form>
    </Modal>
  )
}

function PreviewDialog() {
  const { data, mutate, scenario, setScenario, go, openModal, reset, notify } =
    usePrototype()
  const [role, setRole] = useState<Role>(data.demoRole)
  const [state, setState] = useState<Scenario>(scenario)
  const scenarios: [Scenario, string][] = [
    ["ready", "正常数据"],
    ["loading", "加载中"],
    ["empty", "没有数据"],
    ["error", "请求失败"],
    ["offline", "网络离线"],
    ["expired", "平台登录失效"],
    ["power-off", "计划断电"],
    ["quota", "今日额度用完"],
  ]
  function apply() {
    mutate((d) => {
      d.demoRole = role
    })
    setScenario(state)
    if (role !== data.demoRole)
      go({
        view: role === "admin" ? "admin" : role === "guest" ? "login" : "piles",
      })
    openModal(null)
  }
  function event(kind: "available" | "recovered") {
    let performed = false
    mutate((d) => {
      if (kind === "available") {
        const w = d.watches.find((w) => w.status === "active")
        const pile = d.piles.find((p) => p.id === w?.deviceId)
        if (!w || !pile || !pile.ports.length) return
        w.status = "notified"
        pile.online = true
        pile.ports[0] = {
          ...pile.ports[0],
          status: "idle",
          powerKw: 0,
          energyKwh: 0,
          usedSeconds: 0,
          usedText: undefined,
          remainingText: undefined,
        }
        pile.usedPortIds = pile.ports
          .filter((p) => p.status === "in_use")
          .map((p) => p.id)
        d.notices.unshift({
          id: `notice-${Date.now()}`,
          type: "pile_available",
          deviceId: pile.id,
          portId: pile.ports[0].id,
          title: `${pile.name}有空闲口了`,
          message: `${pile.ports[0].id} 号口已空闲，这次提醒已自动结束。出发前请再确认状态。`,
          createdAt: d.snapshotAt,
          read: false,
          resolved: false,
          delivery: d.wxEnabled && d.wxBound ? "submitted" : "none",
        })
        performed = true
      } else {
        const pile = d.piles.find((p) => !p.online && p.ports.length)
        if (!pile) return
        pile.online = true
        pile.status = "online"
        pile.ports.forEach((p) => {
          p.status = "idle"
        })
        d.notices
          .filter((n) => n.deviceId === pile.id && n.type === "pile_offline")
          .forEach((n) => {
            n.resolved = true
          })
        d.notices.unshift({
          id: `notice-${Date.now()}`,
          type: "pile_recovered",
          deviceId: pile.id,
          title: `${pile.name}恢复连接`,
          message: "已经重新读取到端口状态，原离线问题自动标记为解决。",
          createdAt: d.snapshotAt,
          read: false,
          resolved: false,
          delivery: "none",
        })
        performed = true
      }
    })
    if (performed) {
      setScenario("ready")
      go({ view: "notifications" })
      notify(
        kind === "available"
          ? "已模拟发现空闲：提醒自动结束，并生成通知。"
          : "已模拟恢复连接：原问题已解决，并保留通知记录。"
      )
    } else
      notify(
        kind === "available"
          ? "请先创建一条正在等待的提醒。"
          : "目前没有可恢复的离线示例设备。",
        "info"
      )
  }
  function firstObservation() {
    let deviceId = ""
    mutate((d) => {
      const pile = d.piles.find((p) => p.ports.length === 0)
      if (!pile) return
      const sample = initialData().piles[0]
      deviceId = pile.id
      pile.ports = sample.ports.map((port) => ({
        ...port,
        updatedAt: d.snapshotAt,
      }))
      pile.openNum = pile.ports.length
      pile.usedPortIds = pile.ports
        .filter((port) => port.status === "in_use")
        .map((port) => port.id)
      pile.online = true
      pile.status = "online"
      pile.updatedAt = d.snapshotAt
    })
    if (deviceId) {
      setScenario("ready")
      go({ view: "piles", id: deviceId })
      notify("已模拟首次读取，展示明确标注的示例端口数据。")
    }
  }
  return (
    <Modal
      title="原型预览设置"
      description="数据均为示例。所有操作只保存在此浏览器，不会访问真实设备、账户或第三方通知渠道。"
    >
      <div className="cp-modal-form">
        <Field label="预览角色">
          <select
            className="cp-input"
            value={role}
            onChange={(e) => setRole(e.target.value as Role)}
          >
            <option value="user">普通用户</option>
            <option value="admin">管理员</option>
            <option value="guest">未登录</option>
          </select>
        </Field>
        <Field label="页面状态">
          <select
            className="cp-input"
            value={state}
            onChange={(e) => setState(e.target.value as Scenario)}
          >
            {scenarios.map(([v, label]) => (
              <option value={v} key={v}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <div className="cp-preview-events">
          <h3>走通状态变化</h3>
          <PButton onClick={() => event("available")}>
            <Bell size={15} />
            模拟发现空闲口
            <ArrowRight size={14} />
          </PButton>
          <PButton onClick={() => event("recovered")}>
            <Link2 size={15} />
            模拟离线设备恢复
            <ArrowRight size={14} />
          </PButton>
          <PButton
            disabled={!data.piles.some((p) => p.ports.length === 0)}
            onClick={firstObservation}
          >
            <RefreshCw size={15} />
            模拟新桩首次读取
            <ArrowRight size={14} />
          </PButton>
        </div>
        <div className="cp-preview-reset">
          <span>重置仅恢复此原型的示例数据。</span>
          <PButton tone="quiet" onClick={reset}>
            <RefreshCw size={14} />
            重置原型
          </PButton>
        </div>
        <ModalActions>
          <PButton tone="brand" onClick={apply}>
            应用预览
            <Check size={15} />
          </PButton>
        </ModalActions>
      </div>
    </Modal>
  )
}

function SearchDialog() {
  const { data, go } = usePrototype()
  const [query, setQuery] = useState("")
  const [active, setActive] = useState(0)
  const isAdmin = data.demoRole === "admin"
  const destinations = Object.entries(viewLabel).filter(
    ([v]) =>
      !["login", "register"].includes(v) &&
      (v === "account" || adminViews.has(v as View) === isAdmin)
  )
  const results: { label: string; meta: string; route: Route }[] = [
    ...(!isAdmin
      ? data.piles.map((p) => ({
          label: p.name,
          meta: `充电桩 · ${p.number}`,
          route: { view: "piles" as const, id: p.id },
        }))
      : data.users.map((u) => ({
          label: u.username,
          meta: "用户",
          route: { view: "users" as const, id: u.id },
        }))),
    ...destinations.map(([view, label]) => ({
      label,
      meta: "页面",
      route: { view: view as View },
    })),
  ]
    .filter((r) =>
      `${r.label}${r.meta}`.toLowerCase().includes(query.toLowerCase())
    )
    .slice(0, 9)
  return (
    <Modal title="快速定位" description="搜索充电桩、用户或页面。" wide>
      <div className="cp-command">
        <div className="cp-command-input">
          <Search size={19} />
          <PInput
            aria-label="快速搜索"
            placeholder="输入名称、桩号或页面…"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setActive(0)
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault()
                setActive((i) => Math.min(i + 1, results.length - 1))
              }
              if (e.key === "ArrowUp") {
                e.preventDefault()
                setActive((i) => Math.max(0, i - 1))
              }
              if (e.key === "Enter" && results[active]) {
                e.preventDefault()
                go(results[active].route)
              }
            }}
          />
        </div>
        <div className="cp-command-results">
          {results.length ? (
            results.map((r, i) => (
              <button
                key={`${r.route.view}-${r.route.id}`}
                className={i === active ? "cp-command-active" : ""}
                onClick={() => go(r.route)}
                onFocus={() => setActive(i)}
              >
                <span>
                  {r.label}
                  <small>{r.meta}</small>
                </span>
                <ArrowRight size={15} />
              </button>
            ))
          ) : (
            <EmptyState
              title="没有找到结果"
              description="换个关键词，或者直接使用页面导航。"
            />
          )}
        </div>
        <div className="cp-command-footer">
          <span>↑ ↓ 选择</span>
          <span>Enter 打开</span>
          <span>Esc 关闭</span>
        </div>
      </div>
    </Modal>
  )
}

function ScanDialog() {
  const { modal, commit, openModal, setScenario, offline, busy } =
    usePrototype()
  const wx = modal?.kind === "wx-bind"
  const [status, setStatus] = useState<
    "waiting" | "success" | "expired" | "failed"
  >("waiting")
  async function complete() {
    const success = await commit(
      (d) => {
        if (wx) {
          d.wxBound = true
          d.wxEnabled = true
        } else {
          d.credential = d.piles.length ? "healthy" : "waiting_device"
          d.notices
            .filter((n) => n.type === "credential_expired")
            .forEach((n) => {
              n.resolved = true
            })
        }
      },
      wx ? "已模拟绑定微信提醒。" : "已模拟恢复平台连接。"
    )
    if (success) {
      setStatus("success")
      setScenario("ready")
    }
  }
  return (
    <Modal
      title={wx ? "绑定微信提醒" : "连接充电平台"}
      description={
        wx
          ? "在微信中扫码，授权 WxPusher 接收通知。"
          : "使用你有权访问充电桩的微信账户完成连接。"
      }
    >
      <div className="cp-scan-content">
        {status === "success" ? (
          <>
            <span className="cp-scan-success">
              <CheckCheck size={42} />
            </span>
            <h3>{wx ? "微信提醒已绑定" : "平台连接已恢复"}</h3>
            <p>这是一次模拟绑定，没有进行真实微信授权。</p>
            <PButton
              tone="brand"
              onClick={() =>
                openModal(modal?.returnTo === "add" ? { kind: "add" } : null)
              }
            >
              {modal?.returnTo === "add" ? "继续添加充电桩" : "完成"}
              <ArrowRight size={15} />
            </PButton>
          </>
        ) : (
          <>
            <div
              className={`cp-qr-placeholder ${status !== "waiting" ? "cp-qr-expired" : ""}`}
            >
              <QrCode size={132} strokeWidth={1.1} />
              <span>演示二维码</span>
              {status !== "waiting" && (
                <div className="cp-qr-overlay">
                  <CircleAlert size={27} />
                  <strong>
                    {status === "expired" ? "二维码已过期" : "连接未成功"}
                  </strong>
                  <PButton onClick={() => setStatus("waiting")}>
                    <RefreshCw size={14} />
                    重新生成
                  </PButton>
                </div>
              )}
            </div>
            <h3>
              {status === "waiting"
                ? "等待扫码确认"
                : status === "expired"
                  ? "重新生成后再试一次"
                  : "检查连接后可重试"}
            </h3>
            <p>原型不生成真实授权码，请使用下方按钮体验完成后的流程。</p>
            <PButton
              tone="brand"
              onClick={complete}
              loading={!!busy}
              disabled={status !== "waiting" || offline}
            >
              模拟扫码成功
              <Check size={15} />
            </PButton>
            <details className="cp-demo-states">
              <summary>预览其他结果</summary>
              <div>
                <PButton tone="quiet" onClick={() => setStatus("expired")}>
                  模拟过期
                </PButton>
                <PButton tone="quiet" onClick={() => setStatus("failed")}>
                  模拟失败
                </PButton>
              </div>
            </details>
          </>
        )}
      </div>
    </Modal>
  )
}

function CookieDialog() {
  const { commit, openModal } = usePrototype()
  const [cookie, setCookie] = useState("")
  const [error, setError] = useState("")
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!cookie.includes("=")) {
      setError("请使用下方演示凭据，体验更新流程。")
      return
    }
    if (
      await commit((d) => {
        d.credential = "healthy"
        d.notices
          .filter((n) => n.type === "credential_expired")
          .forEach((n) => {
            n.resolved = true
          })
      }, "已模拟更新平台凭据。输入内容没有保存。")
    )
      openModal(null)
  }
  return (
    <Modal
      title="手动更新访问凭据"
      description="此原型不会保存或传输 Cookie，请勿填写真实凭据。"
    >
      <form className="cp-modal-form" onSubmit={submit}>
        <Field label="演示 Cookie">
          <textarea
            className="cp-input cp-textarea cp-mono"
            value={cookie}
            onChange={(e) => setCookie(e.target.value)}
            placeholder="使用演示凭据体验，不填写真实 Cookie"
          />
        </Field>
        <PButton
          onClick={() => setCookie("demo_session=not-a-real-credential")}
          tone="quiet"
        >
          填入演示凭据
        </PButton>
        <FormError message={error} />
        <ModalActions>
          <SaveButton>更新连接</SaveButton>
        </ModalActions>
      </form>
    </Modal>
  )
}

function PasswordDialog() {
  const { commit, openModal } = usePrototype()
  const [current, setCurrent] = useState("")
  const [next, setNext] = useState("")
  const [confirm, setConfirm] = useState("")
  const [error, setError] = useState("")
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!current || next.length < 8) {
      setError("请填写当前演示密码，新演示密码至少 8 位。")
      return
    }
    if (next !== confirm) {
      setError("两次填写的新密码不一致。")
      return
    }
    if (current === next) {
      setError("新密码需要与当前密码不同。")
      return
    }
    if (
      await commit((d) => {
        d.otherSessions = false
      }, "密码修改流程已演示，其他示例会话已退出；没有改动真实密码。")
    )
      openModal(null)
  }
  return (
    <Modal
      title="修改登录密码"
      description="这里只演示修改流程，不要填写真实密码。"
    >
      <form className="cp-modal-form" onSubmit={submit}>
        <Field label="当前演示密码">
          <PInput
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="off"
            required
          />
        </Field>
        <Field label="新演示密码" hint="至少 8 位，建议使用字母、数字和符号。">
          <PInput
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            required
          />
        </Field>
        <Field label="确认新密码">
          <PInput
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            required
          />
        </Field>
        <FormError message={error} />
        <ModalActions>
          <SaveButton>确认修改</SaveButton>
        </ModalActions>
      </form>
    </Modal>
  )
}

function CreateUserDialog() {
  const { data, commit, go, addAudit } = usePrototype()
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [role, setRole] = useState<"user" | "admin">("user")
  const [limit, setLimit] = useState(String(data.settings.defaultDeviceLimit))
  const [error, setError] = useState("")
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!/^[a-zA-Z0-9_-]{3,32}$/.test(username)) {
      setError("用户名需要 3–32 位字母、数字、下划线或连字符。")
      return
    }
    if (data.users.some((u) => u.username === username)) {
      setError("这个用户名已经存在。")
      return
    }
    if (password.length < 8) {
      setError("请设置至少 8 位演示密码。")
      return
    }
    if (
      !Number.isInteger(Number(limit)) ||
      Number(limit) < 0 ||
      Number(limit) > 100
    ) {
      setError("设备上限需要填写 0–100 之间的整数。")
      return
    }
    const id = `user-${Date.now()}`
    if (
      await commit((d) => {
        d.users.push({
          id,
          username,
          role,
          enabled: true,
          deviceCount: 0,
          deviceLimit: role === "admin" ? 0 : Number(limit),
          refreshEnabled:
            role === "user" && data.settings.defaultRefreshEnabled,
          credential: "unbound",
          lastActive: "尚未登录",
          requests: 0,
          sessions: 0,
        })
        addAudit(d, "创建用户", username)
      }, "示例用户已创建，密码未保存。")
    )
      go({ view: "users", id })
  }
  return (
    <Modal
      title="创建用户"
      description="创建独立账户，设备与连接凭据按账户隔离。"
    >
      <form className="cp-modal-form" onSubmit={submit}>
        <Field label="用户名">
          <PInput
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoComplete="off"
          />
        </Field>
        <Field label="初始演示密码" hint="至少 8 位，不填写真实密码。">
          <PInput
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="new-password"
          />
        </Field>
        <div className="cp-form-grid">
          <Field label="角色">
            <select
              className="cp-input"
              value={role}
              onChange={(e) => setRole(e.target.value as "admin" | "user")}
            >
              <option value="user">普通用户</option>
              <option value="admin">管理员</option>
            </select>
          </Field>
          <Field label="设备上限 / 台">
            <PInput
              type="number"
              min={0}
              max={100}
              value={role === "admin" ? "0" : limit}
              disabled={role === "admin"}
              onChange={(e) => setLimit(e.target.value)}
            />
          </Field>
        </div>
        <FormError message={error} />
        <ModalActions>
          <SaveButton>创建用户</SaveButton>
        </ModalActions>
      </form>
    </Modal>
  )
}

function ResetPasswordDialog() {
  const { data, modal, commit, addAudit, openModal, notify, busy } =
    usePrototype()
  const user = data.users.find((u) => u.id === modal?.id)
  const [generated, setGenerated] = useState(false)
  const temporary = "DEMO-only-7x4P9k"
  async function reset() {
    if (
      await commit((d) => {
        const u = d.users.find((u) => u.id === user?.id)
        if (u) u.sessions = 0
        addAudit(d, "重置用户密码", user?.username || "")
      }, "已模拟重置密码并退出全部会话。")
    )
      setGenerated(true)
  }
  return (
    <Modal
      title={generated ? "临时密码已生成" : `重置 ${user?.username} 的密码？`}
      description={
        generated
          ? "这是演示临时密码，没有更改真实账户。"
          : "该用户的所有登录会话将退出，下次使用临时密码登录后需要改密。"
      }
    >
      <div className="cp-modal-form">
        {generated ? (
          <>
            <div className="cp-temporary-password cp-mono">{temporary}</div>
            <PButton
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(temporary)
                  notify("演示临时密码已复制。")
                } catch {
                  notify("请手动选择并复制演示密码。", "info")
                }
              }}
            >
              <Copy size={14} />
              复制临时密码
            </PButton>
            <div className="cp-modal-actions">
              <PButton tone="brand" onClick={() => openModal(null)}>
                完成
              </PButton>
            </div>
          </>
        ) : (
          <ModalActions>
            <PButton tone="danger" loading={!!busy} onClick={reset}>
              确认重置
            </PButton>
          </ModalActions>
        )}
      </div>
    </Modal>
  )
}

function InviteDialog() {
  const { data, commit, openModal, addAudit } = usePrototype()
  const [code, setCode] = useState("")
  const [expires, setExpires] = useState("2026-09-30")
  const [error, setError] = useState("")
  async function submit(e: FormEvent) {
    e.preventDefault()
    const value =
      code.trim().toUpperCase() ||
      `CHARGE-${String(data.invites.length + 1).padStart(4, "0")}`
    if (!/^[A-Z0-9_-]{4,32}$/.test(value)) {
      setError("邀请码使用 4–32 位字母、数字、下划线或连字符。")
      return
    }
    if (data.invites.some((i) => i.code === value)) {
      setError("这个邀请码已经存在。")
      return
    }
    if (expires && expires < "2026-09-10") {
      setError("到期时间不能早于示例日期 9 月 10 日。")
      return
    }
    if (
      await commit((d) => {
        d.invites.unshift({
          id: `invite-${Date.now()}`,
          code: value,
          expires,
          enabled: true,
          uses: 0,
        })
        addAudit(d, "创建邀请码", value)
      }, "邀请码已创建。")
    )
      openModal(null)
  }
  return (
    <Modal
      title="创建邀请码"
      description="生成后可复制分享，只在此原型内使用。"
    >
      <form className="cp-modal-form" onSubmit={submit}>
        <Field label="邀请码" hint="留空将自动生成。">
          <PInput
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder="例如 GARDEN-AUTUMN"
          />
        </Field>
        <Field label="有效期至" hint="留空表示长期有效。">
          <PInput
            type="date"
            value={expires}
            onChange={(e) => setExpires(e.target.value)}
          />
        </Field>
        <FormError message={error} />
        <ModalActions>
          <SaveButton>创建邀请码</SaveButton>
        </ModalActions>
      </form>
    </Modal>
  )
}

function GuideDialog() {
  const { openModal } = usePrototype()
  return (
    <Modal
      title="开始使用 Charge"
      description="先连接，再添加。之后只需看一眼常去的地方。"
    >
      <div className="cp-guide-steps">
        {[
          ["01", "连接充电平台", "在账户设置中扫码，或使用手动访问凭据。"],
          [
            "02",
            "添加常用充电桩",
            "输入设备上的桩号，补充容易记住的名称和位置。",
          ],
          [
            "03",
            "查看状态，或等一次提醒",
            "没有空闲时选择等待时长，空闲后通知一次并自动结束。",
          ],
        ].map(([number, title, text]) => (
          <div key={number}>
            <span className="cp-mono">{number}</span>
            <div>
              <h3>{title}</h3>
              <p>{text}</p>
            </div>
          </div>
        ))}
        <p className="cp-form-hint">
          <ShieldCheck size={15} />
          仅使用有权访问的设备，不进行高频采集；这里不提供远程启停或预订。
        </p>
        <PButton tone="brand" onClick={() => openModal(null)}>
          知道了
          <Check size={15} />
        </PButton>
      </div>
    </Modal>
  )
}

function ConfirmationDialog() {
  const { modal, data, commit, openModal, go, busy, addAudit } = usePrototype()
  const [text, setText] = useState("")
  const kind = modal?.kind
  const pile = data.piles.find((p) => p.id === modal?.id)
  const user = data.users.find((u) => u.id === modal?.id)
  const options =
    kind === "remove"
      ? {
          title: `移除 ${pile?.name || "这个充电桩"}？`,
          description: "将从常用列表移除，并结束关联提醒。不会操作现场设备。",
          action: "确认移除",
        }
      : kind === "delete-user"
        ? {
            title: `删除账户 ${user?.username}？`,
            description:
              "账户、关联设备、通知和会话将被移除。请输入用户名确认。",
            action: "确认删除账户",
          }
        : kind === "sessions"
          ? {
              title: "退出其他登录设备？",
              description: "当前设备继续保持登录，其他设备需要重新登录。",
              action: "退出其他设备",
            }
          : kind === "logout"
            ? {
                title: "退出当前账户？",
                description: "示例数据会保留，下次可重新进入演示账户。",
                action: "退出登录",
              }
            : kind === "clear-notices"
              ? {
                  title: "清理已解决的通知？",
                  description:
                    "只清理已解决的连接问题，待处理事项和空闲消息会保留。",
                  action: "清理已解决",
                }
              : modal?.id === "wx"
                ? {
                    title: "解除微信提醒绑定？",
                    description:
                      "删除本地绑定并取消尚未发送的消息。站内通知保留；不会替你取消关注 WxPusher。",
                    action: "解除微信绑定",
                  }
                : {
                    title: "解除平台绑定？",
                    description:
                      "停止自动恢复登录。已添加的设备和历史记录不会删除。",
                    action: "解除绑定",
                  }
  async function confirm() {
    const ok = await commit(
      (d) => {
        if (kind === "remove") {
          d.piles = d.piles.filter((p) => p.id !== modal?.id)
          d.watches
            .filter((w) => w.deviceId === modal?.id && w.status === "active")
            .forEach((w) => {
              w.status = "cancelled"
            })
        } else if (kind === "delete-user") {
          d.users = d.users.filter((u) => u.id !== modal?.id)
          addAudit(d, "删除用户", user?.username || "")
          d.incidents
            .filter((i) => i.userId === modal?.id)
            .forEach((i) => {
              i.status = "resolved"
              i.note = "关联账户已删除"
            })
        } else if (kind === "sessions") d.otherSessions = false
        else if (kind === "logout") d.demoRole = "guest"
        else if (kind === "clear-notices")
          d.notices = d.notices.filter(
            (n) =>
              !(
                n.resolved &&
                ["pile_offline", "credential_expired"].includes(n.type)
              )
          )
        else if (modal?.id === "wx") {
          d.wxBound = false
          d.wxEnabled = false
          d.wxTest = "none"
        } else d.credential = "unbound"
      },
      kind === "remove"
        ? "已从常用桩移除，关联提醒已结束。"
        : kind === "delete-user"
          ? "示例账户及其关联记录已移除。"
          : kind === "logout"
            ? "已退出演示账户。"
            : "更改已完成。"
    )
    if (ok) {
      if (kind === "remove") go({ view: "piles" })
      else if (kind === "delete-user") go({ view: "users" })
      else if (kind === "logout") go({ view: "login" })
      else openModal(null)
    }
  }
  return (
    <Modal title={options.title} description={options.description}>
      <div className="cp-modal-form">
        {kind === "delete-user" && (
          <Field label="输入要删除的用户名">
            <PInput
              value={text}
              onChange={(e) => setText(e.target.value)}
              autoComplete="off"
            />
          </Field>
        )}
        <ModalActions>
          <PButton
            tone={kind === "sessions" || kind === "logout" ? "brand" : "danger"}
            onClick={confirm}
            loading={!!busy}
            disabled={kind === "delete-user" && text !== user?.username}
          >
            {kind === "remove" || kind === "delete-user" ? (
              <Trash2 size={14} />
            ) : null}
            {options.action}
          </PButton>
        </ModalActions>
      </div>
    </Modal>
  )
}
