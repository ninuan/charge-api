"use client"

import {
  KeyRoundIcon,
  LaptopIcon,
  LoaderCircleIcon,
  LogOutIcon,
} from "lucide-react"
import { useEffect, useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/lib/auth-context"
import type { SessionView } from "@/lib/types"

export function AccountSecurityPanel() {
  const { changePassword, fetchSessions, logoutOtherSessions } = useAuth()
  const [sessions, setSessions] = useState<SessionView[]>([])
  const [currentPassword, setCurrentPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [saving, setSaving] = useState(false)
  const loadSessions = async () => {
    try {
      setSessions(await fetchSessions())
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  useEffect(() => {
    let active = true
    void fetchSessions()
      .then((nextSessions) => {
        if (active) setSessions(nextSessions)
      })
      .catch((reason) => {
        if (active) toast.error((reason as Error).message)
      })
    return () => {
      active = false
    }
  }, [fetchSessions])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (newPassword.length < 8) return toast.error("新密码至少需要 8 个字符")
    setSaving(true)
    try {
      await changePassword(currentPassword, newPassword)
      setCurrentPassword("")
      setNewPassword("")
      await loadSessions()
      toast.success("密码已修改，其他设备已退出")
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setSaving(false)
    }
  }
  async function logoutOthers() {
    try {
      await logoutOtherSessions()
      await loadSessions()
      toast.success("其他设备已退出")
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <Card id="change-password" className="scroll-mt-24 shadow-xs">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <KeyRoundIcon className="size-4" />
            修改密码
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            保留当前会话，退出其他设备。
          </p>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="current-password">当前密码</FieldLabel>
                <Input
                  id="current-password"
                  type="password"
                  autoComplete="current-password"
                  value={currentPassword}
                  onChange={(event) => setCurrentPassword(event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="new-password">新密码</FieldLabel>
                <Input
                  id="new-password"
                  type="password"
                  autoComplete="new-password"
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                />
              </Field>
              <Button className="w-full" type="submit" disabled={saving}>
                {saving && <LoaderCircleIcon className="animate-spin" />}
                {saving ? "保存中…" : "更新密码"}
              </Button>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      <Card className="shadow-xs">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <LaptopIcon className="size-4" />
            登录会话
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            查看当前账户的有效登录。
          </p>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {sessions.map((session) => (
              <div key={session.id} className="rounded-lg border p-3">
                <p className="flex items-center gap-2 text-sm font-medium">
                  <LaptopIcon className="size-4" />
                  {session.browser || "未知浏览器"} ·
                  {session.deviceType || "未知设备"}
                  {session.current && (
                    <span className="text-primary">当前</span>
                  )}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {session.os || "未知系统"} · {session.ipLabel || "未知网络"} ·
                  最近活动{" "}
                  {new Date(
                    session.lastActiveAt || session.createdAt
                  ).toLocaleString("zh-CN")}
                </p>
              </div>
            ))}
            {!sessions.length && (
              <p className="text-sm text-muted-foreground">暂无有效会话。</p>
            )}
          </div>
          <Button
            className="mt-4 w-full"
            variant="outline"
            onClick={() => void logoutOthers()}
          >
            <LogOutIcon />
            退出其他设备
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
