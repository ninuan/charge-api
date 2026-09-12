"use client"

import { useState } from "react"
import {
  ArrowRight,
  Bell,
  BellOff,
  Check,
  CheckCheck,
  ChevronRight,
  Clock3,
  Inbox,
  MessageCircle,
  Plus,
  ShieldAlert,
  Trash2,
} from "lucide-react"
import { usePrototype } from "./context"
import {
  durationLabel,
  isActionNotice,
  timeLabel,
  watchLabel,
  type DemoNotice,
} from "./model"
import {
  BackLink,
  Badge,
  EmptyState,
  ErrorState,
  LoadingState,
  PageHeading,
  PButton,
  SearchField,
  SectionHeading,
} from "./ui"

export function RemindersPage() {
  const { data, go, openModal, scenario, offline, commit, busy } =
    usePrototype()
  const [tab, setTab] = useState("active")
  const [query, setQuery] = useState("")
  const rules = scenario === "empty" ? [] : data.watches
  const active = rules.filter((w) => w.status === "active")
  const shown = rules
    .filter((w) =>
      tab === "active" ? w.status === "active" : w.status !== "active"
    )
    .filter((w) =>
      data.piles.find((p) => p.id === w.deviceId)?.name.includes(query.trim())
    )
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="空闲提醒"
        description="不用一直刷新。等到有空闲口，告诉你一声。"
        actions={
          <PButton
            tone="brand"
            disabled={offline}
            onClick={() => openModal({ kind: "watch" })}
          >
            <Plus size={16} />
            新建提醒
          </PButton>
        }
      />
      <div className="cp-page-rule">
        <div className="cp-section-tabs">
          <button
            aria-pressed={tab === "active"}
            onClick={() => setTab("active")}
          >
            正在等待 <span>{active.length}</span>
          </button>
          <button
            aria-pressed={tab === "ended"}
            onClick={() => setTab("ended")}
          >
            已结束 <span>{rules.length - active.length}</span>
          </button>
        </div>
        <SearchField
          label="搜索提醒"
          placeholder="搜索充电桩"
          value={query}
          onChange={setQuery}
        />
      </div>
      {scenario === "loading" ? (
        <LoadingState label="正在读取提醒" />
      ) : scenario === "error" ? (
        <ErrorState />
      ) : !shown.length ? (
        <EmptyState
          icon={Bell}
          title={
            query
              ? "没有找到这条提醒"
              : tab === "active"
                ? "现在没有需要等待的地方"
                : "还没有已结束的提醒"
          }
          description={
            query
              ? "试试充电桩的名称。"
              : "在充电桩没有空闲口时，开启一次临时提醒即可。"
          }
          action={
            query ? (
              <PButton onClick={() => setQuery("")}>清除搜索</PButton>
            ) : (
              <PButton onClick={() => openModal({ kind: "watch" })}>
                <Plus size={15} />
                选择充电桩
              </PButton>
            )
          }
        />
      ) : (
        <div className="cp-watch-list">
          {shown.map((w) => {
            const pile = data.piles.find((p) => p.id === w.deviceId)
            return (
              <article className="cp-watch-row" key={w.id}>
                <div
                  className={`cp-watch-icon ${w.status === "active" ? "cp-watch-icon-active" : ""}`}
                >
                  {w.status === "active" ? (
                    <Bell size={22} />
                  ) : w.status === "notified" ? (
                    <Check size={22} />
                  ) : (
                    <BellOff size={22} />
                  )}
                </div>
                <div className="cp-watch-main">
                  <div className="cp-watch-title">
                    <button
                      onClick={() => go({ view: "piles", id: w.deviceId })}
                    >
                      {pile?.name || "已移除的充电桩"}
                      <ChevronRight size={14} />
                    </button>
                    <Badge
                      tone={
                        w.status === "active"
                          ? "brand"
                          : w.status === "notified"
                            ? "idle"
                            : "neutral"
                      }
                    >
                      {watchLabel[w.status]}
                    </Badge>
                  </div>
                  <p>
                    {w.status === "active" ? (
                      <>
                        等到 {timeLabel(w.expiresAt)} · 下次检查{" "}
                        {timeLabel(w.nextCheckAt)}
                      </>
                    ) : w.status === "notified" ? (
                      "发现空闲，已通知一次并自动结束"
                    ) : w.status === "expired" ? (
                      "等待时间已结束，期间未发现空闲"
                    ) : w.status === "legacy" ? (
                      "升级前保留的固定规则，只读，不再运行"
                    ) : (
                      "你已取消这次等待"
                    )}
                  </p>
                  <span className="cp-muted cp-small">
                    {timeLabel(w.createdAt, true)} 开始 ·{" "}
                    {durationLabel[w.duration]} · 整桩任一口空闲时提醒
                  </span>
                </div>
                <div className="cp-row-actions">
                  {w.status === "active" ? (
                    <>
                      <PButton
                        disabled={offline}
                        onClick={() =>
                          openModal({ kind: "watch", id: w.deviceId })
                        }
                      >
                        调整时长
                      </PButton>
                      <PButton
                        tone="quiet"
                        disabled={offline || !!busy}
                        onClick={() =>
                          void commit((d) => {
                            const target = d.watches.find(
                              (item) => item.id === w.id
                            )
                            if (target) target.status = "cancelled"
                          }, "这次提醒已取消。")
                        }
                      >
                        取消提醒
                      </PButton>
                    </>
                  ) : w.status !== "legacy" && pile ? (
                    <PButton
                      tone="quiet"
                      onClick={() =>
                        openModal({ kind: "watch", id: w.deviceId })
                      }
                    >
                      再次提醒
                      <ArrowRight size={14} />
                    </PButton>
                  ) : null}
                </div>
              </article>
            )
          })}
        </div>
      )}
      <section className="cp-reminder-policy">
        <SectionHeading title="提醒如何运行" />
        <div className="cp-policy-inline">
          <div>
            <Clock3 size={17} />
            <p>
              每 {data.settings.watchRefreshIntervalMinutes} 分钟检查一次
              <br />
              <span>仅在你开启提醒后运行</span>
            </p>
          </div>
          <div>
            <Bell size={17} />
            <p>
              发现空闲后自动结束
              <br />
              <span>不会替你预留充电口</span>
            </p>
          </div>
          <div>
            <MessageCircle size={17} />
            <p>
              站内通知{data.wxBound && data.wxEnabled ? " + 微信提醒" : ""}
              <br />
              <button
                className="cp-text-link"
                onClick={() => go({ view: "account", tab: "notifications" })}
              >
                管理接收方式
                <ArrowRight size={12} />
              </button>
            </p>
          </div>
        </div>
        <div className="cp-quota-line">
          <span>
            正在关注 {active.length} / {data.settings.watchPileLimitPerUser} 台
          </span>
          <span>
            今日检查{" "}
            {scenario === "quota" ? data.settings.watchDailyRefreshQuota : 18} /{" "}
            {data.settings.watchDailyRefreshQuota} 次
          </span>
          <span>计划断电 23:00–06:00（上海时间）</span>
        </div>
      </section>
    </div>
  )
}

export function NotificationsPage() {
  const { data, route, go, mutate, commit, offline, scenario, openModal } =
    usePrototype()
  const [filter, setFilter] = useState("all")
  const [query, setQuery] = useState("")
  const notices = scenario === "empty" ? [] : data.notices
  const unread = notices.filter((n) => !n.read).length
  const items = notices
    .filter(
      (n) =>
        filter === "all" ||
        (filter === "unread"
          ? !n.read
          : filter === "pending"
            ? isActionNotice(n) && !n.resolved
            : isActionNotice(n) && n.resolved)
    )
    .filter((n) => `${n.title}${n.message}`.includes(query.trim()))
  const selected = notices.find((n) => n.id === route.id)
  function select(n: DemoNotice) {
    if (!offline && scenario !== "error")
      mutate((d) => {
        const item = d.notices.find((x) => x.id === n.id)
        if (item) item.read = true
      })
    go({ view: "notifications", id: n.id })
  }
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="通知"
        description={
          unread
            ? `${unread} 条未读。空闲消息和需要处理的事情，都在这里。`
            : "没有未读消息，可以安心忙自己的事。"
        }
        actions={
          <>
            <PButton
              tone="quiet"
              disabled={offline || unread === 0}
              onClick={() =>
                void commit(
                  (d) =>
                    d.notices.forEach((n) => {
                      n.read = true
                    }),
                  "所有通知已标为已读。待处理问题仍然保留。"
                )
              }
            >
              <CheckCheck size={16} />
              全部已读
            </PButton>
            <PButton
              tone="quiet"
              disabled={
                offline || !notices.some((n) => isActionNotice(n) && n.resolved)
              }
              onClick={() => openModal({ kind: "clear-notices" })}
            >
              <Trash2 size={15} />
              清理已解决
            </PButton>
          </>
        }
      />
      <div className="cp-page-rule">
        <div className="cp-section-tabs">
          {[
            ["all", "全部"],
            ["unread", "未读"],
            ["pending", "待处理"],
            ["resolved", "已解决"],
          ].map(([v, label]) => (
            <button
              key={v}
              aria-pressed={filter === v}
              onClick={() => {
                setFilter(v)
                go({ view: "notifications" })
              }}
            >
              {label}
              {v === "unread" && unread > 0 && <span>{unread}</span>}
            </button>
          ))}
        </div>
        <SearchField
          label="搜索通知"
          placeholder="搜索通知"
          value={query}
          onChange={setQuery}
        />
      </div>
      {scenario === "loading" ? (
        <LoadingState label="正在读取通知" />
      ) : scenario === "error" ? (
        <ErrorState />
      ) : selected ? (
        <NotificationDetail notice={selected} />
      ) : !items.length ? (
        <EmptyState
          icon={Inbox}
          title={
            query
              ? "没有找到相关通知"
              : filter === "pending"
                ? "没有需要处理的问题"
                : "这里暂时没有通知"
          }
          description={
            query
              ? "换个关键词，或清除筛选看看。"
              : "有空闲口或状态变化时，消息会出现在这里。"
          }
          action={
            query ? (
              <PButton onClick={() => setQuery("")}>清除搜索</PButton>
            ) : (
              <PButton tone="quiet" onClick={() => go({ view: "piles" })}>
                看看常用桩
                <ArrowRight size={15} />
              </PButton>
            )
          }
        />
      ) : (
        <div className="cp-notice-list">
          {items.map((n) => (
            <button
              key={n.id}
              className={`cp-notice-row ${!n.read ? "cp-unread" : ""}`}
              onClick={() => select(n)}
            >
              <span
                className={`cp-notice-icon ${isActionNotice(n) && !n.resolved ? "cp-tone-warning" : "cp-tone-idle"}`}
              >
                {isActionNotice(n) && !n.resolved ? (
                  <ShieldAlert size={19} />
                ) : (
                  <Bell size={19} />
                )}
              </span>
              <span className="cp-notice-main">
                <span className="cp-notice-title">
                  {n.title}
                  {!n.read && <i className="cp-unread-dot" />}
                  {isActionNotice(n) && (
                    <Badge
                      tone={n.resolved ? "neutral" : "warning"}
                      dot={false}
                    >
                      {n.resolved ? "已解决" : "待处理"}
                    </Badge>
                  )}
                </span>
                <span className="cp-notice-message">{n.message}</span>
                <span className="cp-small cp-muted">
                  {n.deviceId
                    ? data.piles.find((p) => p.id === n.deviceId)?.name ||
                      "已移除的充电桩"
                    : "账户连接"}
                </span>
              </span>
              <time>{timeLabel(n.createdAt, true)}</time>
              <ChevronRight size={15} />
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function NotificationDetail({ notice: n }: { notice: DemoNotice }) {
  const { go, data, commit, offline } = usePrototype()
  const pile = data.piles.find((p) => p.id === n.deviceId)
  return (
    <section className="cp-notice-detail">
      <BackLink onClick={() => go({ view: "notifications" })}>
        全部通知
      </BackLink>
      <div className="cp-notice-detail-title">
        <span className="cp-notice-icon">
          <Bell size={24} />
        </span>
        <div>
          <h2>{n.title}</h2>
          <p>
            {timeLabel(n.createdAt, true)} · {n.read ? "已读" : "未读"}
            {isActionNotice(n)
              ? n.resolved
                ? " · 问题已解决"
                : " · 仍需处理"
              : ""}
          </p>
        </div>
      </div>
      <p className="cp-notice-body">{n.message}</p>
      <div className="cp-delivery-fact">
        <MessageCircle size={17} />
        <div>
          <strong>
            {n.delivery === "submitted"
              ? "已提交微信渠道"
              : n.delivery === "queued"
                ? "微信提醒等待发送"
                : n.delivery === "failed"
                  ? "微信提醒发送失败"
                  : n.delivery === "suppressed"
                    ? "免打扰时段未向微信发送"
                    : "已保存在站内通知"}
          </strong>
          <p>
            {n.delivery === "submitted"
              ? "渠道已受理，不代表手机已送达或已读。请在微信或 WxPusher 中确认。"
              : "第三方渠道的状态不影响这条站内通知。"}
          </p>
        </div>
      </div>
      <div className="cp-row-actions">
        {n.type === "credential_expired" ? (
          <PButton
            tone="brand"
            onClick={() => go({ view: "account", tab: "connection" })}
          >
            查看账户连接
            <ArrowRight size={15} />
          </PButton>
        ) : pile ? (
          <PButton
            tone="brand"
            onClick={() => go({ view: "piles", id: pile.id, port: n.portId })}
          >
            查看充电桩
            <ArrowRight size={15} />
          </PButton>
        ) : (
          <PButton onClick={() => go({ view: "piles" })}>
            返回常用充电桩
          </PButton>
        )}
        {!n.read && (
          <PButton
            disabled={offline}
            onClick={() =>
              void commit((d) => {
                const notice = d.notices.find((item) => item.id === n.id)
                if (notice) notice.read = true
              }, "通知已标为已读。")
            }
          >
            标为已读
          </PButton>
        )}
      </div>
    </section>
  )
}
