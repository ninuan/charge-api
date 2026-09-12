"use client"

import {
  ArrowRightIcon,
  BellIcon,
  BellOffIcon,
  CheckIcon,
  ChevronRightIcon,
  Clock3Icon,
  MessageCircleIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { Suspense, useState } from "react"
import { WatchRuleDialog } from "@/components/watch-rule-dialog"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import type { WatchRule, WatchTemporaryDuration } from "@/lib/api/generated"
import { useDashboard } from "@/lib/dashboard-context"
import { useWatch } from "@/lib/watch-context"
import { useMinuteClock, useOnline } from "@/lib/browser-state"
import { notify } from "@/lib/feedback"
import {
  formatCompletionReason,
  formatNextCheck,
  formatPowerWindow,
  formatRemainingTime,
  formatWatchTimestamp,
  getTemporaryDurationOptions,
} from "@/lib/watch-format"
import { dashboardHref, updatePageQuery } from "@/lib/workbench"
import {
  SectionHeading,
  StatusPill,
  WorkbenchButton,
  WorkbenchEmpty,
  WorkbenchError,
  WorkbenchLoading,
  WorkbenchSearch,
} from "./surfaces"
import { UserWorkbenchShell } from "./user-shell"

export function RemindersPage() {
  return (
    <Suspense fallback={<WorkbenchLoading />}>
      <RemindersContent />
    </Suspense>
  )
}
function RemindersContent() {
  const now = useMinuteClock()
  const params = useSearchParams(),
    online = useOnline()
  const { snapshot } = useDashboard()
  const {
    rules,
    overview,
    loading,
    loaded,
    error,
    load,
    updateRule,
    deleteRule,
  } = useWatch()
  const [createTarget, setCreateTarget] = useState<{ pileId?: string } | null>(
    null
  )
  const [action, setAction] = useState<{
    kind: "extend" | "cancel" | "delete"
    rule: WatchRule
  } | null>(null)
  const [duration, setDuration] = useState<WatchTemporaryDuration>("2h")
  const [pending, setPending] = useState(false)
  const [actionError, setActionError] = useState("")
  const query = params.get("q") ?? "",
    tab = params.get("view") === "ended" ? "ended" : "active",
    pileFilter = params.get("pile")
  const active = rules.filter(
    (rule) => rule.mode === "temporary" && rule.enabled && !rule.completedAt
  )
  const completed = rules.filter(
    (rule) => rule.mode !== "temporary" || !!rule.completedAt || !rule.enabled
  )
  const items = (tab === "active" ? active : completed)
    .filter((rule) => !pileFilter || rule.deviceId === pileFilter)
    .filter((rule) => {
      const pile = snapshot.piles.find((p) => p.id === rule.deviceId)
      return `${pile?.name ?? ""} ${pile?.number ?? ""} ${rule.deviceId}`
        .toLocaleLowerCase()
        .includes(query.trim().toLocaleLowerCase())
    })
  const disabled =
    !online ||
    !overview?.backgroundRemindersEnabled ||
    !overview.accountRefreshEnabled ||
    !snapshot.piles.length
  function openAction(kind: "extend" | "cancel" | "delete", rule: WatchRule) {
    setActionError("")
    setDuration("2h")
    setAction({ kind, rule })
  }
  async function apply() {
    if (!action) return
    setPending(true)
    setActionError("")
    try {
      if (action.kind === "delete") await deleteRule(action.rule.id)
      else
        await updateRule(
          action.rule.id,
          action.kind === "cancel" ? { cancel: true } : { duration }
        )
      setAction(null)
      notify.success(
        action.kind === "delete"
          ? "提醒记录已删除"
          : action.kind === "cancel"
            ? "这次提醒已取消"
            : "提醒时长已更新"
      )
    } catch (reason) {
      setActionError(
        reason instanceof Error ? reason.message : "操作未完成，请重试。"
      )
    } finally {
      setPending(false)
    }
  }
  return (
    <UserWorkbenchShell
      title="空闲提醒"
      activeSection="reminders"
      description="不用一直刷新。等到有空闲口，告诉你一声。"
      actions={
        <>
          <WorkbenchButton
            variant="ghost"
            disabled={!online || loading}
            onClick={() => void load().catch(() => undefined)}
            aria-label="更新提醒"
          >
            <RefreshCwIcon size={15} />
          </WorkbenchButton>
          <WorkbenchButton
            disabled={disabled}
            onClick={() => setCreateTarget({})}
          >
            <PlusIcon size={16} />
            新建提醒
          </WorkbenchButton>
        </>
      }
    >
      <div className="wb-standard-page">
        {overview &&
          (!overview.backgroundRemindersEnabled ||
            !overview.accountRefreshEnabled) && (
            <div className="wb-state-banner wb-banner-warning" role="status">
              <BellOffIcon size={16} />
              <div>
                <strong>暂时不能开启后台提醒</strong>
                <span>
                  {overview.accountRefreshEnabled
                    ? "管理员关闭了后台提醒，请稍后再试。"
                    : "当前账户没有远端刷新权限，请联系管理员。"}
                </span>
              </div>
            </div>
          )}
        <div className="wb-page-rule">
          <div className="wb-section-tabs">
            <button
              aria-pressed={tab === "active"}
              onClick={() => updatePageQuery({ view: null })}
            >
              正在等待 <span>{active.length}</span>
            </button>
            <button
              aria-pressed={tab === "ended"}
              onClick={() => updatePageQuery({ view: "ended" })}
            >
              已结束 <span>{completed.length}</span>
            </button>
          </div>
          <WorkbenchSearch
            value={query}
            onChange={(value) => updatePageQuery({ q: value }, true)}
            label="搜索提醒"
            placeholder="搜索充电桩或桩号"
          />
        </div>
        {pileFilter && (
          <div className="wb-selected-scope">
            <span>
              只看：
              {snapshot.piles.find((p) => p.id === pileFilter)?.name ||
                pileFilter}
            </span>
            <button
              className="wb-text-link"
              onClick={() => updatePageQuery({ pile: null })}
            >
              查看全部提醒
            </button>
          </div>
        )}
        {loading && !loaded ? (
          <WorkbenchLoading label="正在读取提醒" />
        ) : error ? (
          <WorkbenchError
            message={error}
            retry={() => void load().catch(() => undefined)}
          />
        ) : !items.length ? (
          <WorkbenchEmpty
            icon={BellIcon}
            title={
              query || pileFilter
                ? "没有匹配的提醒"
                : tab === "active"
                  ? "现在没有需要等待的地方"
                  : "还没有已结束的提醒"
            }
            description={
              query || pileFilter
                ? "调整搜索或筛选条件，再试一次。"
                : "没有空闲口时，开启一次临时提醒即可。"
            }
            action={
              query || pileFilter ? (
                <WorkbenchButton
                  variant="outline"
                  onClick={() => updatePageQuery({ q: null, pile: null })}
                >
                  清除筛选
                </WorkbenchButton>
              ) : (
                <WorkbenchButton
                  disabled={disabled}
                  onClick={() => setCreateTarget({})}
                >
                  选择充电桩
                  <ArrowRightIcon size={15} />
                </WorkbenchButton>
              )
            }
          />
        ) : (
          <div className="wb-watch-list">
            {items.map((rule) => {
              const pile = snapshot.piles.find((p) => p.id === rule.deviceId),
                isActive =
                  rule.mode === "temporary" && rule.enabled && !rule.completedAt
              return (
                <article className="wb-watch-row" key={rule.id}>
                  <span
                    className={`wb-watch-icon ${isActive ? "wb-watch-icon-active" : ""}`}
                  >
                    {isActive ? (
                      <BellIcon size={22} />
                    ) : rule.completionReason === "notified" ? (
                      <CheckIcon size={22} />
                    ) : (
                      <BellOffIcon size={22} />
                    )}
                  </span>
                  <div className="wb-watch-main">
                    <div className="wb-watch-title">
                      <Link href={dashboardHref(rule.deviceId)}>
                        {pile?.name || pile?.number || "已移除的充电桩"}
                        <ChevronRightIcon size={14} />
                      </Link>
                      <StatusPill
                        tone={
                          isActive
                            ? "brand"
                            : rule.completionReason === "notified"
                              ? "idle"
                              : "neutral"
                        }
                      >
                        {isActive
                          ? "等待空闲"
                          : rule.mode === "recurring"
                            ? "旧规则 · 已停用"
                            : rule.completionReason === "notified"
                              ? "已提醒"
                              : rule.completionReason === "cancelled"
                                ? "已取消"
                                : "已到期"}
                      </StatusPill>
                    </div>
                    <p>
                      {isActive
                        ? `剩余 ${formatRemainingTime(rule.expiresAt, now)} · 下次检查 ${formatNextCheck(rule.nextCheckAt, now)}`
                        : rule.mode === "recurring"
                          ? "升级前的固定规则只读，不再运行。"
                          : formatCompletionReason(rule.completionReason)}
                    </p>
                    <span className="wb-small wb-muted">
                      {formatWatchTimestamp(rule.createdAt)} 开始 ·
                      整桩任一口空闲时提醒
                      {rule.expiresAt
                        ? ` · 截止 ${formatWatchTimestamp(rule.expiresAt)}`
                        : ""}
                    </span>
                  </div>
                  <div className="wb-row-actions">
                    {isActive ? (
                      <>
                        <WorkbenchButton
                          variant="outline"
                          disabled={!online}
                          onClick={() => openAction("extend", rule)}
                        >
                          调整时长
                        </WorkbenchButton>
                        <WorkbenchButton
                          variant="ghost"
                          disabled={!online}
                          onClick={() => openAction("cancel", rule)}
                        >
                          取消提醒
                        </WorkbenchButton>
                      </>
                    ) : (
                      <>
                        {pile && rule.mode === "temporary" && (
                          <WorkbenchButton
                            variant="ghost"
                            disabled={disabled}
                            onClick={() =>
                              setCreateTarget({ pileId: rule.deviceId })
                            }
                          >
                            再次提醒
                            <ArrowRightIcon size={14} />
                          </WorkbenchButton>
                        )}
                        <WorkbenchButton
                          variant="ghost"
                          disabled={!online}
                          aria-label="删除提醒记录"
                          onClick={() => openAction("delete", rule)}
                        >
                          <Trash2Icon size={14} />
                        </WorkbenchButton>
                      </>
                    )}
                  </div>
                </article>
              )
            })}
          </div>
        )}
        {overview && (
          <section className="wb-reminder-policy">
            <SectionHeading title="提醒如何运行" />
            <div className="wb-policy-inline">
              <div>
                <Clock3Icon size={17} />
                <p>
                  每 {overview.refreshIntervalMinutes} 分钟检查一次
                  <br />
                  <span>仅在你开启提醒后运行</span>
                </p>
              </div>
              <div>
                <BellIcon size={17} />
                <p>
                  找到空闲后自动结束
                  <br />
                  <span>不预留或锁定充电口</span>
                </p>
              </div>
              <div>
                <MessageCircleIcon size={17} />
                <p>
                  站内通知与可选接收方式
                  <br />
                  <Link
                    className="wb-text-link"
                    href="/account?tab=notifications"
                  >
                    管理通知设置
                    <ArrowRightIcon size={13} />
                  </Link>
                </p>
              </div>
            </div>
            <div className="wb-quota-line">
              <span>
                正在关注 {overview.reminderPileCount} /{" "}
                {overview.reminderPileLimit} 台
              </span>
              <span>
                今日检查 {overview.dailyQuotaUsed} / {overview.dailyQuotaLimit}{" "}
                次
              </span>
              {overview.scheduledPowerOffEnabled && (
                <span>
                  计划断电{" "}
                  {formatPowerWindow(
                    overview.scheduledPowerOffStartMinute,
                    overview.scheduledPowerOffEndMinute
                  )}{" "}
                  · {overview.scheduledPowerOffTimezone}
                </span>
              )}
            </div>
          </section>
        )}
        {createTarget && (
          <WatchRuleDialog
            key={createTarget.pileId || "new"}
            piles={snapshot.piles}
            target={createTarget}
            open
            onOpenChange={(open) => !open && setCreateTarget(null)}
          />
        )}
        <Dialog
          open={!!action}
          onOpenChange={(open) => !open && !pending && setAction(null)}
        >
          <DialogContent className="workbench-dialog">
            <DialogHeader>
              <DialogTitle>
                {action?.kind === "extend"
                  ? "调整这次等待的时长"
                  : action?.kind === "cancel"
                    ? "结束这次提醒？"
                    : "删除这条提醒记录？"}
              </DialogTitle>
              <DialogDescription>
                {action?.kind === "extend"
                  ? "从现在重新计算截止时间；不会超过当天计划断电时间。"
                  : action?.kind === "cancel"
                    ? "停止等待，不影响充电桩和已有通知。"
                    : "删除后不能恢复，已经生成的站内通知仍会保留。"}
              </DialogDescription>
            </DialogHeader>
            {action?.kind === "extend" && (
              <Field>
                <FieldLabel htmlFor="reminder-duration">继续等多久</FieldLabel>
                <select
                  id="reminder-duration"
                  className="wb-input"
                  value={duration}
                  onChange={(event) =>
                    setDuration(event.target.value as WatchTemporaryDuration)
                  }
                >
                  {getTemporaryDurationOptions(
                    !!overview?.scheduledPowerOffEnabled
                  ).map((option) => (
                    <option value={option.value} key={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </Field>
            )}
            {actionError && (
              <p role="alert" className="wb-form-error">
                {actionError}
              </p>
            )}
            <DialogFooter>
              <WorkbenchButton
                variant="outline"
                disabled={pending}
                onClick={() => setAction(null)}
              >
                返回
              </WorkbenchButton>
              <WorkbenchButton
                variant={action?.kind === "delete" ? "destructive" : "default"}
                busy={pending}
                disabled={!online}
                onClick={() => void apply()}
              >
                {action?.kind === "extend"
                  ? "保存时长"
                  : action?.kind === "cancel"
                    ? "确认结束"
                    : "确认删除"}
              </WorkbenchButton>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </UserWorkbenchShell>
  )
}
