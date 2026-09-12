"use client"

import { useState } from "react"
import {
  Activity,
  ArrowRight,
  Bell,
  BookOpen,
  Check,
  ChevronRight,
  CircleAlert,
  ClipboardList,
  Command,
  Inbox,
  KeyRound,
  Menu,
  Moon,
  PlugZap,
  Search,
  Settings2,
  ShieldAlert,
  SlidersHorizontal,
  Sun,
  Users,
  WifiOff,
  X,
  type LucideIcon,
} from "lucide-react"
import { PrototypeProvider, usePrototype } from "./context"
import { adminViews, viewLabel, type View } from "./model"
import { AccountPage, AuthPage } from "./account"
import { AdminPage } from "./admin"
import { NotificationsPage, RemindersPage } from "./activity"
import { PrototypeDialogs } from "./dialogs"
import { PilesPage } from "./piles"
import { Brand, EmptyState, IconButton, PButton } from "./ui"
import "./prototype.css"

type NavItem = { view: View; label: string; icon: LucideIcon; count?: number }

export function ChargePrototype() {
  return (
    <PrototypeProvider>
      <PrototypeApp />
    </PrototypeProvider>
  )
}

function PrototypeApp() {
  const {
    data,
    route,
    go,
    openModal,
    theme,
    mutate,
    scenario,
    setScenario,
    offline,
    toast,
    dismissToast,
  } = usePrototype()
  const [menu, setMenu] = useState(false)
  const isAdmin = data.demoRole === "admin"
  const auth =
    data.demoRole === "guest" ||
    route.view === "login" ||
    route.view === "register"
  const unread = data.notices.filter((n) => !n.read).length
  const active = data.watches.filter((w) => w.status === "active").length
  const pendingIssues = data.incidents.filter(
    (i) => i.status !== "resolved"
  ).length
  const primary: NavItem[] = isAdmin
    ? [
        { view: "admin", label: "运行概览", icon: Activity },
        {
          view: "incidents",
          label: "异常处理",
          icon: ShieldAlert,
          count: pendingIssues,
        },
        { view: "users", label: "用户", icon: Users },
      ]
    : [
        { view: "piles", label: "常用充电桩", icon: PlugZap },
        { view: "reminders", label: "空闲提醒", icon: Bell, count: active },
        { view: "notifications", label: "通知", icon: Inbox, count: unread },
      ]
  const secondary: NavItem[] = isAdmin
    ? [
        { view: "operations", label: "系统运行", icon: Activity },
        { view: "policies", label: "系统策略", icon: SlidersHorizontal },
        { view: "invites", label: "邀请注册", icon: KeyRound },
        { view: "audit", label: "操作记录", icon: ClipboardList },
      ]
    : [{ view: "account", label: "账户与设置", icon: Settings2 }]
  const selected =
    route.view === "piles" && route.id
      ? data.piles.find((p) => p.id === route.id)
      : undefined
  const unauthorized = isAdmin
    ? ["piles", "reminders", "notifications"].includes(route.view)
    : adminViews.has(route.view)
  function navigate(view: View) {
    setMenu(false)
    go({ view })
  }
  const nav = (items: NavItem[]) =>
    items.map(({ view, label, icon: Icon, count }) => (
      <button
        key={view}
        className={`cp-nav-item ${route.view === view ? "cp-nav-active" : ""}`}
        aria-label={count ? `${label} ${count}` : label}
        aria-current={route.view === view ? "page" : undefined}
        onClick={() => navigate(view)}
      >
        <Icon size={18} strokeWidth={1.8} />
        <span>{label}</span>
        {!!count && <span className="cp-nav-count">{count}</span>}
      </button>
    ))
  return (
    <div
      className="cp-theme cp-root"
      data-theme={theme}
      data-reduced={data.reduceMotion || undefined}
    >
      <a
        className="cp-skip-link"
        href="#prototype-main"
        onClick={(event) => {
          event.preventDefault()
          document.getElementById("prototype-main")?.focus()
        }}
      >
        跳到主要内容
      </a>
      {auth ? (
        <AuthPage key={route.view} />
      ) : (
        <div className="cp-app-shell">
          <aside className="cp-sidebar">
            <button
              className="cp-home"
              aria-label="返回工作区首页"
              onClick={() => navigate(isAdmin ? "admin" : "piles")}
            >
              <Brand />
            </button>
            <div className="cp-workspace-label">
              <span className="cp-workspace-dot" />
              {isAdmin ? "管理工作区" : "我的充电空间"}
            </div>
            <nav aria-label="主导航">
              <div className="cp-nav-group-label">
                {isAdmin ? "工作区" : "日常使用"}
              </div>
              {nav(primary)}
              <div className="cp-nav-group-label cp-nav-second">
                {isAdmin ? "系统管理" : "偏好与连接"}
              </div>
              {nav(secondary)}
            </nav>
            <div className="cp-sidebar-spacer" />
            <div className="cp-sidebar-help">
              <button onClick={() => openModal({ kind: "guide" })}>
                <BookOpen size={16} />
                使用说明
                <ArrowRight size={13} />
              </button>
              {!isAdmin && (
                <div className="cp-connection-caption">
                  <i
                    className={
                      data.credential === "healthy" && scenario !== "expired"
                        ? "cp-legend-idle"
                        : "cp-legend-warning"
                    }
                  />
                  {offline
                    ? "网络离线"
                    : data.credential === "healthy" && scenario !== "expired"
                      ? "平台已连接"
                      : "平台连接需处理"}
                </div>
              )}
            </div>
            <button
              className="cp-sidebar-user"
              aria-label="查看账户与设置"
              onClick={() =>
                go({
                  view: "account",
                  tab: isAdmin ? "security" : "connection",
                })
              }
            >
              <span className="cp-avatar">
                {isAdmin ? "A" : data.username.slice(0, 1)}
              </span>
              <span>
                <strong>{isAdmin ? "admin" : data.username}</strong>
                <small>{isAdmin ? "管理员" : "个人账户"}</small>
              </span>
              <ChevronRight size={15} />
            </button>
          </aside>
          <div className="cp-app-main">
            <header className="cp-topbar">
              <div className="cp-mobile-header">
                <IconButton
                  icon={menu ? X : Menu}
                  label={menu ? "关闭导航" : "打开导航"}
                  aria-expanded={menu}
                  onClick={() => setMenu(!menu)}
                />
                <Brand small />
              </div>
              <div className="cp-breadcrumb">
                <span>{isAdmin ? "管理工作区" : "我的空间"}</span>
                <ChevronRight size={12} />
                <strong>{viewLabel[route.view]}</strong>
                {selected && (
                  <>
                    <ChevronRight size={12} />
                    <span>{selected.number}</span>
                  </>
                )}
              </div>
              <div className="cp-topbar-actions">
                <button
                  className="cp-global-search"
                  onClick={() => openModal({ kind: "search" })}
                  aria-label="打开快速搜索"
                >
                  <Search size={15} />
                  <span>{isAdmin ? "搜索用户或页面" : "搜索充电桩或页面"}</span>
                  <kbd>
                    <Command size={10} />K
                  </kbd>
                </button>
                <span className="cp-topbar-divider" />
                <IconButton
                  icon={theme === "light" ? Moon : Sun}
                  label={theme === "light" ? "切换到深色" : "切换到浅色"}
                  onClick={() =>
                    mutate((d) => {
                      d.theme = theme === "light" ? "dark" : "light"
                    })
                  }
                />
                <button
                  className="cp-preview-button"
                  onClick={() => openModal({ kind: "preview" })}
                >
                  <span className="cp-preview-dot" />
                  示例数据
                  <span className="cp-preview-button-label">· 预览设置</span>
                </button>
              </div>
            </header>
            {menu && (
              <nav className="cp-mobile-menu" aria-label="移动导航">
                {nav(primary)}
                {nav(secondary)}
                {isAdmin && (
                  <button
                    className="cp-nav-item"
                    onClick={() => navigate("account")}
                  >
                    <Settings2 size={18} />
                    账户安全
                  </button>
                )}
                <PButton
                  tone="quiet"
                  onClick={() => {
                    setMenu(false)
                    openModal({ kind: "guide" })
                  }}
                >
                  使用说明
                </PButton>
              </nav>
            )}
            <main id="prototype-main" className="cp-main-content" tabIndex={-1}>
              {offline ? (
                <div
                  className="cp-state-banner cp-banner-offline"
                  role="status"
                >
                  <WifiOff size={17} />
                  <div>
                    <strong>当前离线</strong>
                    <span>
                      保留最近快照，状态可能已变化。联网后可以继续操作。
                    </span>
                  </div>
                  <PButton
                    tone="quiet"
                    onClick={() => {
                      if (navigator.onLine) setScenario("ready")
                    }}
                  >
                    重新连接
                    <RefreshIcon />
                  </PButton>
                </div>
              ) : scenario === "expired" ||
                (!isAdmin && data.credential === "expired") ? (
                <div
                  className="cp-state-banner cp-banner-warning"
                  role="status"
                >
                  <KeyRound size={17} />
                  <div>
                    <strong>充电平台登录已过期</strong>
                    <span>快照仍然保留，重新扫码后恢复刷新和提醒。</span>
                  </div>
                  <PButton onClick={() => openModal({ kind: "scan" })}>
                    重新连接
                    <ArrowRight size={14} />
                  </PButton>
                </div>
              ) : scenario === "power-off" ? (
                <div className="cp-state-banner" role="status">
                  <Moon size={17} />
                  <div>
                    <strong>计划断电时段</strong>
                    <span>23:00–06:00 暂停远端检查，恢复供电后继续。</span>
                  </div>
                </div>
              ) : scenario === "quota" ? (
                <div
                  className="cp-state-banner cp-banner-warning"
                  role="status"
                >
                  <CircleAlert size={17} />
                  <div>
                    <strong>今日检查额度已用完</strong>
                    <span>可以查看已有数据；明天恢复远端检查。</span>
                  </div>
                </div>
              ) : null}
              {scenario !== "ready" &&
                !["offline", "expired", "power-off", "quota"].includes(
                  scenario
                ) && (
                  <div className="cp-preview-state">
                    <span>
                      正在预览：
                      {scenario === "loading"
                        ? "加载状态"
                        : scenario === "empty"
                          ? "空状态"
                          : "错误状态"}
                    </span>
                    <button onClick={() => setScenario("ready")}>
                      恢复正常数据
                      <X size={12} />
                    </button>
                  </div>
                )}
              {unauthorized ? (
                <EmptyState
                  icon={ShieldAlert}
                  title="这个页面属于另一类账户"
                  description={
                    isAdmin
                      ? "管理员通过用户页面检查设备，不进入普通用户的个人空间。"
                      : "管理员功能不在普通账户的访问范围内。"
                  }
                  action={
                    <PButton
                      tone="brand"
                      onClick={() => go({ view: isAdmin ? "admin" : "piles" })}
                    >
                      返回我的工作区
                      <ArrowRight size={14} />
                    </PButton>
                  }
                />
              ) : route.view === "piles" ? (
                <PilesPage />
              ) : route.view === "reminders" ? (
                <RemindersPage />
              ) : route.view === "notifications" ? (
                <NotificationsPage />
              ) : route.view === "account" ? (
                <AccountPage />
              ) : (
                <AdminPage />
              )}
            </main>
            <footer className="cp-app-footer">
              <span>
                Charge Console<span>让等待少一点。</span>
              </span>
              <button onClick={() => openModal({ kind: "preview" })}>
                交互原型 · 所有更改仅在本地生效
                <SlidersHorizontal size={12} />
              </button>
            </footer>
          </div>
          <nav className="cp-mobile-bottom" aria-label="常用页面">
            {[
              ...primary,
              { view: "account" as View, label: "我的", icon: Settings2 },
            ].map(({ view, label, icon: Icon, count }) => (
              <button
                key={view}
                aria-current={route.view === view ? "page" : undefined}
                onClick={() => navigate(view)}
              >
                <span>
                  <Icon size={20} />
                  {!!count && <i>{count}</i>}
                </span>
                <span>{label}</span>
              </button>
            ))}
          </nav>
        </div>
      )}
      <PrototypeDialogs />
      {toast && (
        <div
          className={`cp-toast cp-toast-${toast.tone}`}
          role={toast.tone === "error" ? "alert" : "status"}
        >
          {toast.tone === "error" ? (
            <CircleAlert size={17} />
          ) : (
            <Check size={17} />
          )}
          <span>{toast.message}</span>
          <button aria-label="关闭提示" onClick={dismissToast}>
            <X size={14} />
          </button>
        </div>
      )}
    </div>
  )
}

function RefreshIcon() {
  return <ArrowRight size={14} />
}
