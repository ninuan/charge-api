import { PlusIcon, Trash2Icon } from "lucide-react"
import { notify } from "@/lib/feedback"

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
import { Switch } from "@/components/ui/switch"
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

function minuteToTime(value: number) {
  const normalized = Number.isFinite(value)
    ? Math.max(0, Math.min(1439, value))
    : 0
  return `${String(Math.floor(normalized / 60)).padStart(2, "0")}:${String(normalized % 60).padStart(2, "0")}`
}

function timeToMinute(value: string) {
  const match = /^(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  return Number(match[1]) * 60 + Number(match[2])
}

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
      notify.success("系统设置已保存")
    } catch (reason) {
      notify.error(reason, { title: "保存系统设置失败" })
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <div className="grid content-start gap-4">
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
                      <FieldLabel htmlFor={`setting-${key}`}>
                        {label}
                      </FieldLabel>
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
                    min={1}
                    max={365}
                    value={settings.statsRetentionDays}
                    onChange={(event) =>
                      setSettings({
                        ...settings,
                        statsRetentionDays: Number(event.target.value),
                      })
                    }
                  />
                  <FieldDescription>
                    用于运营趋势和异常分析，范围为 1–365 天。
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="port-history-retention">
                    端口历史保留天数
                  </FieldLabel>
                  <Input
                    id="port-history-retention"
                    type="number"
                    min={1}
                    max={365}
                    value={settings.portHistoryRetentionDays}
                    onChange={(event) =>
                      setSettings({
                        ...settings,
                        portHistoryRetentionDays: Number(event.target.value),
                      })
                    }
                  />
                  <FieldDescription>
                    控制状态时间线、占用趋势和热力图的数据范围，范围为 1–365
                    天。
                  </FieldDescription>
                </Field>
                <Button type="submit">保存设置</Button>
              </FieldGroup>
            </form>
          </CardContent>
        </Card>

        <Card className="shadow-xs">
          <CardHeader>
            <CardTitle className="text-base">空闲提醒与后台调度</CardTitle>
            <CardDescription className="text-xs">
              后台以整桩为单位低频刷新；同一桩任一端口变为空闲时生成提醒。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={save}>
              <FieldGroup>
                <Field
                  orientation="horizontal"
                  className="rounded-lg border p-3"
                >
                  <FieldContent>
                    <FieldLabel htmlFor="background-reminders">
                      启用后台空闲提醒
                    </FieldLabel>
                    <FieldDescription>
                      关闭后保留用户规则，但停止所有后台请求和新提醒。
                    </FieldDescription>
                  </FieldContent>
                  <Switch
                    id="background-reminders"
                    checked={settings.backgroundRemindersEnabled}
                    onCheckedChange={(checked) =>
                      setSettings({
                        ...settings,
                        backgroundRemindersEnabled: checked,
                      })
                    }
                  />
                </Field>

                <FieldGroup className="grid gap-4 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="watch-refresh-interval">
                      刷新间隔（分钟）
                    </FieldLabel>
                    <Input
                      id="watch-refresh-interval"
                      type="number"
                      min={5}
                      max={60}
                      value={settings.watchRefreshIntervalMinutes}
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          watchRefreshIntervalMinutes: Number(
                            event.target.value
                          ),
                        })
                      }
                    />
                    <FieldDescription>范围 5–60 分钟。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="watch-pile-limit">
                      单用户提醒桩上限
                    </FieldLabel>
                    <Input
                      id="watch-pile-limit"
                      type="number"
                      min={1}
                      max={20}
                      value={settings.watchPileLimitPerUser}
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          watchPileLimitPerUser: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>范围 1–20 台桩。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="watch-daily-quota">
                      单用户每日请求额度
                    </FieldLabel>
                    <Input
                      id="watch-daily-quota"
                      type="number"
                      min={1}
                      max={10000}
                      value={settings.watchDailyRefreshQuota}
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          watchDailyRefreshQuota: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      只计实际远端请求，缓存命中不扣额度。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="notification-retention">
                      通知保留天数
                    </FieldLabel>
                    <Input
                      id="notification-retention"
                      type="number"
                      min={7}
                      max={365}
                      value={settings.notificationRetentionDays}
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          notificationRetentionDays: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      仅清理超过期限且已经解决的通知。
                    </FieldDescription>
                  </Field>
                </FieldGroup>

                <Field
                  orientation="horizontal"
                  className="rounded-lg border p-3"
                >
                  <FieldContent>
                    <FieldLabel htmlFor="scheduled-power-off">
                      启用学校计划断电窗口
                    </FieldLabel>
                    <FieldDescription>
                      窗口内暂停请求且不发送离线提醒，恢复供电后分散重试。
                    </FieldDescription>
                  </FieldContent>
                  <Switch
                    id="scheduled-power-off"
                    checked={settings.scheduledPowerOffEnabled}
                    onCheckedChange={(checked) =>
                      setSettings({
                        ...settings,
                        scheduledPowerOffEnabled: checked,
                      })
                    }
                  />
                </Field>

                <FieldGroup className="grid gap-4 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="power-off-start">
                      断电开始时间
                    </FieldLabel>
                    <Input
                      id="power-off-start"
                      type="time"
                      value={minuteToTime(
                        settings.scheduledPowerOffStartMinute
                      )}
                      onChange={(event) => {
                        const minute = timeToMinute(event.target.value)
                        if (minute === null) return
                        setSettings({
                          ...settings,
                          scheduledPowerOffStartMinute: minute,
                        })
                      }}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="power-off-end">
                      恢复供电时间
                    </FieldLabel>
                    <Input
                      id="power-off-end"
                      type="time"
                      value={minuteToTime(settings.scheduledPowerOffEndMinute)}
                      onChange={(event) => {
                        const minute = timeToMinute(event.target.value)
                        if (minute === null) return
                        setSettings({
                          ...settings,
                          scheduledPowerOffEndMinute: minute,
                        })
                      }}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="power-off-timezone">时区</FieldLabel>
                    <Input
                      id="power-off-timezone"
                      value={settings.scheduledPowerOffTimezone}
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          scheduledPowerOffTimezone: event.target.value,
                        })
                      }
                    />
                    <FieldDescription>例如 Asia/Shanghai。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="power-restore-jitter">
                      恢复请求分散时间（分钟）
                    </FieldLabel>
                    <Input
                      id="power-restore-jitter"
                      type="number"
                      min={0}
                      max={60}
                      value={settings.powerRestoreJitterMinutes}
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          powerRestoreJitterMinutes: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      避免恢复供电时同时请求，范围 0–60 分钟。
                    </FieldDescription>
                  </Field>
                </FieldGroup>
                <Button type="submit">保存提醒策略</Button>
              </FieldGroup>
            </form>
          </CardContent>
        </Card>
      </div>
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
                    .then(async () => {
                      await reload(1)
                      notify.success("邀请码已生成")
                    })
                    .catch((reason) =>
                      notify.error(reason, { title: "生成邀请码失败" })
                    )
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
                      .then(async () => {
                        await reload(invitePage.page)
                        notify.success("邀请码已删除")
                      })
                      .catch((reason) =>
                        notify.error(reason, { title: "删除邀请码失败" })
                      )
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
