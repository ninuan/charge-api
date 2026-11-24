"use client"

import {
  ArrowRightIcon,
  CheckIcon,
  ExternalLinkIcon,
  KeyRoundIcon,
  LogOutIcon,
  ShieldCheckIcon,
} from "lucide-react"
import dynamic from "next/dynamic"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useCallback, useEffect, useState } from "react"
import { AppShell } from "@/components/app-shell"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { useAuth } from "@/lib/auth-context"
import { DashboardProvider, useDashboard } from "@/lib/dashboard-context"
import { NotificationProvider } from "@/lib/notification-context"
import { WatchProvider, useWatch } from "@/lib/watch-context"
import { useOnline } from "@/lib/browser-state"
import { notify } from "@/lib/feedback"
import { formatQuotaResetTime, updatePageQuery } from "@/lib/workbench"
import { DashboardSession } from "./dashboard-session"
import { UserWorkbenchShell } from "./user-shell"
import {
  SectionHeading,
  StatusPill,
  WorkbenchButton,
  WorkbenchError,
  WorkbenchLoading,
} from "./surfaces"

const AccountSecurityPanel = dynamic(
  () =>
    import("@/components/account-security-panel").then(
      (module) => module.AccountSecurityPanel
    ),
  { ssr: false }
)

const AppearanceSettings = dynamic(
  () =>
    import("@/components/appearance-settings").then(
      (module) => module.AppearanceSettings
    ),
  { ssr: false }
)

const BrowserNotificationSetting = dynamic(
  () =>
    import("@/components/browser-notification-setting").then(
      (module) => module.BrowserNotificationSetting
    ),
  { ssr: false }
)

const WxPusherChannelCard = dynamic(
  () =>
    import("@/components/wxpusher-channel-card").then(
      (module) => module.WxPusherChannelCard
    ),
  { ssr: false }
)

const QuietHoursForm = dynamic(
  () =>
    import("@/components/quiet-hours-form").then(
      (module) => module.QuietHoursForm
    ),
  { ssr: false }
)

export function AccountWorkbench() {
  return (
    <Suspense fallback={<WorkbenchLoading />}>
      <AccountAccess />
    </Suspense>
  )
}
function AccountAccess() {
  const router = useRouter(),
    { currentUser, fetchMe } = useAuth()
  const [error, setError] = useState("")
  const authorize = useCallback(async () => {
    setError("")
    try {
      const user = currentUser ?? (await fetchMe())
      if (!user) router.replace("/login")
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "暂时无法验证账户。")
    }
  }, [currentUser, fetchMe, router])
  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) void authorize()
    })
    return () => {
      active = false
    }
  }, [authorize])
  if (!currentUser)
    return (
      <AppShell
        title="账户与设置"
        activeSection="account"
        description="正在确认登录状态。"
      >
        {error ? (
          <WorkbenchError message={error} retry={() => void authorize()} />
        ) : (
          <WorkbenchLoading label="正在读取账户" />
        )}
      </AppShell>
    )
  if (currentUser.role === "admin") return <AccountContent admin />
  return (
    <DashboardProvider>
      <WatchProvider>
        <NotificationProvider>
          <DashboardSession>
            <AccountContent />
          </DashboardSession>
        </NotificationProvider>
      </WatchProvider>
    </DashboardProvider>
  )
}
function AccountContent({ admin = false }: { admin?: boolean }) {
  const { currentUser } = useAuth(),
    params = useSearchParams(),
    online = useOnline()
  const tabs = admin
    ? [
        ["security", "账户安全"],
        ["appearance", "外观"],
      ]
    : [
        ["connection", "平台连接"],
        ["notifications", "通知设置"],
        ["security", "账户安全"],
        ["appearance", "外观"],
      ]
  const requested = params.get("tab"),
    tab = tabs.some(([value]) => value === requested)
      ? requested!
      : admin
        ? "security"
        : "connection"
  const Shell = admin ? AppShell : UserWorkbenchShell
  return (
    <Shell
      title="账户与设置"
      activeSection="account"
      description="管理连接、接收方式和你的使用习惯。"
      narrow
      backHref={admin ? "/admin" : "/dashboard"}
      backLabel={admin ? "返回管理工作区" : "返回常用充电桩"}
    >
      <div className="wb-standard-page wb-account-page">
        <div className="wb-profile-line">
          <span className="wb-avatar wb-avatar-large">
            {currentUser?.username.slice(0, 1).toUpperCase()}
          </span>
          <div className="wb-profile-identity">
            <div className="wb-profile-title-row">
              <h2>{currentUser?.username}</h2>
              <StatusPill tone="idle">
                <ShieldCheckIcon size={12} />
                独立账户
              </StatusPill>
            </div>
            <p>
              {admin ? "管理员" : "个人账户"} ·{" "}
              {currentUser?.enabled ? "账户已启用" : "账户状态需确认"}
            </p>
          </div>
          <AccountLogout />
        </div>
        <nav
          className="wb-seg-rail wb-account-seg"
          aria-label="账户设置分类"
        >
          {tabs.map(([value, label]) => (
            <button
              key={value}
              type="button"
              className={`wb-seg-btn ${tab === value ? "wb-seg-active" : ""}`}
              aria-current={tab === value ? "page" : undefined}
              aria-pressed={tab === value}
              onClick={() => updatePageQuery({ tab: value, connect: null })}
            >
              {label}
            </button>
          ))}
        </nav>
        {tab === "connection" ? (
          <PlatformConnection />
        ) : tab === "notifications" ? (
          <NotificationPreferences />
        ) : tab === "security" ? (
          <fieldset
            disabled={!online}
            className="wb-account-security wb-continuous"
          >
            <AccountSecurityPanel />
          </fieldset>
        ) : (
          <div className="wb-settings-content">
            <AppearanceSettings />
          </div>
        )}
      </div>
    </Shell>
  )
}

function PlatformConnection() {
  const params = useSearchParams(),
    online = useOnline()
  const {
    snapshot,
    updateCookie,
    testCookie,
    joinPublicPool,
    leavePublicPool,
    clearCookie,
  } = useDashboard()
  const quota = snapshot.quota
  const connectParam = params.get("connect") === "1"
  const [cookieOpen, setCookieOpen] = useState(false)
  const dialogOpen = cookieOpen || connectParam
  function closeCookieDialog() {
    setCookieOpen(false)
    if (connectParam) updatePageQuery({ connect: null }, true)
  }
  const [clearOpen, setClearOpen] = useState(false)
  const [cookie, setCookie] = useState("")
  const [joinPoolChecked, setJoinPoolChecked] = useState(true)
  const [pending, setPending] = useState(false)
  const [testing, setTesting] = useState(false)
  const [poolBusy, setPoolBusy] = useState(false)
  const [formError, setFormError] = useState("")

  async function togglePoolMembership(join: boolean) {
    setPoolBusy(true)
    try {
      if (join) {
        await joinPublicPool()
        notify.success("已加入互助公共池，不限次刷新与空位提醒已解锁")
      } else {
        await leavePublicPool()
        notify.success("已退出互助公共池，当前按免费用户额度计费")
      }
    } catch (reason) {
      notify.error(reason, {
        title: join ? "加入互助公共池失败" : "退出互助公共池失败",
      })
    } finally {
      setPoolBusy(false)
    }
  }

  async function runCookieTest() {
    setTesting(true)
    try {
      const res = await testCookie()
      if (res.valid) {
        notify.success(
          unlimitedActive || !requirePool
            ? "当前个人 Cookie 有效，可正常不限次刷新"
            : "当前个人 Cookie 有效；加入互助公共池后即可解锁不限次刷新"
        )
      } else {
        notify.warning("当前个人 Cookie 已失效，请尽快更新", {
          description: res.error || "更新前将自动平滑降级使用公共池。",
        })
      }
    } catch (reason) {
      notify.error(reason, { title: "测试个人 Cookie 失败" })
    } finally {
      setTesting(false)
    }
  }

  const hasPersonal = Boolean(quota?.hasPersonalCookie)
  const personalStatus = quota?.personalCookieStatus ?? "none"
  const joinedPool = Boolean(quota?.joinedPublicPool)
  const requirePool = Boolean(quota?.requirePoolShareForUnlimited)
  const unlimitedActive = Boolean(quota?.unlimitedActive)

  async function saveCookie(event: React.FormEvent) {
    event.preventDefault()
    if (!cookie.trim()) {
      setFormError("请粘贴有效的 Cookie、请求头或 Copy as cURL。")
      return
    }
    setPending(true)
    setFormError("")
    try {
      await updateCookie(cookie.trim(), joinPoolChecked)
      setCookie("")
      setCookieOpen(false)
      updatePageQuery({ connect: null }, true)
      notify.success(
        joinPoolChecked
          ? "个人 Cookie 已加入互助公共池，已解锁不限次刷新与空位提醒"
          : requirePool
            ? "个人 Cookie 已保存（未加入互助公共池，暂按免费额度计费）"
            : "个人 Cookie 已保存，已解锁不限次刷新与空位提醒"
      )
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "未能更新，请检查内容后重试。"
      )
    } finally {
      setPending(false)
    }
  }

  async function removePersonalCookie() {
    setPending(true)
    setFormError("")
    try {
      await clearCookie()
      setClearOpen(false)
      notify.success("已清除个人 Cookie，当前切回公共池免费模式")
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : "清除失败。")
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="wb-settings-content">
      <SectionHeading
        title="个人 Cookie 与刷新额度"
        description="提供个人 Cookie 可随时不限次刷新并开启空位提醒；未提供时使用公共池，享每日免费额度，录入全站新桩可永久增加奖励次数。"
      />
      <div className="wb-connection-line">
        <span className="wb-service-icon">
          <KeyRoundIcon size={24} />
        </span>
        <div>
          <h3>
            查询通道{" "}
            <StatusPill
              tone={
                unlimitedActive
                  ? "idle"
                  : personalStatus === "expired"
                    ? "warning"
                    : "neutral"
              }
            >
              {unlimitedActive
                ? joinedPool
                  ? "互助 Cookie · 不限次"
                  : "个人 Cookie · 不限次"
                : personalStatus === "expired"
                  ? "个人 Cookie 已失效"
                  : hasPersonal && requirePool
                    ? "未加入互助池 · 免费模式"
                    : "公共池免费模式"}
            </StatusPill>
          </h3>
          <p>
            {unlimitedActive
              ? joinedPool
                ? "当前不限次刷新且支持空位提醒；感谢你参与全站互助公共池。"
                : "当前已使用个人 Cookie，支持不限次刷新与空位提醒。"
              : personalStatus === "expired"
                ? "个人 Cookie 已过期，刷新时会自动平滑降级使用公共池额度；请及时更新。"
                : hasPersonal && requirePool
                  ? "你已有个人 Cookie，但尚未加入互助公共池；加入后立刻解锁不限次刷新与空位提醒。"
                  : quota
                    ? `当前使用公共池查询，贡献全站新桩可额外获赠 +${quota.pileContributionBonusQuota} 次永久额度。`
                    : "正在读取刷新额度…"}
          </p>
        </div>
      </div>

      {quota && (
        <div className="wb-quota-ribbon" aria-label="刷新额度明细">
          <div>
            <span>每日额度</span>
            <strong className="wb-mono">
              {quota.freeDailyRemaining} / {quota.freeDailyLimit} 次
              {quota.freeDailyUsed > 0 ? `（今日已用 ${quota.freeDailyUsed} 次）` : ""}
            </strong>
          </div>
          <div>
            <span>永久额度（跨天不清零）</span>
            <strong className="wb-mono">
              {quota.bonusRemaining} 次（已贡献 {quota.contributedPileCount} 台新桩）
            </strong>
          </div>
          <div>
            <span>每日额度更新时间</span>
            <strong className="wb-mono">
              {formatQuotaResetTime(quota.dailyResetAt)}
            </strong>
          </div>
          <div>
            <span>扣减规则</span>
            <strong>
              优先扣每日，用完扣永久 · {quota.sharedPileCacheTTLSeconds ?? 60}s 内全站缓存免扣
            </strong>
          </div>
        </div>
      )}

      <div className="wb-grouped-list">
        {hasPersonal && personalStatus === "valid" && requirePool && !joinedPool && (
          <div className="wb-setting-row">
            <div>
              <h3>加入互助公共池（解锁不限次与空位提醒）</h3>
              <p>
                你的 Cookie 仅用于全站查询充电桩空闲状态（不涉及支付/启动充电），加入后立刻升级为不限次刷新用户。
              </p>
            </div>
            <WorkbenchButton
              busy={poolBusy}
              disabled={!online}
              onClick={() => void togglePoolMembership(true)}
            >
              立即加入互助池
            </WorkbenchButton>
          </div>
        )}

        {joinedPool && (
          <div className="wb-setting-row">
            <div>
              <h3>互助公共池共享中</h3>
              <p>随时可退出互助池；退出后账户将切回免费用户额度模式。</p>
            </div>
            <WorkbenchButton
              variant="ghost"
              busy={poolBusy}
              disabled={!online}
              onClick={() => void togglePoolMembership(false)}
            >
              退出互助池
            </WorkbenchButton>
          </div>
        )}
        <div className="wb-setting-row">
          <div>
            <h3>{hasPersonal ? "更新个人 Cookie" : "上传个人 Cookie"}</h3>
            <p>
              支持直接粘贴抓包工具复制的 <code>Copy as cURL</code>、完整请求头或纯 Cookie 字符串，系统会自动提取。
            </p>
          </div>
          <WorkbenchButton
            variant="ghost"
            disabled={!online}
            onClick={() => {
              setFormError("")
              setCookieOpen(true)
            }}
          >
            {hasPersonal ? "更新 Cookie" : "上传 Cookie"}
            <ArrowRightIcon size={14} />
          </WorkbenchButton>
        </div>

        {quota?.cookieGuideUrl && (
          <div className="wb-setting-row">
            <div>
              <h3>如何抓取个人 Cookie（视频教程）</h3>
              <p>只需抓一次包粘贴即可长期使用，完全不走微信协议、零风控。</p>
            </div>
            <a
              href={quota.cookieGuideUrl}
              target="_blank"
              rel="noreferrer"
              className="wb-button"
            >
              观看教程
              <ExternalLinkIcon size={14} />
            </a>
          </div>
        )}

        {hasPersonal && (
          <div className="wb-setting-row">
            <div>
              <h3>测试当前 Cookie 有效性</h3>
              <p>实测当前保存的个人 Cookie 是否仍可正常查询充电桩状态。</p>
            </div>
            <WorkbenchButton
              variant="outline"
              busy={testing}
              disabled={!online}
              onClick={() => void runCookieTest()}
            >
              测试有效性
            </WorkbenchButton>
          </div>
        )}

        {hasPersonal && (
          <div className="wb-setting-row">
            <div>
              <h3>清除个人 Cookie</h3>
              <p>移除当前保存的个人 Cookie 并切回公共池免费额度模式。</p>
            </div>
            <WorkbenchButton
              variant="ghost"
              disabled={!online}
              onClick={() => {
                setFormError("")
                setClearOpen(true)
              }}
            >
              清除 Cookie
            </WorkbenchButton>
          </div>
        )}
      </div>

      <div className="wb-settings-note">
        <ShieldCheckIcon size={17} />
        <p>
          个人 Cookie 均加密保存在服务端，不会在任何页面回显明文；如你选择加入互助公共池，也仅用于全站只读查询充电桩空闲状态，随时可一键退出。
        </p>
      </div>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          if (!pending) {
            if (open) setCookieOpen(true)
            else closeCookieDialog()
          }
        }}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>
              {hasPersonal ? "更新个人 Cookie" : "上传个人 Cookie"}
            </DialogTitle>
            <DialogDescription>
              直接粘贴 Copy as cURL、HTTP 请求头或 Cookie 字符串，系统会自动提取有效字段并验证。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={saveCookie}>
            <Field>
              <FieldLabel htmlFor="connection-cookie">
                Cookie / cURL 内容
              </FieldLabel>
              <textarea
                id="connection-cookie"
                className="wb-input wb-textarea"
                autoComplete="off"
                value={cookie}
                onChange={(event) => setCookie(event.target.value)}
                placeholder="粘贴 curl 'https://ele.mocele.com/...' 或 wxopenid=...; info=..."
              />
            </Field>
            <label className="mt-3 flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                className="mt-1"
                checked={joinPoolChecked}
                onChange={(event) => setJoinPoolChecked(event.target.checked)}
              />
              <span>
                同意将该 Cookie 加入全站互助公共池（仅用于查询空闲状态，不涉及支付或充电），立即解锁不限次刷新与空位提醒
              </span>
            </label>
            {quota?.cookieGuideUrl && (
              <p className="wb-field-hint">
                不知道怎么获取？
                <a
                  href={quota.cookieGuideUrl}
                  target="_blank"
                  rel="noreferrer"
                >
                  点此查看抓包视频教程
                </a>
              </p>
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
                disabled={pending}
                onClick={closeCookieDialog}
              >
                取消
              </WorkbenchButton>
              <WorkbenchButton type="submit" busy={pending} disabled={!online}>
                保存并验证
              </WorkbenchButton>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={clearOpen}
        onOpenChange={(open) => !pending && setClearOpen(open)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>清除个人 Cookie？</DialogTitle>
            <DialogDescription>
              清除后将切换为公共池免费额度模式，且空位提醒会暂停，直到重新上传个人 Cookie。
            </DialogDescription>
          </DialogHeader>
          {formError && (
            <p role="alert" className="wb-form-error">
              {formError}
            </p>
          )}
          <DialogFooter>
            <WorkbenchButton
              variant="outline"
              disabled={pending}
              onClick={() => setClearOpen(false)}
            >
              取消
            </WorkbenchButton>
            <WorkbenchButton
              variant="destructive"
              busy={pending}
              disabled={!online}
              onClick={() => void removePersonalCookie()}
            >
              确认清除
            </WorkbenchButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
function NotificationPreferences() {
  const { preference, loading, error, load, updatePreference } = useWatch(),
    online = useOnline()
  return (
    <fieldset
      disabled={!online}
      className="wb-settings-content wb-notification-preferences wb-continuous"
    >
      <SectionHeading
        title="接收方式"
        description="站内通知始终保留；浏览器和微信是可选的投递渠道。"
      />
      <div className="wb-setting-row">
        <div>
          <h3>站内通知</h3>
          <p>空闲提醒与连接问题的主记录，始终可以回看。</p>
        </div>
        <StatusPill tone="idle">
          <CheckIcon size={12} />
          始终开启
        </StatusPill>
      </div>
      <BrowserNotificationSetting />
      <WxPusherChannelCard active />
      {loading && !preference ? (
        <WorkbenchLoading label="正在读取偏好" />
      ) : error && !preference ? (
        <WorkbenchError
          message={error}
          retry={() => void load().catch(() => undefined)}
        />
      ) : (
        preference && (
          <section className="wb-quiet-settings">
            <SectionHeading
              title="免打扰"
              description="按设置的时区生效，暂停外部提醒但保留站内记录。"
            />
            <QuietHoursForm
              key={`${preference.quietHoursEnabled}:${preference.quietStartMinute}:${preference.quietEndMinute}`}
              preference={preference}
              onSave={updatePreference}
            />
          </section>
        )
      )}
      <Link className="wb-text-link" href="/dashboard/notifications">
        查看站内通知
        <ArrowRightIcon size={14} />
      </Link>
    </fieldset>
  )
}

function AccountLogout() {
  const { logout } = useAuth()
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  async function confirm() {
    setPending(true)
    try {
      await logout()
      router.replace("/login")
    } catch (reason) {
      notify.error(reason, { title: "退出登录失败" })
    } finally {
      setPending(false)
    }
  }
  return (
    <div className="wb-account-logout">
      <WorkbenchButton variant="outline" onClick={() => setOpen(true)}>
        <LogOutIcon size={15} />
        退出登录
      </WorkbenchButton>
      <Dialog open={open} onOpenChange={(value) => !pending && setOpen(value)}>
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>退出当前账户？</DialogTitle>
            <DialogDescription>
              常用桩和设置会保留，下次需要重新登录。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <WorkbenchButton
              variant="outline"
              disabled={pending}
              onClick={() => setOpen(false)}
            >
              取消
            </WorkbenchButton>
            <WorkbenchButton busy={pending} onClick={() => void confirm()}>
              退出登录
            </WorkbenchButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
