"use client"

import {
  BookOpenCheckIcon,
  CheckCircle2Icon,
  MousePointer2Icon,
  ShieldCheckIcon,
  SparklesIcon,
} from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { notify } from "@/lib/feedback"

import { useCloseAppShellMenu } from "@/components/app-shell"
import { WorkbenchButton as Button } from "@/components/workbench/surfaces"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { useAuth } from "@/lib/auth-context"

const updates = [
  {
    badge: "老用户必看",
    title: "告别微信扫码，零风控使用",
    summary:
      "原微信扫码登录已下线。如果你此前已配置个人 Cookie，前往「账户与设置 → 平台连接」点击「立即加入互助池」即可恢复不限次刷新与空位提醒。",
  },
  {
    badge: "免费可用",
    title: "公共桩库 + 每日/永久额度",
    summary:
      "不传 Cookie 也能直接使用：可从「公共桩库」一键加桩，享每日 00:00 重置的每日额度；录入全站新桩获赠永久额度（跨天不清零），全站 60s 共享缓存免扣次数。",
  },
  {
    badge: "省额防风控",
    title: "支持单桩刷新 & 一号一池",
    summary:
      "每台桩卡片和右侧详情抽屉均可单独刷新，避免全量刷浪费额度；同一微信账号的 Cookie 仅可入池一次，请勿与他人共用同一个凭据。",
  },
] as const

const steps = [
  [
    "添加常用充电桩",
    "从公共桩库一键添加，或录入新桩赚取永久额度",
    [
      "登录账号后即可直接使用，无需繁琐初始化配置。",
      "在看板顶部切换到“公共桩库”，或点击右上角“添加充电桩 → 从公共桩库添加”，按名称、桩号或位置搜索并一键加入常用列表。",
      "如果公共桩库里还没有目标充电桩，切换到“录入新桩”，输入桩身二维码下方的数字桩号（或设备长 ID）即可校验并收录。",
      "首次成功收录全站尚未登记的新桩时，系统会自动赠送永久查询额度（跨天不清零，用完即止）。",
      "录入新桩时“常用名称”可留空，系统会自动采用平台真实站点名称；添加后也可在充电桩详情中按个人习惯自定义备注。",
    ],
  ],
  [
    "刷新状态与查询额度",
    "区分每日额度、永久额度与全站共享缓存",
    [
      "在“我的常用桩”中，可以点击单台桩卡片（或右侧详情抽屉顶部）的刷新按钮只刷这一台，也可以点击右上角“刷新全部”。",
      "免费模式下的查询额度分为两类：每日额度（每天北京时间 00:00 自动重置）和永久额度（贡献新桩或管理员发放，跨天不清零）。",
      "发起实时刷新时，系统会优先消耗每日额度，每日额度用完后才会消耗永久额度。",
      "如果某台充电桩在全站缓存有效期内（默认 60 秒）刚被其他用户刷新过，会直接命中共享缓存同步最新状态，不消耗任何查询次数。",
      "点击充电桩卡片中的任意充电口，可查看该端口是空闲、使用中还是离线，以及已用时长与预计剩余时间（以现场状态为准）。",
    ],
  ],
  [
    "解锁不限次与互助公共池",
    "上传个人 Cookie，解锁不限次刷新与空位提醒",
    [
      "打开“账户与设置 → 平台连接”，点击“上传 Cookie”。",
      "直接粘贴抓包工具复制的 Copy as cURL、完整 HTTP 请求头或纯 Cookie 字符串，系统会自动提取有效字段并校验。",
      "当系统开启互助池策略时，勾选“加入全站互助公共池”即可立即解锁不限次刷新与空位提醒；已有个人 Cookie 时也可一键“立即加入互助池”或随时“退出互助池”。",
      "为避免凭据重复占用与集中风控，同一上游微信账号的 Cookie 在公共池中仅允许绑定一个账户，请勿与他人共用同一个 Cookie。",
      "个人 Cookie 均加密保存在服务端且不会在页面回显明文；若后续失效，刷新时会自动平滑降级使用公共池额度，并通知你及时更新。",
    ],
  ],
  [
    "使用空闲提醒",
    "需要充电时开启，找到空位后自动停止",
    [
      "空闲提醒默认对已解锁个人 Cookie 权益的用户开放（具体以管理员系统策略为准）。",
      "在看板中找到目标充电桩，点击“有空闲时提醒我”，选择等待 1、2、4 小时或持续到今晚断电前，然后点击“开始提醒”。",
      "系统会先查看一次当前状态；如果已经有空闲口，会直接告诉你端口号，不再继续后台检查。",
      "没有空闲口时，提醒会在所选时间内按固定间隔后台检查；发现空闲、等待到期、进入计划断电时段或手动取消后都会自动停止。",
      "打开导航里的“空闲提醒”，可以查看剩余时间和下次检查，也可以延长或取消。",
      "学校计划断电时段不会发送离线提醒；恢复供电后仍持续离线，才会提示你检查。",
      "如需浏览器弹窗提醒，请在“账户与设置 → 通知设置”中允许通知，并保持网页打开。",
    ],
  ],
  [
    "使用微信提醒",
    "离开网页也能收到空闲口与凭据失效通知",
    [
      "打开“账户与设置 → 通知设置”，在“微信提醒”中点击“获取二维码”。",
      "使用微信扫码并关注，等待页面显示“已绑定”。",
      "打开微信提醒，并选择要接收的空闲口、凭据失效、无法连接或恢复连接消息。",
      "点击“发送测试消息”，在最近测试中确认发送结果。",
      "免打扰时段不弹出网页或微信提醒，消息仍会保留在通知中心。",
      "微信接收方式与渠道限制以 WxPusher 当前说明为准；可以在对应客户端确认消息。",
      "关闭总开关或解除绑定不会删除通知中心里的历史消息。",
    ],
  ],
] as const

export function UsageGuideDialog({
  initialOpen = false,
  compact = false,
}: { initialOpen?: boolean; compact?: boolean } = {}) {
  const { currentUser, acknowledgeUsageGuide } = useAuth()
  const closeAppShellMenu = useCloseAppShellMenu()
  const [open, setOpen] = useState(initialOpen)
  const [required, setRequired] = useState(false)
  const [reachedEnd, setReachedEnd] = useState(initialOpen)
  const [saving, setSaving] = useState(false)
  const promptedRef = useRef("")
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const user = currentUser
    if (
      !user ||
      user.role !== "user" ||
      user.usageGuideAckAt ||
      promptedRef.current === user.id
    )
      return
    promptedRef.current = user.id
    setRequired(true)
    setReachedEnd(false)
    setOpen(true)
  }, [currentUser])

  useEffect(() => {
    // 视口够高、内容无需滚动时 onScroll 永远不会触发，
    // 打开后先做一次初始检测，避免确认按钮被永久锁死。
    if (!open) return
    const frame = requestAnimationFrame(() => {
      if (scrollRef.current) checkEnd(scrollRef.current)
    })
    return () => cancelAnimationFrame(frame)
  }, [open])

  function openReference() {
    setRequired(false)
    setReachedEnd(true)
    setOpen(true)
  }
  async function close() {
    if (required && !reachedEnd) return
    if (!required) {
      setOpen(false)
      closeAppShellMenu()
      return
    }
    setSaving(true)
    try {
      await acknowledgeUsageGuide()
      setRequired(false)
      setOpen(false)
      closeAppShellMenu()
    } catch (reason) {
      notify.error(reason, {
        title: "保存阅读状态失败",
        id: "usage-guide-acknowledge",
      })
    } finally {
      setSaving(false)
    }
  }
  function handleOpen(next: boolean) {
    if (!next && required && !reachedEnd) return
    setOpen(next)
    if (!next) closeAppShellMenu()
  }
  // 读到过底部就保持已读：往回翻不该撤销"已看完"。
  function checkEnd(target: HTMLElement) {
    setReachedEnd(
      (current) =>
        current ||
        target.scrollTop + target.clientHeight >= target.scrollHeight - 8
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpen}>
      <DialogTrigger
        render={
          compact ? (
            <button
              type="button"
              className="wb-header-icon-link wb-guide-header-btn"
              aria-label="使用说明"
              title="使用说明"
              onClick={openReference}
            >
              <BookOpenCheckIcon size={16} strokeWidth={1.8} />
            </button>
          ) : (
            <Button variant="outline" onClick={openReference}>
              <BookOpenCheckIcon />
              使用说明
            </Button>
          )
        }
      />
      <DialogContent
        showCloseButton={!required || reachedEnd}
        className="grid h-[min(46rem,calc(100dvh-2rem))] w-[min(64rem,calc(100%-2rem))] max-w-none grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-none"
      >
        <DialogHeader className="border-b px-5 py-4 sm:px-6 sm:py-5">
          <div className="flex items-center gap-3 pr-8">
            <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-muted">
              <BookOpenCheckIcon className="size-5" />
            </span>
            <div className="min-w-0">
              <DialogTitle className="text-base leading-snug">
                Charge Console 使用说明
              </DialogTitle>
              <DialogDescription className="mt-0.5 text-xs leading-5">
                {required
                  ? "首次使用请先查看最新变化，以及公共桩库、查询额度与空闲提醒说明。"
                  : "老用户可优先查看顶部升级要点，下方附完整分步操作指引。"}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>
        <div
          ref={scrollRef}
          className="min-h-0 overflow-y-auto p-5 sm:p-6"
          onScroll={(event) => checkEnd(event.currentTarget)}
        >
          <div className="grid gap-6 md:grid-cols-[12.5rem_1fr]">
            <aside className="sticky top-0 hidden self-start border-r pr-4 md:block">
              <a
                className="flex items-center gap-1.5 rounded-md bg-primary/10 px-2.5 py-1.5 text-sm font-medium text-primary hover:bg-primary/15"
                href="#guide-whats-new"
              >
                <SparklesIcon className="size-3.5 shrink-0" />
                本次升级要点
              </a>
              <p className="mt-4 text-xs font-medium text-muted-foreground">
                操作路径
              </p>
              <ol className="mt-2.5 space-y-2.5">
                {steps.map(([title], index) => (
                  <li key={title}>
                    <a
                      className="block text-sm text-muted-foreground hover:text-foreground hover:underline"
                      href={`#guide-${index + 1}`}
                    >
                      {index + 1}. {title}
                    </a>
                  </li>
                ))}
              </ol>
            </aside>
            <div className="space-y-5">
              <section
                id="guide-whats-new"
                aria-label="本次升级要点"
                className="rounded-xl border border-primary/25 bg-primary/5 p-5"
              >
                <div className="flex items-center gap-2.5">
                  <span className="grid size-7 shrink-0 place-items-center rounded-full bg-primary/15 text-primary">
                    <SparklesIcon className="size-4" />
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold">
                      本次升级要点（老用户优先看这里）
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      使用方式已升级为「公共池 + 个人 Cookie 互助」，30 秒快速了解变化
                    </p>
                  </div>
                </div>
                <div className="mt-4 grid gap-3 sm:grid-cols-3">
                  {updates.map((item) => (
                    <div
                      key={item.title}
                      className="rounded-lg border bg-background/90 p-3.5"
                    >
                      <span className="inline-block rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary">
                        {item.badge}
                      </span>
                      <h4 className="mt-2 text-sm font-semibold leading-snug">
                        {item.title}
                      </h4>
                      <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                        {item.summary}
                      </p>
                    </div>
                  ))}
                </div>
              </section>

              <div className="flex items-center gap-3 pt-1">
                <span className="text-xs font-medium text-muted-foreground">
                  完整操作指引（按需查阅）
                </span>
                <span className="h-px flex-1 bg-border" />
              </div>

              <div className="space-y-4">
                {steps.map(([title, detail, items], index) => (
                  <section
                    id={`guide-${index + 1}`}
                    key={title}
                    className="rounded-lg border p-5"
                  >
                    <div className="flex items-center gap-3">
                      <span className="grid size-7 shrink-0 place-items-center rounded-full bg-muted text-sm font-medium">
                        {index + 1}
                      </span>
                      <div>
                        <h3 className="font-semibold leading-snug">{title}</h3>
                        <p className="mt-0.5 text-sm text-muted-foreground">
                          {detail}
                        </p>
                      </div>
                    </div>
                    <ol className="mt-4 list-decimal space-y-2 pl-5 text-sm leading-6 text-muted-foreground">
                      {items.map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                    </ol>
                  </section>
                ))}
              </div>
            </div>
          </div>
        </div>
        <DialogFooter className="mx-0 mb-0 rounded-none border-t bg-background p-4 sm:px-6">
          <div className="flex w-full flex-col justify-between gap-3 sm:flex-row sm:items-center">
            <p className="flex items-center gap-2 text-xs text-muted-foreground">
              {reachedEnd ? (
                <ShieldCheckIcon className="size-4" />
              ) : (
                <MousePointer2Icon className="size-4" />
              )}
              {reachedEnd
                ? "已读到说明底部，可以开始使用。"
                : "请继续向下滚动，看完整个说明。"}
            </p>
            <Button
              disabled={!reachedEnd || saving}
              onClick={() => void close()}
            >
              <CheckCircle2Icon />
              {saving ? "正在确认…" : required ? "我已看完并关闭" : "关闭"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
