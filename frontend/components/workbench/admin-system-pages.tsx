"use client"

import {
  ClipboardListIcon,
  CopyIcon,
  DatabaseIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react"
import { useSearchParams } from "next/navigation"
import { useCallback, useEffect, useState, type ReactNode } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { adminApi } from "@/lib/admin-api"
import { useMinuteClock, useOnline } from "@/lib/browser-state"
import { notify } from "@/lib/feedback"
import { formatSnapshotTime, updatePageQuery } from "@/lib/workbench"
import { minutesToTime, timeToMinutes } from "@/lib/watch-format"
import type {
  AuditPage,
  InviteCode,
  InviteCodePage,
  OperationsStatus,
  RegistrationSettings,
} from "@/lib/types"
import {
  SectionHeading,
  StatusPill,
  WorkbenchButton,
  WorkbenchEmpty,
  WorkbenchError,
  WorkbenchLoading,
  WorkbenchSearch,
} from "./surfaces"

function useRemote<T>(fetcher: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null),
    [error, setError] = useState("")
  const [loading, setLoading] = useState(true),
    [revision, setRevision] = useState(0)
  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) setLoading(true)
    })
    fetcher()
      .then((value) => {
        if (active) {
          setData(value)
          setError("")
        }
      })
      .catch((reason) => {
        if (active)
          setError(
            reason instanceof Error ? reason.message : "暂时无法读取数据。"
          )
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [fetcher, revision])
  return {
    data,
    setData,
    error,
    loading,
    reload: () => {
      setLoading(true)
      setRevision((value) => value + 1)
    },
  }
}
function Facts({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="wb-ops-facts">
      {items.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value ?? "—"}</dd>
        </div>
      ))}
    </dl>
  )
}
function bytes(value?: number) {
  return value === undefined
    ? "尚未记录"
    : value < 1024 * 1024
      ? `${(value / 1024).toFixed(1)} KB`
      : `${(value / 1024 / 1024).toFixed(1)} MB`
}
function percent(value: number) {
  return Number.isFinite(value) ? `${value.toFixed(1)}%` : "—"
}

export function WorkbenchOperations() {
  const { data, error, loading, reload } = useRemote<OperationsStatus>(
      adminApi.operations
    ),
    online = useOnline()
  if (error && !data) return <WorkbenchError message={error} retry={reload} />
  if (!data) return <WorkbenchLoading label="正在检查系统运行" />
  const r = data.reminders,
    w = data.wxPusher
  const reminderState = {
    healthy: "运行正常",
    degraded: "需要关注",
    disabled: "已关闭",
    power_off: "计划断电",
    stopped: "已停止",
  }
  const wxState = {
    healthy: "运行正常",
    degraded: "需要关注",
    disabled: "未开启",
    stopped: "已停止",
  }
  return (
    <div className="wb-operations">
      <div className="wb-admin-healthline">
        <StatusPill tone={data.integrityResult === "ok" ? "idle" : "danger"}>
          数据库{data.integrityResult === "ok" ? "完整" : "需检查"}
        </StatusPill>
        <span>检查于 {formatSnapshotTime(data.checkedAt, true)}</span>
        <WorkbenchButton
          variant="ghost"
          busy={loading}
          disabled={!online}
          onClick={reload}
        >
          <RefreshCwIcon size={14} />
          重新检查
        </WorkbenchButton>
      </div>
      {error && (
        <p className="wb-form-error" role="alert">
          {error} · 当前显示上次检查结果
        </p>
      )}
      <section className="wb-operations-section">
        <SectionHeading title="后台提醒">
          <StatusPill
            tone={
              r.state === "healthy"
                ? "idle"
                : r.state === "degraded"
                  ? "warning"
                  : "neutral"
            }
          >
            {reminderState[r.state]}
          </StatusPill>
        </SectionHeading>
        <p className="wb-section-description">{r.message}</p>
        <Facts
          items={[
            ["活动临时提醒", `${r.activeTemporaryRules} 条`],
            ["正在关注的充电桩", `${r.trackedPiles} 台`],
            ["等待检查 / 检查中", `${r.duePiles} / ${r.inFlightPiles} 台`],
            ["下次检查", formatSnapshotTime(r.nextAttemptAt, true)],
            ["已通知结束 · 24h", `${r.completedNotified24Hours} 条`],
            ["已到期结束 · 24h", `${r.completedExpired24Hours} 条`],
            ["平均等待", `${r.averageTemporaryMinutes.toFixed(1)} 分钟`],
            ["后台调度器", r.schedulerRunning ? "运行中" : "已停止"],
          ]}
        />
        <details className="wb-operational-details">
          <summary>检查结果与请求保护</summary>
          <Facts
            items={[
              ["远端请求 · 24h", r.remoteAttempts24Hours],
              [
                "成功 / 失败",
                `${r.remoteSuccesses24Hours} / ${r.remoteFailures24Hours}`,
              ],
              ["远端成功率", percent(r.remoteSuccessRate24Hours)],
              ["缓存命中", r.cacheHits24Hours],
              ["同桩合并", r.coalesced24Hours],
              ["额度跳过", r.quotaSkips24Hours],
              ["调度错误", r.schedulerErrors24Hours],
              ["最多连续失败", r.maxConsecutiveFailures],
              ["计划断电窗口", r.scheduledPowerOffActive ? "生效中" : "未生效"],
              ["系统允许提醒", r.enabled ? "是" : "否"],
            ]}
          />
        </details>
      </section>
      <section className="wb-operations-section">
        <SectionHeading
          title="微信消息投递"
          description="渠道受理、供应商处理与手机送达是不同阶段。"
        >
          <StatusPill
            tone={
              w.state === "healthy"
                ? "idle"
                : w.state === "degraded"
                  ? "warning"
                  : "neutral"
            }
          >
            {wxState[w.state]}
          </StatusPill>
        </SectionHeading>
        <p className="wb-section-description">{w.message}</p>
        <Facts
          items={[
            ["有效绑定", w.activeBindings],
            [
              "待发送 / 发送中",
              `${w.pendingDeliveries} / ${w.sendingDeliveries}`,
            ],
            ["已受理待确认", w.acceptedPendingDeliveries],
            ["等待重试", w.retryingDeliveries],
            ["结果不确定", w.uncertainDeliveries],
            ["发送失败", w.failedDeliveries],
            ["最早待发送", formatSnapshotTime(w.oldestPendingAt, true)],
            ["投递服务", w.dispatcherRunning ? "运行中" : "已停止"],
          ]}
        />
        <details className="wb-operational-details">
          <summary>24 小时结果与失败诊断</summary>
          <Facts
            items={[
              ["发送尝试", w.attempts24Hours],
              ["已受理", w.accepted24Hours],
              ["供应商确认处理", w.providerSucceeded24Hours],
              [
                "受理率 / 处理成功率",
                `${percent(w.acceptanceRate24Hours)} / ${percent(w.providerSuccessRate24Hours)}`,
              ],
              ["系统失败", w.systemFailures24Hours],
              [
                "绑定失败 / 影响用户",
                `${w.bindingFailures24Hours} / ${w.affectedBindingUsers}`,
              ],
              ["连续系统失败", w.consecutiveSystemFailures],
              ["最近受理", formatSnapshotTime(w.lastAcceptedAt, true)],
              [
                "最近处理成功",
                formatSnapshotTime(w.lastProviderSuccessAt, true),
              ],
              ["最近失败", formatSnapshotTime(w.lastFailureAt, true)],
              ["错误类别", w.lastErrorCategory || "无"],
              ["服务端配置", w.configured ? "已配置" : "未配置"],
            ]}
          />
        </details>
      </section>
      <section className="wb-operations-section">
        <SectionHeading title="数据与备份" />
        <div className="wb-operation-row">
          <DatabaseIcon size={22} />
          <div>
            <strong>SQLite 数据库 · {bytes(data.databaseSizeBytes)}</strong>
            <p>最近校验 {formatSnapshotTime(data.checkedAt, true)}</p>
          </div>
          <StatusPill tone={data.integrityResult === "ok" ? "idle" : "danger"}>
            {data.integrityResult === "ok" ? "校验通过" : data.integrityResult}
          </StatusPill>
        </div>
        <Facts
          items={[
            ["统计记录", `${data.metricRows.toLocaleString()} 条`],
            ["提醒状态事件", `${data.portHistoryRows.toLocaleString()} 条`],
            [
              "最早 / 最近事件",
              `${formatSnapshotTime(data.portHistoryOldestAt, true)} / ${formatSnapshotTime(data.portHistoryNewestAt, true)}`,
            ],
            [
              "通知 / 已解决",
              `${data.notificationRows} / ${data.resolvedNotificationRows}`,
            ],
            ["统计保留", `${data.metricRetentionDays} 天`],
            ["提醒状态事件保留", `${data.portHistoryRetentionDays} 天`],
            ["通知保留", `${data.notificationRetentionDays} 天`],
            [
              "最近备份",
              data.lastBackupAt
                ? formatSnapshotTime(data.lastBackupAt, true)
                : "尚未发现",
            ],
            ["备份大小", bytes(data.lastBackupSizeBytes)],
            ["备份状态", data.backupMessage],
          ]}
        />
        <p className="wb-data-note">
          这里只读取运行情况。备份任务在服务器上执行，不从此页面触发数据库删除或备份。
        </p>
      </section>
    </div>
  )
}

const numericSettings: [keyof RegistrationSettings, string, number, number][] =
  [
    ["defaultDeviceLimit", "新用户设备上限 / 台", 1, 100],
    ["watchRefreshIntervalMinutes", "检查间隔 / 分钟", 5, 60],
    ["watchPileLimitPerUser", "每人同时提醒的充电桩 / 台", 1, 20],
    ["watchDailyRefreshQuota", "每人每日检查额度 / 次", 1, 10000],
    ["statsRetentionDays", "运行统计保留 / 天", 1, 365],
    ["portHistoryRetentionDays", "提醒状态事件保留 / 天", 1, 365],
    ["notificationRetentionDays", "已解决通知保留 / 天", 7, 365],
    ["powerRestoreJitterMinutes", "恢复供电后错峰 / 分钟", 0, 60],
  ]
export function WorkbenchPolicies() {
  const { data, error, reload, setData } = useRemote<RegistrationSettings>(
    adminApi.settings
  )
  if (error && !data) return <WorkbenchError message={error} retry={reload} />
  if (!data) return <WorkbenchLoading label="正在读取系统策略" />
  return (
    <PolicyForm key={JSON.stringify(data)} initial={data} onSaved={setData} />
  )
}
function PolicyForm({
  initial,
  onSaved,
}: {
  initial: RegistrationSettings
  onSaved: (value: RegistrationSettings) => void
}) {
  const online = useOnline()
  const [draft, setDraft] = useState(initial),
    [saving, setSaving] = useState(false),
    [error, setError] = useState("")
  const [confirmRetention, setConfirmRetention] = useState(false)
  const dirty = JSON.stringify(draft) !== JSON.stringify(initial)
  useEffect(() => {
    if (!dirty) return
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ""
    }
    window.addEventListener("beforeunload", handler)
    return () => window.removeEventListener("beforeunload", handler)
  }, [dirty])
  function change(
    key: keyof RegistrationSettings,
    value: RegistrationSettings[keyof RegistrationSettings]
  ) {
    setDraft((current) => ({ ...current, [key]: value }))
  }
  async function save() {
    setSaving(true)
    setError("")
    try {
      const value = await adminApi.saveSettings({
        ...draft,
        recurringRemindersEnabled: false,
      })
      onSaved(value)
      setConfirmRetention(false)
      notify.success("系统策略已保存")
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "策略未保存，请重试。"
      )
      setConfirmRetention(false)
    } finally {
      setSaving(false)
    }
  }
  function submit(event: React.FormEvent) {
    event.preventDefault()
    for (const [key, label, min, max] of numericSettings) {
      const value = Number(draft[key])
      if (!Number.isInteger(value) || value < min || value > max) {
        setError(`${label}需要在 ${min}–${max} 之间。`)
        return
      }
    }
    if (
      draft.scheduledPowerOffEnabled &&
      draft.scheduledPowerOffStartMinute === draft.scheduledPowerOffEndMinute
    ) {
      setError("断电开始和恢复时间不能相同。")
      return
    }
    if (
      [
        "statsRetentionDays",
        "portHistoryRetentionDays",
        "notificationRetentionDays",
      ].some(
        (key) =>
          Number(draft[key as keyof RegistrationSettings]) <
          Number(initial[key as keyof RegistrationSettings])
      )
    ) {
      setConfirmRetention(true)
      return
    }
    void save()
  }
  const input = (key: keyof RegistrationSettings) => {
    const [, label, min, max] = numericSettings.find((item) => item[0] === key)!
    return (
      <Field key={key}>
        <FieldLabel htmlFor={`policy-${key}`}>{label}</FieldLabel>
        <Input
          id={`policy-${key}`}
          type="number"
          min={min}
          max={max}
          required
          value={Number(draft[key])}
          onChange={(event) =>
            change(
              key,
              event.target.value === "" ? 0 : Number(event.target.value)
            )
          }
        />
      </Field>
    )
  }
  const toggle = (
    key: keyof RegistrationSettings,
    title: string,
    description: string
  ) => (
    <div className="wb-setting-row">
      <div>
        <h3>{title}</h3>
        <p>{description}</p>
      </div>
      <Switch
        aria-label={title}
        checked={!!draft[key]}
        disabled={!online || saving}
        onCheckedChange={(value) => change(key, value)}
      />
    </div>
  )
  return (
    <form onSubmit={submit} className="wb-policies-form">
      <SectionHeading title="注册与默认权限" />
      {toggle(
        "openRegistration",
        "开放自助注册",
        "开启后，不填写邀请码也可以创建账户。"
      )}
      {toggle(
        "inviteRequired",
        "允许邀请码注册",
        "关闭自助注册时，持有有效邀请码的人仍可注册。"
      )}
      {toggle(
        "defaultRefreshEnabled",
        "新用户允许刷新",
        "新建账户的默认远端访问权限，不影响已有账户。"
      )}
      <div className="wb-form-grid">{input("defaultDeviceLimit")}</div>
      <SectionHeading title="按需提醒" />
      {toggle(
        "backgroundRemindersEnabled",
        "允许后台空闲提醒",
        "只为用户主动开启的临时提醒检查设备；固定周期规则已停用。"
      )}
      <div className="wb-form-grid">
        {input("watchRefreshIntervalMinutes")}
        {input("watchPileLimitPerUser")}
        {input("watchDailyRefreshQuota")}
      </div>
      <SectionHeading title="计划断电" />
      {toggle(
        "scheduledPowerOffEnabled",
        "在断电时段暂停请求",
        "临时提醒的截止时间不超过当日断电开始时间。"
      )}
      {draft.scheduledPowerOffEnabled && (
        <div className="wb-form-grid">
          {(
            [
              ["scheduledPowerOffStartMinute", "断电开始时间"],
              ["scheduledPowerOffEndMinute", "恢复供电时间"],
            ] as const
          ).map(([key, label]) => (
            <Field key={key}>
              <FieldLabel htmlFor={`policy-${key}`}>{label}</FieldLabel>
              <Input
                id={`policy-${key}`}
                type="time"
                required
                value={minutesToTime(draft[key])}
                onChange={(event) => {
                  const value = timeToMinutes(event.target.value)
                  if (value !== null) change(key, value)
                }}
              />
            </Field>
          ))}
          {input("powerRestoreJitterMinutes")}
          <Field>
            <FieldLabel htmlFor="policy-timezone">时区</FieldLabel>
            <Input
              id="policy-timezone"
              value={draft.scheduledPowerOffTimezone}
              required
              onChange={(event) =>
                change("scheduledPowerOffTimezone", event.target.value)
              }
              placeholder="Asia/Shanghai"
            />
          </Field>
        </div>
      )}
      <SectionHeading
        title="数据保留"
        description="缩短保留时间会清理过期记录，尚未解决的问题会继续保留。"
      />
      <div className="wb-form-grid">
        {input("statsRetentionDays")}
        {input("portHistoryRetentionDays")}
        {input("notificationRetentionDays")}
      </div>
      {error && (
        <p className="wb-form-error" role="alert">
          {error}
        </p>
      )}
      <div className="wb-save-bar">
        <span>{dirty ? "有尚未保存的更改" : "所有更改已保存"}</span>
        <div className="wb-row-actions">
          <WorkbenchButton
            variant="ghost"
            disabled={!dirty || saving}
            type="button"
            onClick={() => {
              setDraft(initial)
              setError("")
            }}
          >
            撤销更改
          </WorkbenchButton>
          <WorkbenchButton
            type="submit"
            busy={saving}
            disabled={!online || !dirty}
          >
            保存更改
          </WorkbenchButton>
        </div>
      </div>
      <Dialog
        open={confirmRetention}
        onOpenChange={(open) => !saving && setConfirmRetention(open)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>缩短事件保留时间？</DialogTitle>
            <DialogDescription>
              服务器会清理超出新保留范围的记录，不能在页面中恢复。请确认备份和保留范围后继续。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <WorkbenchButton
              variant="outline"
              type="button"
              disabled={saving}
              onClick={() => setConfirmRetention(false)}
            >
              返回检查
            </WorkbenchButton>
            <WorkbenchButton
              variant="destructive"
              type="button"
              busy={saving}
              disabled={!online}
              onClick={() => void save()}
            >
              确认保存并清理
            </WorkbenchButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </form>
  )
}

export function WorkbenchInvites() {
  const now = useMinuteClock()
  const params = useSearchParams(),
    online = useOnline()
  const page = Math.max(1, Number(params.get("page")) || 1),
    search = params.get("q") ?? ""
  const fetcher = useCallback(
    () => adminApi.invites({ page, pageSize: 20 }),
    [page]
  )
  const { data, error, loading, reload } = useRemote<InviteCodePage>(fetcher)
  const [createOpen, setCreateOpen] = useState(false),
    [remove, setRemove] = useState<InviteCode | null>(null)
  const [code, setCode] = useState(""),
    [expires, setExpires] = useState(""),
    [pending, setPending] = useState(false),
    [formError, setFormError] = useState("")
  const items =
    data?.items.filter((item) =>
      item.code.toLowerCase().includes(search.toLowerCase())
    ) ?? []
  async function create(event: React.FormEvent) {
    event.preventDefault()
    setFormError("")
    let expiresAt: string | undefined
    if (expires) {
      const date = new Date(expires)
      if (!Number.isFinite(date.getTime()) || date.getTime() <= Date.now()) {
        setFormError("到期时间需要晚于现在。")
        return
      }
      expiresAt = date.toISOString()
    }
    setPending(true)
    try {
      await adminApi.createInvite({ code: code.trim() || undefined, expiresAt })
      setCreateOpen(false)
      setCode("")
      setExpires("")
      updatePageQuery({ page: null, q: null })
      reload()
      notify.success("邀请码已创建")
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : "邀请码未创建。")
    } finally {
      setPending(false)
    }
  }
  async function deleteInvite() {
    if (!remove) return
    setPending(true)
    try {
      await adminApi.removeInvite(remove.id)
      setRemove(null)
      reload()
      notify.success("邀请码已删除，不能再用于注册")
    } catch (reason) {
      notify.error(reason, { title: "邀请码未删除" })
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <div className="wb-page-rule">
        <WorkbenchSearch
          label="搜索邀请码"
          placeholder="搜索当前页邀请码"
          value={search}
          onChange={(value) => updatePageQuery({ q: value }, true)}
        />
        <WorkbenchButton
          disabled={!online}
          onClick={() => {
            setFormError("")
            setCreateOpen(true)
          }}
        >
          <PlusIcon size={15} />
          创建邀请码
        </WorkbenchButton>
      </div>
      {error ? (
        <WorkbenchError message={error} retry={reload} />
      ) : !data || loading ? (
        <WorkbenchLoading label="正在读取邀请码" />
      ) : !items.length ? (
        <WorkbenchEmpty
          icon={ClipboardListIcon}
          title="没有可显示的邀请码"
          description="可以创建新的邀请码，或调整搜索条件。"
        />
      ) : (
        <div className="wb-table-wrap">
          <table className="wb-data-table">
            <thead>
              <tr>
                <th>邀请码</th>
                <th>状态</th>
                <th>使用次数</th>
                <th>到期时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => {
                const expired =
                  !!item.expiresAt && Date.parse(item.expiresAt) <= now
                return (
                  <tr key={item.id}>
                    <td className="wb-mono">{item.code}</td>
                    <td>
                      <StatusPill
                        tone={item.enabled && !expired ? "idle" : "neutral"}
                      >
                        {expired
                          ? "已过期"
                          : item.enabled
                            ? "可使用"
                            : "已停用"}
                      </StatusPill>
                    </td>
                    <td>{item.usedCount} 次</td>
                    <td>
                      {item.expiresAt
                        ? formatSnapshotTime(item.expiresAt, true)
                        : "长期有效"}
                    </td>
                    <td>
                      <div className="wb-row-actions">
                        <WorkbenchButton
                          variant="ghost"
                          aria-label={`复制邀请码 ${item.code}`}
                          onClick={async () => {
                            try {
                              await navigator.clipboard.writeText(item.code)
                              notify.success("邀请码已复制")
                            } catch {
                              notify.error("请手动选择并复制邀请码。")
                            }
                          }}
                        >
                          <CopyIcon size={14} />
                          复制
                        </WorkbenchButton>
                        <WorkbenchButton
                          variant="ghost"
                          disabled={!online}
                          aria-label={`删除邀请码 ${item.code}`}
                          onClick={() => setRemove(item)}
                        >
                          <Trash2Icon size={14} />
                        </WorkbenchButton>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      {data && (
        <div className="wb-pagination">
          <span>
            共 {data.total} 个 · 第 {data.page} / {Math.max(data.totalPages, 1)}{" "}
            页
          </span>
          <div>
            <WorkbenchButton
              variant="ghost"
              disabled={data.page <= 1 || loading}
              onClick={() => updatePageQuery({ page: data.page - 1 })}
            >
              上一页
            </WorkbenchButton>
            <WorkbenchButton
              variant="ghost"
              disabled={data.page >= data.totalPages || loading}
              onClick={() => updatePageQuery({ page: data.page + 1 })}
            >
              下一页
            </WorkbenchButton>
          </div>
        </div>
      )}
      <Dialog
        open={createOpen}
        onOpenChange={(open) => !pending && setCreateOpen(open)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>创建邀请码</DialogTitle>
            <DialogDescription>
              留空将自动生成。可以设置到期时间，限制可使用的范围。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={create}>
            <Field>
              <FieldLabel htmlFor="invite-code">邀请码</FieldLabel>
              <Input
                id="invite-code"
                value={code}
                maxLength={64}
                onChange={(event) => setCode(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="invite-expires">
                到期时间（本地时间）
              </FieldLabel>
              <Input
                id="invite-expires"
                type="datetime-local"
                value={expires}
                onChange={(event) => setExpires(event.target.value)}
              />
            </Field>
            {formError && (
              <p className="wb-form-error" role="alert">
                {formError}
              </p>
            )}
            <DialogFooter>
              <WorkbenchButton
                type="button"
                variant="outline"
                disabled={pending}
                onClick={() => setCreateOpen(false)}
              >
                取消
              </WorkbenchButton>
              <WorkbenchButton type="submit" disabled={!online} busy={pending}>
                创建邀请码
              </WorkbenchButton>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!remove}
        onOpenChange={(open) => !open && !pending && setRemove(null)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>删除这个邀请码？</DialogTitle>
            <DialogDescription>
              {remove?.code} 将不能再用于注册，已经注册的账户不受影响。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <WorkbenchButton
              variant="outline"
              disabled={pending}
              onClick={() => setRemove(null)}
            >
              取消
            </WorkbenchButton>
            <WorkbenchButton
              variant="destructive"
              busy={pending}
              disabled={!online}
              onClick={() => void deleteInvite()}
            >
              确认删除
            </WorkbenchButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

const auditLabels: Record<string, string> = {
  "user.create": "创建用户",
  "user.update": "更新用户",
  "user.delete": "删除用户",
  "user.password_reset": "重置密码",
  "user.refresh": "刷新用户设备",
  "settings.update": "更新系统策略",
  "incident.update": "更新异常",
  "invite.create": "创建邀请码",
  "invite.delete": "删除邀请码",
}
export function WorkbenchAudit() {
  const params = useSearchParams(),
    page = Math.max(1, Number(params.get("page")) || 1),
    query = params.get("q") ?? "",
    result = params.get("result") ?? "all"
  const fetcher = useCallback(() => adminApi.audit(page, 20), [page])
  const { data, error, loading, reload } = useRemote<AuditPage>(fetcher)
  const items =
    data?.items.filter(
      (item) =>
        (result === "all" || item.result === result) &&
        `${item.actor} ${auditLabels[item.action] ?? item.action} ${item.targetLabel} ${item.message ?? ""}`.includes(
          query.trim()
        )
    ) ?? []
  return (
    <>
      <div className="wb-page-rule">
        <WorkbenchSearch
          label="搜索操作记录"
          placeholder="搜索当前页操作或对象"
          value={query}
          onChange={(value) => updatePageQuery({ q: value }, true)}
        />
        <select
          aria-label="操作结果"
          className="wb-input wb-select-compact"
          value={result}
          onChange={(event) =>
            updatePageQuery({
              result: event.target.value === "all" ? null : event.target.value,
            })
          }
        >
          <option value="all">全部结果</option>
          <option value="success">成功</option>
          <option value="failure">失败</option>
        </select>
      </div>
      {error ? (
        <WorkbenchError message={error} retry={reload} />
      ) : loading && !data ? (
        <WorkbenchLoading label="正在读取操作记录" />
      ) : !items.length ? (
        <WorkbenchEmpty
          icon={ClipboardListIcon}
          title="没有匹配的记录"
          description="调整筛选，或查看其他页的管理操作。"
        />
      ) : (
        <div className="wb-table-wrap">
          <table className="wb-data-table">
            <thead>
              <tr>
                <th>操作</th>
                <th>对象</th>
                <th>操作人</th>
                <th>结果</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td>
                    <strong>{auditLabels[item.action] ?? item.action}</strong>
                    {item.message && (
                      <p className="wb-small wb-muted">{item.message}</p>
                    )}
                  </td>
                  <td>{item.targetLabel || item.targetId || "系统"}</td>
                  <td>{item.actor}</td>
                  <td>
                    <StatusPill
                      tone={item.result === "success" ? "idle" : "danger"}
                    >
                      {item.result === "success" ? "成功" : "失败"}
                    </StatusPill>
                  </td>
                  <td>{formatSnapshotTime(item.createdAt, true)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {data && (
        <div className="wb-pagination">
          <span>
            共 {data.total} 条 · 第 {data.page} / {Math.max(1, data.totalPages)}{" "}
            页
          </span>
          <div>
            <WorkbenchButton
              variant="ghost"
              disabled={data.page <= 1 || loading}
              onClick={() => updatePageQuery({ page: data.page - 1 })}
            >
              上一页
            </WorkbenchButton>
            <WorkbenchButton
              variant="ghost"
              disabled={data.page >= data.totalPages || loading}
              onClick={() => updatePageQuery({ page: data.page + 1 })}
            >
              下一页
            </WorkbenchButton>
          </div>
        </div>
      )}
    </>
  )
}
