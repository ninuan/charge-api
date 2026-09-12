"use client"

import {
  ActivityIcon,
  ArrowRightIcon,
  BellIcon,
  BookOpenIcon,
  ChevronRightIcon,
  ClipboardListIcon,
  CommandIcon,
  InboxIcon,
  KeyRoundIcon,
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
import { WorkbenchButton } from "@/components/workbench/surfaces"
import { useAuth } from "@/lib/auth-context"
import { useOnline } from "@/lib/browser-state"
import { sessionExpiredEvent } from "@/lib/http"

const AppShellMenuContext = createContext<() => void>(() => {})
export function useCloseAppShellMenu() {
  return useContext(AppShellMenuContext)
}

export type WorkbenchSection =
  | "piles"
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

  const links = (items: NavItem[]) =>
    items.map(({ id, href, label, icon: Icon, count }) => (
      <Link
        key={id}
        href={href}
        className={`wb-nav-item ${active === id ? "wb-nav-active" : ""}`}
        aria-label={count ? `${label}，${count} 项` : label}
        aria-current={active === id ? "page" : undefined}
        onClick={closeMenu}
      >
        <Icon size={18} strokeWidth={1.8} />
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
      <Skeleton className="size-8 rounded-full" />
      <Skeleton className="h-5 w-20" />
    </div>
  ) : (
    <Link
      href={isAdmin ? "/account?tab=security" : "/account"}
      aria-label="进入账户中心"
      className="wb-sidebar-user"
      onClick={closeMenu}
    >
      <span className="wb-avatar">
        {currentUser?.username.slice(0, 1).toUpperCase()}
      </span>
      <span>
        <strong>{currentUser?.username}</strong>
        <small>{isAdmin ? "管理员" : "普通用户"}</small>
      </span>
      <ChevronRightIcon size={15} />
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
        <div className="wb-app-shell">
          <aside className="wb-sidebar" aria-label="工作台导航">
            <Link
              className="wb-home"
              href={isAdmin ? "/admin" : "/dashboard"}
              aria-label="返回工作区首页"
            >
              <Brand />
            </Link>
            <div className="wb-workspace-label">
              <span className="wb-workspace-dot" />
              {isAdmin ? "管理工作区" : "我的充电空间"}
            </div>
            <nav aria-label="主导航">
              <div className="wb-nav-group-label">
                {isAdmin ? "工作区" : "日常使用"}
              </div>
              {links(primary)}
              <div className="wb-nav-group-label wb-nav-second">
                {isAdmin ? "系统管理" : "偏好与连接"}
              </div>
              {links(secondary)}
            </nav>
            <div className="wb-sidebar-spacer" />
            <div className="wb-sidebar-help">
              {guideAction || (
                <Link
                  href={
                    isAdmin ? "/account?tab=security" : "/dashboard?guide=1"
                  }
                >
                  <BookOpenIcon size={16} />
                  {isAdmin ? "账户安全" : "使用说明"}
                  <ArrowRightIcon size={13} />
                </Link>
              )}
            </div>
            {identity}
          </aside>
          <div className="wb-app-main">
            <header data-slot="app-header" className="wb-topbar">
              <div className="wb-mobile-header">
                <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
                  <SheetTrigger
                    render={
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label="打开菜单"
                      />
                    }
                  >
                    <MenuIcon />
                  </SheetTrigger>
                  <SheetContent
                    side="left"
                    className="wb-theme workbench-drawer w-[calc(100vw-1rem)] max-w-[22rem] overflow-y-auto"
                  >
                    <SheetTitle>账户与操作</SheetTitle>
                    <nav aria-label="移动导航" className="mt-6">
                      {links(primary)}
                      <div className="wb-nav-group-label wb-nav-second">
                        {isAdmin ? "系统管理" : "偏好与连接"}
                      </div>
                      {links(secondary)}
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
                <Link
                  href={isAdmin ? "/admin" : "/dashboard"}
                  aria-label="工作区首页"
                >
                  <Brand small />
                </Link>
              </div>
              <div className="wb-breadcrumb">
                <span>{isAdmin ? "管理工作区" : "我的空间"}</span>
                <ChevronRightIcon size={12} />
                <strong>{title}</strong>
              </div>
              <div className="wb-topbar-actions">
                <button
                  className="wb-global-search"
                  aria-label="打开快速搜索"
                  onClick={() => setSearchOpen(true)}
                >
                  <SearchIcon size={15} />
                  <span>{isAdmin ? "搜索用户或页面" : "搜索充电桩或页面"}</span>
                  <kbd>
                    <CommandIcon size={10} />K
                  </kbd>
                </button>
                {notificationAction}
                <span className="wb-topbar-divider" />
                <ThemeToggle />
              </div>
            </header>
            <main id="main-content" tabIndex={-1} className="wb-main-content">
              <GlobalFeedbackBanner />
              {!online && (
                <div
                  role="status"
                  className="wb-state-banner wb-banner-offline"
                >
                  <WifiOffIcon size={17} />
                  <div>
                    <strong>当前离线</strong>
                    <span>保留最近读取的内容，联网后再刷新或保存。</span>
                  </div>
                </div>
              )}
              {currentUser?.mustChangePassword && (
                <div
                  className="wb-state-banner wb-banner-warning"
                  role="status"
                >
                  <KeyRoundIcon size={18} />
                  <div>
                    <strong>当前使用的是管理员生成的临时密码</strong>
                    <span>请修改为只有你本人知道的新密码。</span>
                  </div>
                  <WorkbenchButton
                    variant="outline"
                    onClick={() =>
                      router.push("/account?tab=security#change-password")
                    }
                  >
                    修改密码
                  </WorkbenchButton>
                </div>
              )}
              <header className="wb-page-heading">
                <div>
                  <h1>{title}</h1>
                  <p>{description}</p>
                </div>
                {actions && <div className="wb-heading-actions">{actions}</div>}
              </header>
              {children}
            </main>
            <footer className="wb-app-footer">
              <span>
                Charge Console<span>让等待少一点。</span>
              </span>
            </footer>
          </div>
          <nav className="wb-mobile-bottom" aria-label="常用页面">
            {[
              ...primary,
              {
                id: "account" as const,
                href: isAdmin ? "/account?tab=security" : "/account",
                label: "我的",
                icon: Settings2Icon,
              },
            ].map(({ id, href, label, icon: Icon, count }) => (
              <Link
                key={id}
                href={href}
                aria-label={label}
                aria-current={active === id ? "page" : undefined}
              >
                <span>
                  <Icon size={20} />
                  {!!count && <i>{count > 99 ? "99+" : count}</i>}
                </span>
                <span>{label}</span>
              </Link>
            ))}
          </nav>
        </div>
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
                <SearchIcon size={18} />
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
                      <ArrowRightIcon size={15} />
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
