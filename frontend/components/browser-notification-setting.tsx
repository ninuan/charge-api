"use client"

import { BellIcon, BellOffIcon, LoaderCircleIcon } from "lucide-react"
import { useState } from "react"

import { LeadingIcon } from "@/components/leading-icon"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { notify } from "@/lib/feedback"
import { useNotifications } from "@/lib/notification-context"
import { useWatch } from "@/lib/watch-context"
import { formatPowerWindow } from "@/lib/watch-format"

const browserSettingLayout =
  "grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 p-4 [&>[data-slot=leading-icon]]:row-span-2"

export function BrowserNotificationSetting() {
  const { preference } = useWatch()
  const { browserPermission, requestBrowserPermission, setBrowserEnabled } =
    useNotifications()
  const [pending, setPending] = useState(false)
  const enabled =
    browserPermission === "granted" && Boolean(preference?.browserEnabled)

  async function requestPermission() {
    setPending(true)
    try {
      const permission = await requestBrowserPermission()
      if (permission === "granted") notify.success("浏览器通知已开启")
      else if (permission === "denied")
        notify.warning("通知权限未开启", {
          description: "请在浏览器的网站设置中允许通知。",
          id: "browser-notification-permission",
        })
    } catch (reason) {
      notify.error(reason, { title: "申请通知权限失败" })
    } finally {
      setPending(false)
    }
  }

  async function toggle(next: boolean) {
    setPending(true)
    try {
      const applied = await setBrowserEnabled(next)
      const livePermission =
        typeof window !== "undefined" && "Notification" in window
          ? window.Notification.permission
          : browserPermission
      if (next && !applied && livePermission === "denied")
        notify.warning("通知权限未开启", {
          description: "请先在浏览器的网站设置中允许通知。",
          id: "browser-notification-permission",
        })
      else notify.success(applied ? "浏览器通知已开启" : "浏览器通知已关闭")
    } catch (reason) {
      notify.error(reason, { title: "更新浏览器通知失败" })
    } finally {
      setPending(false)
    }
  }

  if (browserPermission === "unsupported")
    return (
      <Alert className={browserSettingLayout}>
        <LeadingIcon icon={BellOffIcon} />
        <AlertTitle>当前浏览器不支持网页通知</AlertTitle>
        <AlertDescription>
          消息仍会保留在通知中心，可以随时从右上角查看。
        </AlertDescription>
      </Alert>
    )

  if (browserPermission === "denied")
    return (
      <Alert variant="destructive" className={browserSettingLayout}>
        <LeadingIcon icon={BellOffIcon} className="bg-destructive/10" />
        <AlertTitle>浏览器通知权限已被拒绝</AlertTitle>
        <AlertDescription>
          请在浏览器的网站通知设置中开启权限，消息仍可在通知中心查看。
        </AlertDescription>
      </Alert>
    )

  if (browserPermission === "default")
    return (
      <Alert className="wb-browser-notification">
        <LeadingIcon icon={BellIcon} className="bg-primary/10 text-primary" />
        <div className="wb-browser-notification-copy">
          <AlertTitle>网页开着时提醒我</AlertTitle>
          <AlertDescription>
            只要此网页保持打开，就会弹出空闲口和账户异常提醒。
          </AlertDescription>
        </div>
        <div className="wb-browser-notification-action">
          <Button
            type="button"
            disabled={pending}
            onClick={() => void requestPermission()}
          >
            {pending ? (
              <LoaderCircleIcon className="motion-safe:animate-spin" />
            ) : null}
            允许通知
          </Button>
        </div>
      </Alert>
    )

  return (
    <Alert className={browserSettingLayout}>
      <LeadingIcon icon={BellIcon} className="bg-primary/10 text-primary" />
      <AlertTitle className="flex items-center justify-between gap-3">
        <span>网页提醒</span>
        <Switch
          size="sm"
          aria-label="网页提醒"
          checked={enabled}
          disabled={pending || !preference}
          onCheckedChange={(next) => void toggle(next)}
        />
      </AlertTitle>
      <AlertDescription>
        {preference?.quietHoursEnabled
          ? `${formatPowerWindow(preference.quietStartMinute, preference.quietEndMinute)} 暂停弹窗，消息仍会保留在通知中心。`
          : "网页保持打开时，有空闲口或账户问题会及时提醒你。"}
      </AlertDescription>
    </Alert>
  )
}
