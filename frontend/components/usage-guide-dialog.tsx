"use client"

import {
  BookOpenCheckIcon,
  CheckCircle2Icon,
  MousePointer2Icon,
  ShieldCheckIcon,
} from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { notify } from "@/lib/feedback"

import { useCloseAppShellMenu } from "@/components/app-shell"
import { Button } from "@/components/ui/button"
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

const steps = [
  [
    "准备微信",
    "确保可以扫码登录",
    [
      "准备一台可以正常使用微信的手机。",
      "确认微信可以扫码并完成授权。",
      "本系统不会要求输入微信密码。",
    ],
  ],
  [
    "打开扫码登录",
    "在系统里生成二维码",
    [
      "打开“账户与设置”，进入“平台连接”。",
      "点击“扫码登录”。",
      "在弹窗中点击“生成二维码”。",
      "等待二维码显示出来，不要关闭弹窗。",
    ],
  ],
  [
    "使用微信扫码",
    "完成授权登录",
    [
      "使用微信扫描页面里的二维码。",
      "按微信页面提示完成确认。",
      "扫码后回到系统页面，扫码状态会自动更新。",
    ],
  ],
  [
    "确认绑定状态",
    "完成当前账号绑定",
    [
      "微信授权成功后，“确认绑定”按钮会亮起，请点击完成绑定。",
      "页面显示“微信已绑定”后，点击“继续添加充电桩”或“完成”。",
      "如果已经添加过充电桩，绑定成功后即可继续刷新查看。",
    ],
  ],
  [
    "添加充电桩",
    "输入桩号或设备长 ID",
    [
      "打开“常用充电桩”。",
      "点击“添加充电桩”。",
      "输入桩号或设备长 ID。",
      "点击添加后，系统会自动查询并保存该充电桩。",
    ],
  ],
  [
    "刷新查看状态",
    "查看充电口占用情况",
    [
      "添加成功后，充电桩会出现在看板中。",
      "点击“刷新状态”获取最新充电口占用情况。",
      "系统会显示每个充电口是空闲、使用中、离线还是异常。",
      "刚刷新过时，页面可能继续显示最近一次结果。",
    ],
  ],
  [
    "使用空闲提醒",
    "需要充电时开启，找到空位后自动停止",
    [
      "在看板中找到目标充电桩，点击“有空闲时提醒我”。",
      "选择等待 1、2、4 小时或持续到今晚断电前，然后点击“开始提醒”。",
      "系统会先查看一次当前状态；如果已经有空闲口，会直接告诉你端口号，不再继续后台检查。",
      "没有空闲口时，提醒会在所选时间内运行；发现空闲、等待到期或手动取消后都会自动停止。",
      "打开导航里的“空闲提醒”，可以查看剩余时间和下次检查，也可以调整或取消。",
      "每次需要充电时重新开启即可，发现空闲口、到期或进入计划断电时段后会自动停止。",
      "学校计划断电时段不会发送离线提醒；恢复供电后仍持续离线，才会提示你检查。",
      "如需浏览器弹窗提醒，请在“账户与设置 → 通知设置”中允许通知，并保持网页打开。",
    ],
  ],
  [
    "使用微信提醒",
    "离开网页也能收到已开启的消息",
    [
      "打开“账户与设置 → 通知设置”，在“微信提醒”中点击“获取二维码”。",
      "使用微信扫码并关注，等待页面显示“已绑定”。",
      "打开微信提醒，并选择要接收的空闲口、重新登录、无法连接或恢复连接消息。",
      "点击“发送测试消息”，在最近测试中确认发送结果。",
      "免打扰时段不弹出网页或微信提醒，消息仍会保留在通知中心。",
      "微信接收方式与渠道限制以 WxPusher 当前说明为准；可以在对应客户端确认消息。",
      "关闭总开关或解除绑定不会删除通知中心里的历史消息。",
      "消息发出后，请在 WxPusher App 或微信中确认是否收到；页面无法确认是否已经阅读。",
    ],
  ],
] as const

export function UsageGuideDialog({
  initialOpen = false,
}: { initialOpen?: boolean } = {}) {
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
          <Button variant="outline" onClick={openReference}>
            <BookOpenCheckIcon />
            使用说明
          </Button>
        }
      />
      <DialogContent
        showCloseButton={!required || reachedEnd}
        className="grid h-[min(46rem,calc(100dvh-2rem))] w-[min(64rem,calc(100%-2rem))] max-w-none grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-none"
      >
        <DialogHeader className="border-b p-5 sm:p-6">
          <div className="flex gap-3">
            <span className="rounded-lg bg-muted p-2">
              <BookOpenCheckIcon className="size-5" />
            </span>
            <div>
              <DialogTitle>Charge Console 使用说明</DialogTitle>
              {required && (
                <DialogDescription className="mt-2">
                  首次使用请先了解扫码绑定、添加充电桩和空闲提醒。
                </DialogDescription>
              )}
            </div>
          </div>
        </DialogHeader>
        <div
          ref={scrollRef}
          className="min-h-0 overflow-y-auto p-5 sm:p-6"
          onScroll={(event) => checkEnd(event.currentTarget)}
        >
          <div className="grid gap-6 md:grid-cols-[12rem_1fr]">
            <aside className="sticky top-0 hidden self-start border-r pr-4 md:block">
              <p className="text-xs font-medium text-muted-foreground">
                操作路径
              </p>
              <ol className="mt-3 space-y-3">
                {steps.map(([title], index) => (
                  <li key={title}>
                    <a
                      className="text-sm hover:underline"
                      href={`#guide-${index + 1}`}
                    >
                      {index + 1}. {title}
                    </a>
                  </li>
                ))}
              </ol>
            </aside>
            <div className="space-y-4">
              {steps.map(([title, detail, items], index) => (
                <section
                  id={`guide-${index + 1}`}
                  key={title}
                  className="rounded-lg border p-5"
                >
                  <div className="flex gap-3">
                    <span className="grid size-7 shrink-0 place-items-center rounded-full bg-muted text-sm font-medium">
                      {index + 1}
                    </span>
                    <div>
                      <h3 className="font-semibold">{title}</h3>
                      <p className="mt-1 text-sm text-muted-foreground">
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
