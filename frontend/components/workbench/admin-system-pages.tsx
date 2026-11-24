"use client"

import {
  ClipboardListIcon,
  CopyIcon,
  DatabaseIcon,
  PencilIcon,
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
  PublicPoolCookie,
  RegistrationSettings,
  SharedPile,
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
    ["freeDailyRefreshQuota", "免费用户每日刷新额度 / 次", 0, 50],
    ["pileContributionBonusQuota", "贡献新桩奖励永久次数 / 次", 0, 20],
    ["sharedPileCacheTTLSeconds", "全站桩状态共享缓存 / 秒", 15, 600],
  ]
type PolicySection = "pool" | "registration" | "reminders" | "retention"

export function WorkbenchPolicies() {
  const { data, error, reload, setData } = useRemote<RegistrationSettings>(
    adminApi.settings
  )
  const [section, setSection] = useState<PolicySection>("pool")
  if (error && !data) return <WorkbenchError message={error} retry={reload} />
  if (!data) return <WorkbenchLoading label="正在读取系统策略" />
  return (
    <PolicyForm
      key={JSON.stringify(data)}
      initial={data}
      section={section}
      onSectionChange={setSection}
      onSaved={setData}
    />
  )
}

export function WorkbenchPublicPool() {
  const online = useOnline()
  return <PublicCookiePoolManager online={online} />
}

export function WorkbenchSharedPiles() {
  const online = useOnline()
  const [query, setQuery] = useState("")
  const [debouncedQuery, setDebouncedQuery] = useState("")
  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(query), 200)
    return () => window.clearTimeout(timer)
  }, [query])
  const fetcher = useCallback(
    () => adminApi.sharedPiles(debouncedQuery),
    [debouncedQuery]
  )
  const { data, error, loading, reload } = useRemote<SharedPile[]>(fetcher)
  const [editing, setEditing] = useState<SharedPile | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<SharedPile | null>(null)
  const [editName, setEditName] = useState("")
  const [editAddress, setEditAddress] = useState("")
  const [busyId, setBusyId] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  async function saveEdit(event: React.FormEvent) {
    event.preventDefault()
    if (!editing) return
    setSaving(true)
    try {
      await adminApi.updateSharedPile(editing.deviceId, {
        name: editName.trim(),
        address: editAddress.trim(),
      })
      setEditing(null)
      reload()
      notify.success("公共桩信息已更新")
    } catch (reason) {
      notify.error(reason instanceof Error ? reason.message : "更新失败")
    } finally {
      setSaving(false)
    }
  }

  async function refreshItem(item: SharedPile) {
    setBusyId(item.deviceId)
    try {
      await adminApi.refreshSharedPile(item.deviceId)
      reload()
      notify.success(`已刷新「${item.name || item.number}」`)
    } catch (reason) {
      notify.error(reason instanceof Error ? reason.message : "刷新失败")
    } finally {
      setBusyId(null)
    }
  }

  async function deleteItem(item: SharedPile) {
    setBusyId(item.deviceId)
    try {
      await adminApi.deleteSharedPile(item.deviceId)
      setConfirmDelete(null)
      reload()
      notify.success(`已从公共桩库移除「${item.name || item.number}」`)
    } catch (reason) {
      notify.error(reason instanceof Error ? reason.message : "删除失败")
    } finally {
      setBusyId(null)
    }
  }

  const items = data ?? []

  return (
    <section className="wb-invites">
      <div className="wb-page-rule">
        <WorkbenchSearch
          label="搜索公共桩库"
          placeholder="按桩名、短桩号、设备 ID 或位置搜索"
          value={query}
          onChange={setQuery}
        />
        <span className="wb-small wb-muted">共收录 {items.length} 台</span>
      </div>
      {error ? (
        <WorkbenchError message={error} retry={reload} />
      ) : loading && !data ? (
        <WorkbenchLoading label="正在读取公共桩库" />
      ) : items.length === 0 ? (
        <WorkbenchEmpty
          icon={DatabaseIcon}
          title="公共桩库暂无记录"
          description="用户添加并校验通过的新充电桩会自动收录到这里。"
        />
      ) : (
        <div className="wb-table-wrap">
          <table className="wb-data-table">
            <thead>
              <tr>
                <th>充电桩</th>
                <th>桩号 / 设备 ID</th>
                <th>口数</th>
                <th>最近全站缓存</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.deviceId}>
                  <td>
                    <strong>{item.name || "未命名充电桩"}</strong>
                    {item.address && (
                      <p className="wb-small wb-muted">{item.address}</p>
                    )}
                  </td>
                  <td className="wb-mono">
                    <div>{item.number || "—"}</div>
                    <div className="wb-small wb-muted">{item.deviceId}</div>
                  </td>
                  <td>{item.openNum || 0} 口</td>
                  <td>
                    {item.lastFetchedAt
                      ? formatSnapshotTime(item.lastFetchedAt, true)
                      : "尚未缓存"}
                  </td>
                  <td>
                    <div className="wb-row-actions">
                      <WorkbenchButton
                        variant="ghost"
                        disabled={!online || busyId === item.deviceId}
                        onClick={() => {
                          setEditing(item)
                          setEditName(item.name || "")
                          setEditAddress(item.address || "")
                        }}
                      >
                        <PencilIcon size={14} />
                        编辑
                      </WorkbenchButton>
                      <WorkbenchButton
                        variant="ghost"
                        busy={busyId === item.deviceId}
                        disabled={!online || busyId !== null}
                        onClick={() => void refreshItem(item)}
                      >
                        <RefreshCwIcon size={14} />
                        刷新
                      </WorkbenchButton>
                      <WorkbenchButton
                        variant="ghost"
                        disabled={!online || busyId !== null}
                        onClick={() => setConfirmDelete(item)}
                      >
                        <Trash2Icon size={14} />
                        删除
                      </WorkbenchButton>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Dialog
        open={!!editing}
        onOpenChange={(open) => !open && !saving && setEditing(null)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>编辑公共充电桩信息</DialogTitle>
            <DialogDescription>
              修改后的名称和位置将同步展示给公共桩库里的所有用户。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={saveEdit} className="space-y-4">
            <Field>
              <FieldLabel htmlFor="shared-pile-name">名称</FieldLabel>
              <Input
                id="shared-pile-name"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                placeholder="例如：北门车棚 3 号桩"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="shared-pile-addr">位置</FieldLabel>
              <Input
                id="shared-pile-addr"
                value={editAddress}
                onChange={(e) => setEditAddress(e.target.value)}
                placeholder="例如：1 号楼东侧"
              />
            </Field>
            <DialogFooter>
              <WorkbenchButton
                type="button"
                variant="outline"
                disabled={saving}
                onClick={() => setEditing(null)}
              >
                取消
              </WorkbenchButton>
              <WorkbenchButton type="submit" busy={saving} disabled={!online}>
                保存
              </WorkbenchButton>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!confirmDelete}
        onOpenChange={(open) => !open && !busyId && setConfirmDelete(null)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>从公共桩库删除该充电桩？</DialogTitle>
            <DialogDescription>
              将从全站公共桩库移除「
              {confirmDelete?.name || confirmDelete?.number || confirmDelete?.deviceId}
              」，已将该桩加入常用列表的用户不会受影响。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <WorkbenchButton
              type="button"
              variant="outline"
              disabled={busyId !== null}
              onClick={() => setConfirmDelete(null)}
            >
              取消
            </WorkbenchButton>
            <WorkbenchButton
              type="button"
              variant="destructive"
              busy={busyId !== null && busyId === confirmDelete?.deviceId}
              disabled={!online}
              onClick={() => confirmDelete && void deleteItem(confirmDelete)}
            >
              确认删除
            </WorkbenchButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
function PolicyForm({
  initial,
  section,
  onSectionChange,
  onSaved,
}: {
  initial: RegistrationSettings
  section: PolicySection
  onSectionChange: (value: PolicySection) => void
  onSaved: (value: RegistrationSettings) => void
}) {
  const online = useOnline()
  const setSection = onSectionChange
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
          value={Number(draft[key] ?? 0)}
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
      <div
        className="wb-seg-rail"
        role="tablist"
        aria-label="系统策略分类"
        style={{ marginBottom: "20px" }}
      >
        {(
          [
            ["pool", "公共池与额度"],
            ["registration", "注册与权限"],
            ["reminders", "提醒与断电"],
            ["retention", "数据保留"],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={section === id}
            className={`wb-seg-btn ${section === id ? "wb-seg-active" : ""}`}
            onClick={() => setSection(id)}
          >
            {label}
          </button>
        ))}
      </div>

      {section === "pool" && (
        <>
          <SectionHeading
            title="公共 Cookie 池与免费额度"
            description="免费用户共享公共池查询；贡献全站新桩后自动发放永不过期的额外刷新次数。"
          />
          {toggle(
            "requirePoolShareForUnlimited",
            "需要加入互助公共池才解锁个人 Cookie 权益",
            "开启后，用户需同意将个人 Cookie 加入公共池才算个人 Cookie 用户（不限次刷新+空位提醒）；未加入则按免费用户额度计费。"
          )}
          {toggle(
            "freeUserWatchAllowed",
            "允许免费用户开启空位提醒",
            "关闭时，仅已解锁个人 Cookie 权益的用户可以创建和运行后台空位提醒。"
          )}
          <div className="wb-form-grid">
            {input("freeDailyRefreshQuota")}
            {input("pileContributionBonusQuota")}
            {input("sharedPileCacheTTLSeconds")}
            <Field>
              <FieldLabel htmlFor="policy-cookieGuideUrl">
                Cookie 抓包视频教程链接（可选）
              </FieldLabel>
              <Input
                id="policy-cookieGuideUrl"
                value={draft.cookieGuideUrl ?? ""}
                onChange={(event) =>
                  change("cookieGuideUrl", event.target.value)
                }
                placeholder="https://www.bilibili.com/video/..."
              />
            </Field>
          </div>
        </>
      )}

      {section === "registration" && (
        <>
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
        </>
      )}

      {section === "reminders" && (
        <>
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
        </>
      )}

      {section === "retention" && (
        <>
          <SectionHeading
            title="数据保留"
            description="缩短保留时间会清理过期记录，尚未解决的问题会继续保留。"
          />
          <div className="wb-form-grid">
            {input("statsRetentionDays")}
            {input("portHistoryRetentionDays")}
            {input("notificationRetentionDays")}
          </div>
        </>
      )}
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

function PublicCookiePoolManager({ online }: { online: boolean }) {
  const { data, error, reload } = useRemote<PublicPoolCookie[]>(
    adminApi.publicPool
  )
  const [addOpen, setAddOpen] = useState(false)
  const [editingItem, setEditingItem] = useState<PublicPoolCookie | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<PublicPoolCookie | null>(
    null
  )
  const [label, setLabel] = useState("")
  const [cookie, setCookie] = useState("")
  const [editLabel, setEditLabel] = useState("")
  const [editCookie, setEditCookie] = useState("")
  const [busy, setBusy] = useState(false)
  const [testingId, setTestingId] = useState<string | null>(null)
  const [formError, setFormError] = useState("")

  async function testItem(item: PublicPoolCookie) {
    setTestingId(item.id)
    try {
      const updated = await adminApi.testPublicPoolCookie(item.id)
      reload()
      if (updated.status === "healthy") {
        notify.success(`${item.label} 实测有效`)
      } else {
        notify.warning(`${item.label} 实测已失效，请更新`)
      }
    } catch (reason) {
      notify.error(reason instanceof Error ? reason.message : "测试失败")
    } finally {
      setTestingId(null)
    }
  }

  async function addPoolCookie(event: React.FormEvent) {
    event.preventDefault()
    if (!label.trim() || !cookie.trim()) {
      setFormError("请填写备注名和 Cookie（支持直接粘贴 Copy as cURL）。")
      return
    }
    setBusy(true)
    setFormError("")
    try {
      await adminApi.createPublicPoolCookie({
        label: label.trim(),
        cookie: cookie.trim(),
      })
      setLabel("")
      setCookie("")
      setAddOpen(false)
      reload()
      notify.success("公共池 Cookie 已添加")
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : "添加失败")
    } finally {
      setBusy(false)
    }
  }

  async function saveEditCookie(event: React.FormEvent) {
    event.preventDefault()
    if (!editingItem) return
    if (!editLabel.trim()) {
      setFormError("请填写备注名。")
      return
    }
    setBusy(true)
    setFormError("")
    try {
      const payload: { label?: string; cookie?: string } = {
        label: editLabel.trim(),
      }
      if (editCookie.trim()) {
        payload.cookie = editCookie.trim()
      }
      await adminApi.updatePublicPoolCookie(editingItem.id, payload)
      setEditingItem(null)
      setEditCookie("")
      reload()
      notify.success(
        payload.cookie ? "公共池 Cookie 已替换并恢复可用" : "公共池备注已更新"
      )
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : "更新失败")
    } finally {
      setBusy(false)
    }
  }

  async function toggleEnabled(item: PublicPoolCookie) {
    try {
      await adminApi.updatePublicPoolCookie(item.id, {
        enabled: !item.enabled,
      })
      reload()
      notify.success(item.enabled ? "已停用该公共 Cookie" : "已启用该公共 Cookie")
    } catch (reason) {
      notify.error(reason instanceof Error ? reason.message : "更新失败")
    }
  }

  async function removeItem(item: PublicPoolCookie) {
    setBusy(true)
    try {
      await adminApi.deletePublicPoolCookie(item.id)
      setConfirmDelete(null)
      reload()
      notify.success("已删除公共 Cookie")
    } catch (reason) {
      notify.error(reason instanceof Error ? reason.message : "删除失败")
    } finally {
      setBusy(false)
    }
  }

  const items = data ?? []
  const healthyCount = items.filter(
    (i) => i.enabled && i.status === "healthy"
  ).length

  return (
    <section className="wb-invites">
      <div className="wb-page-rule">
        <div className="flex items-center gap-3">
          <StatusPill
            tone={
              healthyCount >= 2
                ? "idle"
                : healthyCount === 1
                  ? "warning"
                  : "danger"
            }
          >
            {healthyCount} / {items.length} 可用
          </StatusPill>
          <span className="wb-small wb-muted">
            轮询供给免费用户刷新与新桩校验，任一失效自动熔断
          </span>
        </div>
        <WorkbenchButton
          type="button"
          disabled={!online}
          onClick={() => {
            setFormError("")
            setAddOpen(true)
          }}
        >
          <PlusIcon size={15} />
          添加公共 Cookie
        </WorkbenchButton>
      </div>
      {error ? (
        <WorkbenchError message={error} retry={() => void reload()} />
      ) : items.length === 0 ? (
        <WorkbenchEmpty
          icon={DatabaseIcon}
          title="暂无公共池 Cookie"
          description="请添加至少 1~2 个长效 Cookie，或引导已有 Cookie 的用户在账号页加入互助公共池。"
        />
      ) : (
        <div className="wb-table-wrap">
          <table className="wb-data-table">
            <thead>
              <tr>
                <th>凭据备注</th>
                <th>来源</th>
                <th>状态</th>
                <th>最近使用</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td>
                    <strong>{item.label}</strong>
                    {item.lastError && (
                      <p className="wb-small wb-muted">{item.lastError}</p>
                    )}
                  </td>
                  <td>
                    <StatusPill tone={item.ownerUserId ? "brand" : "neutral"}>
                      {item.ownerUserId ? "用户互助" : "管理员"}
                    </StatusPill>
                  </td>
                  <td>
                    <StatusPill
                      tone={
                        !item.enabled
                          ? "neutral"
                          : item.status === "healthy"
                            ? "idle"
                            : "warning"
                      }
                    >
                      {!item.enabled
                        ? "已停用"
                        : item.status === "healthy"
                          ? "正常"
                          : "已失效"}
                    </StatusPill>
                  </td>
                  <td>
                    {item.lastUsedAt
                      ? formatSnapshotTime(item.lastUsedAt, true)
                      : "尚未使用"}
                  </td>
                  <td>
                    <div className="wb-row-actions">
                      <WorkbenchButton
                        type="button"
                        variant="outline"
                        busy={testingId === item.id}
                        disabled={!online || testingId !== null}
                        onClick={() => void testItem(item)}
                      >
                        测试
                      </WorkbenchButton>
                      <WorkbenchButton
                        type="button"
                        variant="ghost"
                        disabled={!online || busy}
                        onClick={() => {
                          setFormError("")
                          setEditingItem(item)
                          setEditLabel(item.label)
                          setEditCookie("")
                        }}
                      >
                        <PencilIcon size={14} />
                        更新
                      </WorkbenchButton>
                      <Switch
                        checked={item.enabled}
                        disabled={!online || busy}
                        onCheckedChange={() => void toggleEnabled(item)}
                        aria-label={`切换 ${item.label}`}
                      />
                      <WorkbenchButton
                        type="button"
                        variant="ghost"
                        disabled={!online || busy}
                        onClick={() => setConfirmDelete(item)}
                      >
                        <Trash2Icon size={14} />
                        删除
                      </WorkbenchButton>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Dialog open={addOpen} onOpenChange={(open) => !busy && setAddOpen(open)}>
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>添加公共 Cookie</DialogTitle>
            <DialogDescription>
              支持直接粘贴抓包工具复制的 Copy as cURL、请求头或纯 Cookie 字符串。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={addPoolCookie} className="wb-form-stack">
            <Field>
              <FieldLabel htmlFor="pool-cookie-label">备注名</FieldLabel>
              <Input
                id="pool-cookie-label"
                value={label}
                onChange={(event) => setLabel(event.target.value)}
                placeholder="例如：手机号 1 / 备用号 2"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="pool-cookie-value">Cookie / cURL</FieldLabel>
              <textarea
                id="pool-cookie-value"
                className="wb-input wb-textarea"
                autoComplete="off"
                value={cookie}
                onChange={(event) => setCookie(event.target.value)}
                placeholder="粘贴 curl 'https://ele.mocele.com/...' 或 wxopenid=...; info=..."
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
                disabled={busy}
                onClick={() => setAddOpen(false)}
              >
                取消
              </WorkbenchButton>
              <WorkbenchButton type="submit" busy={busy} disabled={!online}>
                保存入池
              </WorkbenchButton>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!editingItem}
        onOpenChange={(open) => !open && !busy && setEditingItem(null)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>
              {editingItem?.ownerUserId ? "编辑互助凭据备注" : "更新公共 Cookie"}
            </DialogTitle>
            <DialogDescription>
              {editingItem?.ownerUserId
                ? "用户互助 Cookie 仅可由用户本人在账号页更新，管理员此处仅可修改备注名。"
                : "可修改备注名，或粘贴新 Cookie / Copy as cURL 原地替换失效凭据（留空则保留原 Cookie）。"}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={saveEditCookie} className="wb-form-stack">
            <Field>
              <FieldLabel htmlFor="edit-pool-cookie-label">备注名</FieldLabel>
              <Input
                id="edit-pool-cookie-label"
                value={editLabel}
                onChange={(event) => setEditLabel(event.target.value)}
                placeholder="例如：手机号 1 / 备用号 2"
              />
            </Field>
            {!editingItem?.ownerUserId && (
              <Field>
                <FieldLabel htmlFor="edit-pool-cookie-value">
                  新 Cookie / cURL（可选）
                </FieldLabel>
                <textarea
                  id="edit-pool-cookie-value"
                  className="wb-input wb-textarea"
                  autoComplete="off"
                  value={editCookie}
                  onChange={(event) => setEditCookie(event.target.value)}
                  placeholder="留空则仅修改备注名；粘贴新内容将替换原 Cookie 并重置为正常状态"
                />
              </Field>
            )}
            {formError && (
              <p role="alert" className="wb-form-error">
                {formError}
              </p>
            )}
            <DialogFooter>
              <WorkbenchButton
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => setEditingItem(null)}
              >
                取消
              </WorkbenchButton>
              <WorkbenchButton type="submit" busy={busy} disabled={!online}>
                保存更新
              </WorkbenchButton>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!confirmDelete}
        onOpenChange={(open) => !open && !busy && setConfirmDelete(null)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>删除该公共 Cookie？</DialogTitle>
            <DialogDescription>
              删除「{confirmDelete?.label}」后将立即从公共池移除
              {confirmDelete?.ownerUserId
                ? "，该互助用户将切回免费额度模式。"
                : "，不可恢复。"}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <WorkbenchButton
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => setConfirmDelete(null)}
            >
              取消
            </WorkbenchButton>
            <WorkbenchButton
              type="button"
              variant="destructive"
              busy={busy}
              disabled={!online}
              onClick={() => confirmDelete && void removeItem(confirmDelete)}
            >
              确认删除
            </WorkbenchButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
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
    <section className="wb-invites">
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
    </section>
  )
}

const auditLabels: Record<string, string> = {
  "user.create": "创建用户",
  "user.update": "更新用户",
  "user.delete": "删除用户",
  "user.password_reset": "重置密码",
  "user.refresh": "刷新用户设备",
  "user.quota_update": "调整用户奖励次数",
  "settings.update": "更新系统策略",
  "incident.update": "更新异常",
  "invite.create": "创建邀请码",
  "invite.delete": "删除邀请码",
  "public_pool.create": "新增公共 Cookie",
  "public_pool.update": "更新公共 Cookie",
  "public_pool.delete": "删除公共 Cookie",
  "public_pool.test": "实测公共 Cookie",
  "shared_pile.update": "编辑公共桩信息",
  "shared_pile.refresh": "刷新公共桩",
  "shared_pile.delete": "删除公共桩",
  "announcement.create": "发布公告",
  "announcement.update": "更新公告",
  "announcement.delete": "删除公告",
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
    <section className="wb-audit">
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
    </section>
  )
}
