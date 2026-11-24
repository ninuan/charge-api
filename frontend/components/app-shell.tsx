"use client"

import {
  ActivityIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  BellIcon,
  BellRingIcon,
  ChevronRightIcon,
  ClipboardListIcon,
  CommandIcon,
  InboxIcon,
  KeyRoundIcon,
  MegaphoneIcon,
  MenuIcon,
  PlugZapIcon,
  SearchIcon,
  Settings2Icon,
  ShieldAlertIcon,
  SlidersHorizontalIcon,
  UsersIcon,
  WifiOffIcon,
  type LucideIcon,
} from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { GlobalFeedbackBanner } from "@/components/global-feedback-banner"
import { ThemeToggle } from "@/components/theme-toggle"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Brand } from "@/components/workbench/brand"
import { PulseDot, WorkbenchButton } from "@/components/workbench/surfaces"
import { useAuth } from "@/lib/auth-context"
import { useOnline } from "@/lib/browser-state"
import { sessionExpiredEvent } from "@/lib/http"

const AppShellMenuContext = createContext<() => void>(() => {})
export function useCloseAppShellMenu() {
  return useContext(AppShellMenuContext)
}

export type WorkbenchSection =
  | "piles"
  | "pool"
  | "reminders"
  | "notifications"
  | "account"
  | "overview"
  | "incidents"
  | "users"
  | "operations"
  | "settings"
  | "invites"
  | "audit"
  | "announcements"

export type WorkbenchSearchItem = {
  title: string
  description: string
  href: string
}

type NavItem = {
  id: WorkbenchSection
  href: string
  label: string
  icon: LucideIcon
  count?: number
}

export function AppShell({
  title,
  description,
  actions,
  notificationAction,
  children,
  activeSection,
  counts = {},
  searchItems = [],
  guideAction,
  hidePageHeader = false,
  narrow = false,
  backHref,
  backLabel = "返回常用充电桩",
  connectionState,
}: {
  compact?: boolean
  title: string
  description: string
  actions?: ReactNode
  notificationAction?: ReactNode
  children: ReactNode
  activeSection?: WorkbenchSection
  counts?: { reminders?: number; notifications?: number; incidents?: number }
  searchItems?: WorkbenchSearchItem[]
  guideAction?: ReactNode
  hidePageHeader?: boolean
  narrow?: boolean
  backHref?: string
  backLabel?: string
  connectionState?: "connected" | "reconnecting" | "offline"
}) {
  const router = useRouter()
  const { currentUser, isAdmin, ready, clearSession } = useAuth()
  const online = useOnline()
  const [menuOpen, setMenuOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [searchIndex, setSearchIndex] = useState(0)
  const searchRef = useRef<HTMLInputElement>(null)
  const closeMenu = useCallback(() => setMenuOpen(false), [])

  useEffect(() => {
    const media = window.matchMedia?.("(min-width: 768px)")
    if (!media) return
    const handleChange = (event: MediaQueryListEvent) => {
      if (event.matches) setMenuOpen(false)
    }
    media.addEventListener("change", handleChange)
    return () => media.removeEventListener("change", handleChange)
  }, [])

  useEffect(() => {
    const expired = () => {
      clearSession()
      router.replace("/login")
    }
    window.addEventListener(sessionExpiredEvent, expired)
    return () => window.removeEventListener(sessionExpiredEvent, expired)
  }, [clearSession, router])

  const active =
    activeSection ??
    (
      {
        运营总览: "overview",
        运行概览: "overview",
        用户管理: "users",
        系统设置: "settings",
        运维与审计: "operations",
        账户中心: "account",
      } as Record<string, WorkbenchSection>
    )[title] ??
    (isAdmin ? "overview" : "piles")

  const primary: NavItem[] = isAdmin
    ? [
        {
          id: "overview",
          href: "/admin",
          label: "运行概览",
          icon: ActivityIcon,
        },
        {
          id: "incidents",
          href: "/admin?tab=incidents",
          label: "异常处理",
          icon: ShieldAlertIcon,
          count: counts.incidents,
        },
        {
          id: "users",
          href: "/admin?tab=users",
          label: "用户",
          icon: UsersIcon,
        },
        {
          id: "announcements",
          href: "/admin?tab=announcements",
          label: "公告管理",
          icon: MegaphoneIcon,
        },
      ]
    : [
        {
          id: "piles",
          href: "/dashboard",
          label: "常用充电桩",
          icon: PlugZapIcon,
        },
        {
          id: "reminders",
          href: "/dashboard/reminders",
          label: "空闲提醒",
          icon: BellIcon,
          count: counts.reminders,
        },
        {
          id: "notifications",
          href: "/dashboard/notifications",
          label: "通知",
          icon: InboxIcon,
          count: counts.notifications,
        },
      ]

  const secondary: NavItem[] = isAdmin
    ? [
        {
          id: "operations",
          href: "/admin?tab=operations",
          label: "系统运行",
          icon: ActivityIcon,
        },
        {
          id: "settings",
          href: "/admin?tab=settings",
          label: "系统策略",
          icon: SlidersHorizontalIcon,
        },
        {
          id: "invites",
          href: "/admin?tab=invites",
          label: "邀请注册",
          icon: KeyRoundIcon,
        },
        {
          id: "audit",
          href: "/admin?tab=audit",
          label: "操作记录",
          icon: ClipboardListIcon,
        },
      ]
    : [
        {
          id: "account",
          href: "/account",
          label: "账户与设置",
          icon: Settings2Icon,
        },
      ]

  const pages = [
    ...primary,
    ...secondary,
    {
      id: "account" as const,
      href: isAdmin ? "/account?tab=security" : "/account",
      label: "账户与设置",
      icon: Settings2Icon,
    },
  ]

  const candidates = [
    ...searchItems,
    ...pages
      .filter(
        (item, index, all) =>
          all.findIndex((candidate) => candidate.id === item.id) === index
      )
      .map((item) => ({
        title: item.label,
        description: "页面",
        href: item.href,
      })),
  ]

  const results = [
    ...candidates
      .filter((item) =>
        `${item.title} ${item.description}`
          .toLocaleLowerCase()
          .includes(query.trim().toLocaleLowerCase())
      )
      .slice(0, 11),
    ...(isAdmin && query.trim()
      ? [
          {
            title: `查找用户“${query.trim()}”`,
            description: "按用户名搜索全部账户",
            href: `/admin?tab=users&search=${encodeURIComponent(query.trim())}`,
          },
        ]
      : []),
  ]

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        setSearchOpen((value) => !value)
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [])

  const renderDrawerLinks = (items: NavItem[]) =>
    items.map(({ id, href, label, icon: Icon, count }) => (
      <Link
        key={id}
        href={href}
        className={`wb-nav-item ${active === id ? "wb-nav-active" : ""}`}
        aria-label={count ? `${label}，${count} 项` : label}
        aria-current={active === id ? "page" : undefined}
        onClick={closeMenu}
      >
        <Icon size={16} strokeWidth={1.75} />
        <span>{label}</span>
        {!!count && (
          <span className="wb-nav-count">{count > 99 ? "99+" : count}</span>
        )}
      </Link>
    ))

  const identity = !ready ? (
    <div
      data-testid="identity-skeleton"
      className="wb-identity-loading"
      aria-label="正在加载账户信息"
    >
      <Skeleton className="size-7 rounded-lg" />
    </div>
  ) : (
    <Link
      href={isAdmin ? "/account?tab=security" : "/account"}
      aria-label="进入账户中心"
      title={currentUser?.username || "账户中心"}
      className={`wb-avatar-link ${active === "account" ? "wb-avatar-link-active" : ""}`}
      onClick={closeMenu}
    >
      {currentUser?.username.slice(0, 1).toUpperCase() || "U"}
    </Link>
  )

  function choose(item: WorkbenchSearchItem) {
    setSearchOpen(false)
    setQuery("")
    router.push(item.href)
  }

  return (
    <AppShellMenuContext.Provider value={closeMenu}>
      <div className="wb-theme wb-root app-workbench">
        <a
          className="wb-skip-link"
          href="#main-content"
          onClick={(event) => {
            event.preventDefault()
            document.getElementById("main-content")?.focus()
          }}
        >
          跳到主要内容
        </a>

        <div className="wb-spatial-shell">
          {/* MINIMAL FLOATING 52px TOPBAR (Zero Heavy Sidebar) */}
          <header data-slot="app-header" className="wb-header">
            <div className="wb-header-inner">
              <div className="wb-header-left">
                <div className="wb-mobile-menu-trigger">
                  <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
                    <SheetTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon"
                          className="wb-header-icon-btn"
                          aria-label="打开菜单"
                        />
                      }
                    >
                      <MenuIcon size={17} strokeWidth={1.75} />
                    </SheetTrigger>
                    <SheetContent
                      side="left"
                      className="wb-theme workbench-drawer overflow-y-auto"
                    >
                      <div className="wb-drawer-header">
                        <SheetTitle>账户与操作</SheetTitle>
                      </div>
                      <nav aria-label="移动导航" className="wb-drawer-nav">
                        {renderDrawerLinks(primary)}
                        <div className="wb-nav-group-label wb-nav-second">
                          {isAdmin ? "系统管理" : "偏好与连接"}
                        </div>
                        {renderDrawerLinks(secondary)}
                      </nav>
                      <div className="wb-menu-account">
                        {ready && (
                          <p>
                            {currentUser?.username} ·{" "}
                            {isAdmin ? "管理员" : "普通用户"}
                          </p>
                        )}
                        <Link href="/account?tab=security" onClick={closeMenu}>
                          账户安全
                          <ChevronRightIcon size={14} />
                        </Link>
                      </div>
                    </SheetContent>
                  </Sheet>
                </div>

                <Link
                  className="wb-home"
                  href={isAdmin ? "/admin" : "/dashboard"}
                  aria-label="返回工作区首页"
                >
                  <Brand />
                </Link>

                {isAdmin ? (
                  <>
                    <span className="wb-space-chip">管理工作区</span>
                    <nav className="wb-top-nav" aria-label="主导航">
                      {[
                        {
                          href: "/admin",
                          label: "值守中心",
                          icon: ActivityIcon,
                          isCurrent:
                            active === "overview" || active === "incidents",
                          count: counts.incidents,
                        },
                        {
                          href: "/admin?tab=pool",
                          label: "资源与用户",
                          icon: UsersIcon,
                          isCurrent:
                            active === "pool" ||
                            active === "piles" ||
                            active === "users" ||
                            active === "announcements",
                        },
                        {
                          href: "/admin?tab=operations",
                          label: "系统与合规",
                          icon: SlidersHorizontalIcon,
                          isCurrent:
                            active === "operations" ||
                            active === "settings" ||
                            active === "invites" ||
                            active === "audit",
                        },
                      ].map(({ href, label, icon: Icon, isCurrent, count }) => (
                        <Link
                          key={href}
                          href={href}
                          className={`wb-top-nav-link ${
                            isCurrent ? "wb-top-nav-active" : ""
                          }`}
                          aria-label={count ? `${label}，${count} 项` : label}
                          aria-current={isCurrent ? "page" : undefined}
                        >
                          <Icon size={14} strokeWidth={1.75} aria-hidden="true" />
                          <span>{label}</span>
                          {!!count && (
                            <span className="wb-top-nav-badge">
                              {count > 99 ? "99+" : count}
                            </span>
                          )}
                        </Link>
                      ))}
                    </nav>
                  </>
                ) : (
                  <Link
                    href="/account?tab=connection"
                    className="wb-conn-chip"
                    title="查看平台连接状态"
                  >
                    <PulseDot
                      tone={
                        !online || connectionState === "offline"
                          ? "offline"
                          : connectionState === "reconnecting"
                            ? "warning"
                            : "idle"
                      }
                      pulse={online && connectionState !== "offline" && connectionState !== "reconnecting"}
                    />
                    <span className="wb-conn-chip-label">
                      {!online || connectionState === "offline"
                        ? "离线"
                        : connectionState === "reconnecting"
                          ? "重连中"
                          : "已连接"}
                    </span>
                  </Link>
                )}
              </div>

              <button
                type="button"
                className="wb-global-search"
                aria-label="打开快速搜索"
                onClick={() => setSearchOpen(true)}
              >
                <SearchIcon size={14} strokeWidth={1.75} aria-hidden="true" />
                <span>
                  {isAdmin ? "搜索用户、公告或设置" : "快速定位充电桩或操作"}
                </span>
                <kbd>
                  <CommandIcon size={10} />K
                </kbd>
              </button>

              <div className="wb-header-right">
                {!isAdmin && (
                  <nav className="wb-header-icon-nav" aria-label="主导航">
                    {counts.reminders ? (
                      <Link
                        href="/dashboard/reminders"
                        className={`wb-watch-pill ${
                          active === "reminders" ? "wb-watch-pill-active" : ""
                        }`}
                        aria-label={`空闲提醒，${counts.reminders} 项`}
                        aria-current={
                          active === "reminders" ? "page" : undefined
                        }
                      >
                        <BellRingIcon size={14} strokeWidth={1.85} />
                        <span>守候 {counts.reminders}</span>
                      </Link>
                    ) : (
                      <Link
                        href="/dashboard/reminders"
                        className={`wb-header-icon-link ${
                          active === "reminders"
                            ? "wb-header-icon-link-active"
                            : ""
                        }`}
                        aria-label="空闲提醒"
                        title="空闲提醒"
                        aria-current={
                          active === "reminders" ? "page" : undefined
                        }
                      >
                        <BellIcon size={16} strokeWidth={1.8} />
                      </Link>
                    )}

                    <Link
                      href="/dashboard/notifications"
                      className={`wb-header-icon-link ${
                        active === "notifications"
                          ? "wb-header-icon-link-active"
                          : ""
                      }`}
                      aria-label={
                        counts.notifications
                          ? `通知，${counts.notifications} 项`
                          : "通知"
                      }
                      title="消息与通知"
                      aria-current={
                        active === "notifications" ? "page" : undefined
                      }
                    >
                      <InboxIcon size={16} strokeWidth={1.8} />
                      {!!counts.notifications && (
                        <span className="wb-icon-badge">
                          {counts.notifications > 99
                            ? "99+"
                            : counts.notifications}
                        </span>
                      )}
                    </Link>
                  </nav>
                )}

                {notificationAction}
                {guideAction && (
                  <div className="wb-header-guide">{guideAction}</div>
                )}
                <ThemeToggle />
                {identity}
              </div>
            </div>
          </header>

          {/* CONTEXTUAL GLOBAL BANNERS */}
          <GlobalFeedbackBanner />
          {!online && (
            <div role="status" className="wb-global-strip wb-strip-offline">
              <div className="wb-global-strip-inner">
                <WifiOffIcon size={15} />
                <span>
                  <strong>当前离线</strong> —
                  保留最近读取的内容，联网后再刷新或保存。
                </span>
              </div>
            </div>
          )}
          {currentUser?.mustChangePassword && (
            <div role="status" className="wb-global-strip wb-strip-warning">
              <div className="wb-global-strip-inner">
                <KeyRoundIcon size={15} />
                <span>
                  <strong>当前使用的是管理员生成的临时密码</strong> —
                  请修改为只有你本人知道的新密码。
                </span>
                <WorkbenchButton
                  variant="outline"
                  onClick={() =>
                    router.push("/account?tab=security#change-password")
                  }
                >
                  修改密码
                </WorkbenchButton>
              </div>
            </div>
          )}

          {/* MAIN CENTERED WORKSPACE CANVAS */}
          <main
            id="main-content"
            tabIndex={-1}
            className={`wb-stage ${narrow ? "wb-stage-narrow" : ""}`}
          >
            {backHref && (
              <div className="wb-back-bar">
                <Link href={backHref} className="wb-stage-back-link">
                  <ArrowLeftIcon size={14} />
                  {backLabel}
                </Link>
              </div>
            )}
            {!hidePageHeader && (
              <header className="wb-page-heading">
                <div>
                  <h1>{title}</h1>
                  <p>{description}</p>
                </div>
                {actions && <div className="wb-heading-actions">{actions}</div>}
              </header>
            )}
            {children}
          </main>

          <footer className="wb-app-footer">
            <span>Charge Console · 常用充电桩状态与空闲提醒</span>
          </footer>
        </div>

        {/* COMMAND PALETTE (⌘K) */}
        <Dialog open={searchOpen} onOpenChange={setSearchOpen}>
          <DialogContent
            className="wb-theme wb-dialog wb-dialog-wide wb-search-dialog"
            initialFocus={searchRef}
          >
            <DialogHeader className="wb-dialog-header">
              <DialogTitle className="wb-dialog-title">快速定位</DialogTitle>
              <DialogDescription className="wb-dialog-description">
                {isAdmin ? "搜索用户或页面。" : "搜索充电桩名称、桩号或页面。"}
              </DialogDescription>
            </DialogHeader>
            <div className="wb-command">
              <div className="wb-command-input">
                <SearchIcon size={16} />
                <Input
                  ref={searchRef}
                  className="wb-input"
                  aria-label="快速搜索"
                  placeholder={
                    isAdmin ? "输入用户名或页面…" : "输入名称、桩号或页面…"
                  }
                  value={query}
                  onChange={(event) => {
                    setQuery(event.target.value)
                    setSearchIndex(0)
                  }}
                  onKeyDown={(event) => {
                    if (event.key === "ArrowDown") {
                      event.preventDefault()
                      const next = Math.max(
                        0,
                        Math.min(searchIndex + 1, results.length - 1)
                      )
                      setSearchIndex(next)
                      event.currentTarget
                        .closest(".wb-command")
                        ?.querySelectorAll(".wb-command-results > button")
                        [next]?.scrollIntoView({ block: "nearest" })
                    } else if (event.key === "ArrowUp") {
                      event.preventDefault()
                      const next = Math.max(0, searchIndex - 1)
                      setSearchIndex(next)
                      event.currentTarget
                        .closest(".wb-command")
                        ?.querySelectorAll(".wb-command-results > button")
                        [next]?.scrollIntoView({ block: "nearest" })
                    } else if (event.key === "Enter" && results[searchIndex]) {
                      if (event.nativeEvent.isComposing) return
                      event.preventDefault()
                      choose(results[searchIndex])
                    }
                  }}
                />
              </div>
              <div className="wb-command-results">
                {results.length ? (
                  results.map((item, index) => (
                    <button
                      key={item.href}
                      className={
                        index === searchIndex ? "wb-command-active" : ""
                      }
                      onClick={() => choose(item)}
                      onFocus={() => setSearchIndex(index)}
                    >
                      <span>
                        {item.title}
                        <small>{item.description}</small>
                      </span>
                      <ArrowRightIcon size={14} />
                    </button>
                  ))
                ) : (
                  <p className="wb-search-empty">
                    没有找到结果，换个关键词试试。
                  </p>
                )}
              </div>
              <div className="wb-command-footer">
                <span>↑ ↓ 选择</span>
                <span>Enter 打开</span>
                <span>Esc 关闭</span>
              </div>
            </div>
          </DialogContent>
        </Dialog>
      </div>
    </AppShellMenuContext.Provider>
  )
}
