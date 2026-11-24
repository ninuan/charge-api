"use client"

import { LoaderCircleIcon, MoonIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import type {
  NotificationPreference,
  NotificationPreferenceUpdateRequest,
} from "@/lib/api/generated"
import { notify } from "@/lib/feedback"
import { minutesToTime, timeToMinutes } from "@/lib/watch-format"

export function QuietHoursForm({
  preference,
  onSave,
}: {
  preference: NotificationPreference
  onSave: (
    payload: NotificationPreferenceUpdateRequest
  ) => Promise<NotificationPreference>
}) {
  const [saving, setSaving] = useState(false)
  const [enabled, setEnabled] = useState(preference.quietHoursEnabled)
  const [start, setStart] = useState(minutesToTime(preference.quietStartMinute))
  const [end, setEnd] = useState(minutesToTime(preference.quietEndMinute))

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const startMinute = timeToMinutes(start)
    const endMinute = timeToMinutes(end)
    if (startMinute == null || endMinute == null) {
      notify.warning("请选择有效的免打扰时段", {
        id: "quiet-hours-validation",
      })
      return
    }
    setSaving(true)
    try {
      await onSave({
        quietHoursEnabled: enabled,
        quietStartMinute: startMinute,
        quietEndMinute: endMinute,
        timezone: "Asia/Shanghai",
      })
      notify.success("免打扰设置已保存")
    } catch (reason) {
      notify.error(reason, { title: "保存免打扰设置失败" })
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <FieldGroup>
        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="quiet-enabled">
              <MoonIcon className="size-4" />
              启用免打扰
            </FieldLabel>
            <FieldDescription>
              开启后，这段时间不弹出网页或微信提醒，消息仍会保留在通知中心。
            </FieldDescription>
          </FieldContent>
          <Switch
            id="quiet-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
          />
        </Field>
        <FieldGroup className="grid gap-4 sm:grid-cols-2">
          <Field data-disabled={!enabled}>
            <FieldLabel htmlFor="quiet-start">开始时间</FieldLabel>
            <Input
              id="quiet-start"
              type="time"
              value={start}
              disabled={!enabled}
              onChange={(event) => setStart(event.target.value)}
            />
          </Field>
          <Field data-disabled={!enabled}>
            <FieldLabel htmlFor="quiet-end">结束时间</FieldLabel>
            <Input
              id="quiet-end"
              type="time"
              value={end}
              disabled={!enabled}
              onChange={(event) => setEnd(event.target.value)}
            />
          </Field>
        </FieldGroup>
        <Button className="wb-form-submit" type="submit" disabled={saving}>
          {saving ? (
            <LoaderCircleIcon
              data-icon="inline-start"
              className="motion-safe:animate-spin"
            />
          ) : null}
          {saving ? "保存中…" : "保存免打扰设置"}
        </Button>
      </FieldGroup>
    </form>
  )
}
