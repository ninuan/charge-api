"use client"

import {
  ArrowRightIcon,
  CheckIcon,
  Link2Icon,
  LogOutIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
} from "lucide-react"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useCallback, useEffect, useState } from "react"
import { AppShell } from "@/components/app-shell"
import { AccountSecurityPanel } from "@/components/account-security-panel"
import { AppearanceSettings } from "@/components/appearance-settings"
import { YybLoginDialog } from "@/components/yyb-login-dialog"
import { BrowserNotificationSetting } from "@/components/notification-center"
import { WxPusherChannelCard } from "@/components/wxpusher-channel-card"
import { QuietHoursForm } from "@/components/watch-management-sheet"
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
import { requestJSON, requestEmpty } from "@/lib/http"
import { notify } from "@/lib/feedback"
import { formatSnapshotTime, updatePageQuery } from "@/lib/workbench"
import { DashboardSession } from "./dashboard-session"
import { UserWorkbenchShell } from "./user-shell"
import {
  SectionHeading,
  StatusPill,
  WorkbenchButton,
  WorkbenchError,
  WorkbenchLoading,
} from "./surfaces"

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
    >
      <div className="wb-standard-page wb-account-page">
        <div className="wb-profile-line">
          <span className="wb-avatar wb-avatar-large">
            {currentUser?.username.slice(0, 1).toUpperCase()}
          </span>
          <div>
            <h2>{currentUser?.username}</h2>
            <p>
              {admin ? "管理员" : "个人账户"} ·{" "}
              {currentUser?.enabled ? "账户已启用" : "账户状态需确认"}
            </p>
          </div>
          <StatusPill>
            <ShieldCheckIcon size={12} />
            独立账户
          </StatusPill>
        </div>
        <AccountLogout />
        <nav
          className="wb-section-tabs wb-account-tabs"
          aria-label="账户设置分类"
        >
          {tabs.map(([value, label]) => (
            <button
              key={value}
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

type BindingState = {
  bound: boolean
  nickname?: string
  openidSuffix?: string
  status?: string
  boundAt?: string
  lastCheckedAt?: string
}
function PlatformConnection() {
  const params = useSearchParams(),
    online = useOnline()
  const { snapshot, fetchSnapshot, updateCookie } = useDashboard()
  const [binding, setBinding] = useState<BindingState | null>(null)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  const [scanOpen, setScanOpen] = useState(params.get("connect") === "1")
  const [cookieOpen, setCookieOpen] = useState(false)
  const [unlinkOpen, setUnlinkOpen] = useState(false)
  const [cookie, setCookie] = useState("")
  const [pending, setPending] = useState(false)
  const [formError, setFormError] = useState("")
  const load = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      setBinding(
        await requestJSON<BindingState>(
          "/api/session/yyb-binding",
          {},
          "暂时无法读取平台绑定状态。"
        )
      )
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "暂时无法读取平台绑定状态。"
      )
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) void load()
    })
    return () => {
      active = false
    }
  }, [load])
  async function sync() {
    const deviceId = snapshot.piles[0]?.id
    if (!deviceId) return
    setPending(true)
    try {
      await requestJSON(
        "/api/session/mocele-cookie",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ deviceId }),
          timeoutMs: 120_000,
        },
        "同步访问凭据失败。"
      )
      await fetchSnapshot()
      await load()
      notify.success("平台凭据已同步")
    } catch (reason) {
      notify.error(reason, { title: "同步未完成" })
    } finally {
      setPending(false)
    }
  }
  async function saveCookie(event: React.FormEvent) {
    event.preventDefault()
    if (!cookie.trim()) {
      setFormError("请填写有效的 Cookie。")
      return
    }
    setPending(true)
    setFormError("")
    try {
      await updateCookie(cookie.trim())
      setCookie("")
      setCookieOpen(false)
      notify.success("访问凭据已更新")
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "未能更新，请重试。"
      )
    } finally {
      setPending(false)
    }
  }
  async function unlink() {
    setPending(true)
    setFormError("")
    try {
      await requestEmpty(
        "/api/session/yyb-binding",
        { method: "DELETE" },
        "解除平台绑定失败。"
      )
      setUnlinkOpen(false)
      await load()
      notify.success("平台绑定已解除")
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : "解除绑定失败。")
    } finally {
      setPending(false)
    }
  }
  return (
    <div className="wb-settings-content">
      <SectionHeading
        title="充电平台连接"
        description="绑定微信用于获取访问凭据；设备是否可用，以实际读取结果为准。"
      />
      {loading && !binding ? (
        <WorkbenchLoading label="正在检查平台绑定" />
      ) : (
        <>
          <div className="wb-connection-line">
            <span className="wb-service-icon">
              <Link2Icon size={26} />
            </span>
            <div>
              <h3>
                充电平台{" "}
                <StatusPill tone={binding?.bound ? "idle" : "neutral"}>
                  {error
                    ? "暂时无法检查"
                    : binding?.bound
                      ? "微信已绑定"
                      : "尚未绑定"}
                </StatusPill>
              </h3>
              <p>
                {binding?.bound
                  ? `${binding.nickname || "微信账户"}${binding.openidSuffix ? ` · 尾号 ${binding.openidSuffix}` : ""} · 最近检查 ${formatSnapshotTime(binding.lastCheckedAt, true)}`
                  : "完成扫码确认后，才能通过绑定账户恢复登录。"}
              </p>
            </div>
            <YybLoginDialog
              open={scanOpen}
              onOpenChange={(open) => {
                setScanOpen(open)
                if (!open) {
                  updatePageQuery({ connect: null }, true)
                  void load()
                }
              }}
            />
          </div>
          {error && (
            <div role="alert" className="wb-state-banner wb-banner-warning">
              <div>
                <strong>绑定状态读取失败</strong>
                <span>{error}</span>
              </div>
              <WorkbenchButton variant="outline" onClick={() => void load()}>
                重试
              </WorkbenchButton>
            </div>
          )}
        </>
      )}
      <div className="wb-setting-row">
        <div>
          <h3>自动恢复登录</h3>
          <p>已绑定微信时，凭据过期会尝试恢复；仍未成功时再提醒你。</p>
        </div>
        <span className="wb-small wb-muted">
          {binding?.bound ? "已具备恢复条件" : "绑定后可用"}
        </span>
      </div>
      <div className="wb-setting-row">
        <div>
          <h3>手动同步凭据</h3>
          <p>
            {snapshot.piles.length
              ? "使用当前已绑定账户，重新获取访问凭据。"
              : "添加一台充电桩后才能同步设备访问凭据。"}
          </p>
        </div>
        <WorkbenchButton
          variant="outline"
          busy={pending}
          disabled={!online || !binding?.bound || !snapshot.piles.length}
          onClick={() => void sync()}
        >
          <RefreshCwIcon size={15} />
          同步一次
        </WorkbenchButton>
      </div>
      <div className="wb-setting-row">
        <div>
          <h3>手动更新 Cookie</h3>
          <p>未部署扫码服务时，也可以手动更新你有权使用的访问凭据。</p>
        </div>
        <WorkbenchButton
          variant="ghost"
          disabled={!online}
          onClick={() => {
            setFormError("")
            setCookieOpen(true)
          }}
        >
          手动更新
          <ArrowRightIcon size={14} />
        </WorkbenchButton>
      </div>
      {binding?.bound && (
        <div className="wb-setting-row">
          <div>
            <h3>解除平台绑定</h3>
            <p>停止使用当前扫码绑定，不移除常用桩与已有通知。</p>
          </div>
          <WorkbenchButton
            variant="ghost"
            disabled={!online}
            onClick={() => {
              setFormError("")
              setUnlinkOpen(true)
            }}
          >
            解除绑定
          </WorkbenchButton>
        </div>
      )}
      <div className="wb-settings-note">
        <ShieldCheckIcon size={17} />
        <p>访问凭据只提交到当前服务，不在页面上回显。请勿公开或分享 Cookie。</p>
      </div>
      <Dialog
        open={cookieOpen}
        onOpenChange={(open) => !pending && setCookieOpen(open)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>手动更新访问凭据</DialogTitle>
            <DialogDescription>
              只更新当前账户的 Cookie，不影响其他用户。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={saveCookie}>
            <Field>
              <FieldLabel htmlFor="connection-cookie">Cookie</FieldLabel>
              <textarea
                id="connection-cookie"
                className="wb-input wb-textarea"
                autoComplete="off"
                value={cookie}
                onChange={(event) => setCookie(event.target.value)}
                placeholder="粘贴你有权使用的 Cookie"
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
                disabled={pending}
                onClick={() => setCookieOpen(false)}
              >
                取消
              </WorkbenchButton>
              <WorkbenchButton type="submit" busy={pending} disabled={!online}>
                更新连接
              </WorkbenchButton>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog
        open={unlinkOpen}
        onOpenChange={(open) => !pending && setUnlinkOpen(open)}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>解除平台绑定？</DialogTitle>
            <DialogDescription>
              之后需要重新扫码，才能再次通过绑定账户恢复登录。
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
              onClick={() => setUnlinkOpen(false)}
            >
              取消
            </WorkbenchButton>
            <WorkbenchButton
              variant="destructive"
              busy={pending}
              disabled={!online}
              onClick={() => void unlink()}
            >
              确认解除
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
            <QuietHoursForm preference={preference} onSave={updatePreference} />
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
