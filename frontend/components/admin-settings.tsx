import { PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"

import { AppearanceSettings } from "@/components/appearance-settings"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { adminApi } from "@/lib/admin-api"
import type { InviteCodePage, RegistrationSettings } from "@/lib/types"

type AdminSettingsProps = {
  settings: RegistrationSettings
  setSettings: (settings: RegistrationSettings) => void
  invitePage: InviteCodePage | null
  reload: (page?: number) => Promise<void>
}

const toggles = [
  [
    "openRegistration",
    "开放自助注册",
    "关闭后仅能通过管理员创建账户或邀请码注册。",
  ],
  ["inviteRequired", "注册需要邀请码", "仅在关闭公共注册时要求邀请码。"],
  [
    "defaultRefreshEnabled",
    "新账户默认允许刷新",
    "允许新账户主动向远端设备请求最新状态。",
  ],
] as const

export function AdminSettings({
  settings,
  setSettings,
  invitePage,
  reload,
}: AdminSettingsProps) {
  async function save(event: React.FormEvent) {
    event.preventDefault()
    try {
      setSettings(await adminApi.saveSettings(settings))
      toast.success("系统设置已保存")
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card className="shadow-xs">
        <CardHeader>
          <CardTitle className="text-base">注册策略</CardTitle>
          <CardDescription className="text-xs">
            这些规则只影响之后创建或注册的账户。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={save}>
            <FieldGroup>
              {toggles.map(([key, label, description]) => (
                <Field
                  key={key}
                  orientation="horizontal"
                  className="rounded-lg border p-3"
                >
                  <Checkbox
                    id={`setting-${key}`}
                    checked={settings[key]}
                    onCheckedChange={(checked) =>
                      setSettings({
                        ...settings,
                        [key]: checked,
                      })
                    }
                  />
                  <FieldContent>
                    <FieldLabel htmlFor={`setting-${key}`}>{label}</FieldLabel>
                    <FieldDescription>{description}</FieldDescription>
                  </FieldContent>
                </Field>
              ))}
              <Field>
                <FieldLabel htmlFor="device-limit">默认设备额度</FieldLabel>
                <Input
                  id="device-limit"
                  type="number"
                  value={settings.defaultDeviceLimit}
                  onChange={(event) =>
                    setSettings({
                      ...settings,
                      defaultDeviceLimit: Number(event.target.value),
                    })
                  }
                />
                <p className="text-xs text-muted-foreground">
                  每个新普通用户最多可添加的充电桩数量。
                </p>
              </Field>
              <Field>
                <FieldLabel htmlFor="retention">统计保留天数</FieldLabel>
                <Input
                  id="retention"
                  type="number"
                  value={settings.statsRetentionDays}
                  onChange={(event) =>
                    setSettings({
                      ...settings,
                      statsRetentionDays: Number(event.target.value),
                    })
                  }
                />
                <p className="text-xs text-muted-foreground">
                  用于运营总览和异常分析的历史数据保留时间。
                </p>
              </Field>
              <Button type="submit">保存设置</Button>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      <div className="grid content-start gap-4">
        <AppearanceSettings />
        <Card className="shadow-xs">
          <CardHeader>
            <CardTitle className="text-base">邀请码</CardTitle>
            <CardDescription className="text-xs">
              邀请码仅在关闭公共注册并启用邀请码要求时生效。
            </CardDescription>
            <CardAction className="max-sm:col-start-1 max-sm:row-start-3 max-sm:mt-2 max-sm:w-full max-sm:justify-self-stretch">
              <Button
                size="sm"
                className="max-sm:w-full"
                onClick={() =>
                  void adminApi
                    .createInvite()
                    .then(() => reload(1))
                    .catch((reason) => toast.error(reason.message))
                }
              >
                <PlusIcon />
                生成邀请码
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {invitePage?.items.map((invite) => (
              <div
                key={invite.id}
                className="flex items-center justify-between gap-3 rounded-lg border p-3"
              >
                <div>
                  <p className="font-mono text-sm">{invite.code}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    已使用 {invite.usedCount} 次
                    {invite.expiresAt
                      ? ` · 到期 ${new Date(invite.expiresAt).toLocaleDateString("zh-CN")}`
                      : " · 永不过期"}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`删除邀请码 ${invite.code}`}
                  onClick={() =>
                    void adminApi
                      .removeInvite(invite.id)
                      .then(() => reload(invitePage.page))
                      .catch((reason) => toast.error(reason.message))
                  }
                >
                  <Trash2Icon />
                </Button>
              </div>
            ))}
            {!invitePage?.items.length && (
              <p className="py-4 text-sm text-muted-foreground">
                暂无邀请码。生成后可用于受限注册。
              </p>
            )}
            {invitePage && invitePage.totalPages > 1 && (
              <div className="flex items-center justify-between pt-2">
                <p className="text-xs text-muted-foreground">
                  第 {invitePage.page}/{invitePage.totalPages} 页 · 共{" "}
                  {invitePage.total} 个
                </p>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={invitePage.page <= 1}
                    onClick={() => void reload(invitePage.page - 1)}
                  >
                    上一页
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={invitePage.page >= invitePage.totalPages}
                    onClick={() => void reload(invitePage.page + 1)}
                  >
                    下一页
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
