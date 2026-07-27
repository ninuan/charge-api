"use client"

import {
  ActivityIcon,
  CheckIcon,
  CopyIcon,
  DatabaseIcon,
  KeyRoundIcon,
  LaptopIcon,
  LoaderCircleIcon,
  PowerIcon,
  RefreshCwIcon,
  ServerIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import { AdminUserDiagnostics } from "@/components/admin-user-diagnostics"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { adminApi } from "@/lib/admin-api"
import type { AdminUserDetail } from "@/lib/types"

const credentialLabels = {
  unbound: "未绑定扫码",
  waiting_device: "等待添加设备",
  healthy: "凭据正常",
  sync_failed: "凭据同步失败",
  expired: "凭据已失效",
} as const

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN") : "暂无记录"
}

function DetailSection({
  icon: Icon,
  title,
  children,
}: {
  icon: typeof ServerIcon
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="rounded-xl border bg-card p-4">
      <h3 className="mb-3 flex items-center gap-2 text-sm font-medium">
        <Icon className="size-4 text-primary" aria-hidden="true" />
        {title}
      </h3>
      {children}
    </section>
  )
}

type AdminUserDetailSheetProps = {
  userId: string | null
  open: boolean
  currentUserId?: string
  onOpenChange: (open: boolean) => void
  onChanged: () => Promise<void>
}

export function AdminUserDetailSheet({
  userId,
  open,
  currentUserId,
  onOpenChange,
  onChanged,
}: AdminUserDetailSheetProps) {
  const [detail, setDetail] = useState<AdminUserDetail | null>(null)
  const [working, setWorking] = useState("")
  const [passwordOpen, setPasswordOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [temporaryPassword, setTemporaryPassword] = useState("")

  async function loadDetail(id: string) {
    try {
      setDetail(await adminApi.userDetail(id))
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  useEffect(() => {
    if (!open || !userId) return
    let ignore = false
    void adminApi
      .userDetail(userId)
      .then((next) => {
        if (!ignore) setDetail(next)
      })
      .catch((reason) => {
        if (!ignore) toast.error((reason as Error).message)
      })
    return () => {
      ignore = true
    }
  }, [open, userId])

  const offlinePorts = useMemo(
    () =>
      detail?.piles.flatMap((pile) =>
        pile.ports
          .filter((port) => port.status === "offline")
          .map((port) => `${pile.name || pile.number} · ${port.id} 号口`)
      ) ?? [],
    [detail]
  )

  async function updateUser(
    action: string,
    payload: Parameters<typeof adminApi.updateUser>[1],
    message: string
  ) {
    if (!userId) return
    setWorking(action)
    try {
      await adminApi.updateUser(userId, payload)
      toast.success(message)
      await Promise.all([loadDetail(userId), onChanged()])
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setWorking("")
    }
  }

  async function refreshUser() {
    if (!userId) return
    setWorking("refresh")
    try {
      await adminApi.refreshUser(userId)
      toast.success("已完成一次强制刷新")
      await Promise.all([loadDetail(userId), onChanged()])
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setWorking("")
    }
  }

  async function resetPassword() {
    if (!userId) return
    setWorking("password")
    try {
      const result = await adminApi.resetUserPassword(userId)
      setTemporaryPassword(result.temporaryPassword)
      toast.success("临时密码已生成，原登录会话已撤销")
      await Promise.all([loadDetail(userId), onChanged()])
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setWorking("")
    }
  }

  async function copyTemporaryPassword() {
    try {
      await navigator.clipboard.writeText(temporaryPassword)
      toast.success("临时密码已复制")
    } catch {
      toast.error("复制失败，请手动选择临时密码")
    }
  }

  async function deleteUser() {
    if (!userId || !detail) return
    setWorking("delete")
    try {
      await adminApi.removeUser(userId)
      toast.success(`用户 ${detail.summary.user.username} 已删除`)
      setDeleteOpen(false)
      onOpenChange(false)
      await onChanged()
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setWorking("")
    }
  }

  const summary = detail?.summary
  const isCurrent = summary?.user.id === currentUserId
  const loading = Boolean(open && userId && detail?.summary.user.id !== userId)

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
          <SheetHeader className="border-b pr-14">
            <SheetTitle className="flex flex-wrap items-center gap-2">
              {summary?.user.username ?? "用户详情"}
              {summary && (
                <>
                  <Badge
                    variant={summary.user.enabled ? "secondary" : "destructive"}
                  >
                    {summary.user.enabled ? "已启用" : "已停用"}
                  </Badge>
                  <Badge variant="outline">
                    {summary.user.role === "admin" ? "管理员" : "普通用户"}
                  </Badge>
                </>
              )}
            </SheetTitle>
            <SheetDescription>
              账户、设备、凭据、诊断和登录会话集中视图。所有管理操作都会进入审计日志。
            </SheetDescription>
          </SheetHeader>

          <div className="grid gap-3 px-4 pb-6">
            {loading && !detail ? (
              <>
                <Skeleton className="h-28 w-full" />
                <Skeleton className="h-40 w-full" />
                <Skeleton className="h-36 w-full" />
              </>
            ) : summary ? (
              <>
                <div className="grid grid-cols-2 gap-2">
                  <div className="rounded-xl border bg-muted/25 p-3">
                    <p className="text-xs text-muted-foreground">设备额度</p>
                    <p className="mt-1 text-lg font-semibold tabular-nums">
                      {summary.dashboard.pileCount}/{summary.user.deviceLimit}
                    </p>
                  </div>
                  <div className="rounded-xl border bg-muted/25 p-3">
                    <p className="text-xs text-muted-foreground">离线端口</p>
                    <p className="mt-1 text-lg font-semibold tabular-nums">
                      {summary.dashboard.offlinePorts}
                    </p>
                  </div>
                </div>

                <DetailSection icon={ActivityIcon} title="状态与刷新">
                  <dl className="grid gap-2 text-xs sm:grid-cols-2">
                    <div>
                      <dt className="text-muted-foreground">凭据状态</dt>
                      <dd className="mt-1 font-medium">
                        {summary.user.role === "admin"
                          ? "无需扫码"
                          : credentialLabels[summary.credential.state]}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">最近远端刷新</dt>
                      <dd className="mt-1 font-medium">
                        {formatTime(summary.lastRefresh.lastRemoteAt)}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">最近数据时间</dt>
                      <dd className="mt-1 font-medium">
                        {formatTime(summary.snapshotUpdatedAt)}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">自动刷新</dt>
                      <dd className="mt-1 font-medium">
                        {summary.user.refreshEnabled ? "已开启" : "已关闭"}
                      </dd>
                    </div>
                  </dl>
                </DetailSection>

                <DetailSection icon={ServerIcon} title="设备与离线端口">
                  {detail.piles.length ? (
                    <div className="grid gap-2">
                      {detail.piles.map((pile) => (
                        <div
                          key={pile.id}
                          className="flex items-center justify-between gap-3 rounded-lg bg-muted/35 p-2.5"
                        >
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium">
                              {pile.name || pile.number}
                            </p>
                            <p className="mt-0.5 truncate text-xs text-muted-foreground">
                              {pile.address || `设备尾号 ${pile.id.slice(-6)}`}
                            </p>
                          </div>
                          <Badge
                            variant={pile.online ? "secondary" : "destructive"}
                          >
                            {pile.online
                              ? `${pile.openNum} 个端口`
                              : "设备离线"}
                          </Badge>
                        </div>
                      ))}
                      {offlinePorts.length > 0 && (
                        <p className="flex gap-2 rounded-lg bg-warning/10 p-2.5 text-xs text-warning-foreground">
                          <TriangleAlertIcon className="mt-0.5 size-3.5 shrink-0" />
                          {offlinePorts.join("、")}
                        </p>
                      )}
                    </div>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      尚未添加设备。
                    </p>
                  )}
                </DetailSection>

                <DetailSection icon={LaptopIcon} title="当前登录会话">
                  {detail.sessions.length ? (
                    <div className="grid gap-2">
                      {detail.sessions.map((session) => (
                        <div
                          key={session.id}
                          className="rounded-lg bg-muted/35 p-2.5"
                        >
                          <div className="flex items-center justify-between gap-2">
                            <p className="text-sm font-medium">
                              {session.browser} · {session.os}
                            </p>
                            <Badge variant="outline">
                              {session.deviceType}
                            </Badge>
                          </div>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {session.ipLabel} · 最近活动{" "}
                            {formatTime(session.lastActiveAt)}
                          </p>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      当前没有有效登录会话。
                    </p>
                  )}
                </DetailSection>

                <DetailSection icon={DatabaseIcon} title="最近诊断记录">
                  <AdminUserDiagnostics
                    diagnostics={summary.recoveryDiagnostics ?? []}
                  />
                </DetailSection>

                <div className="sticky bottom-0 grid gap-2 rounded-xl border bg-background/95 p-3 shadow-lg backdrop-blur sm:grid-cols-2">
                  <Button
                    variant="outline"
                    disabled={Boolean(working) || isCurrent}
                    onClick={() =>
                      void updateUser(
                        "enabled",
                        { enabled: !summary.user.enabled },
                        summary.user.enabled ? "用户已停用" : "用户已启用"
                      )
                    }
                  >
                    <PowerIcon />
                    {summary.user.enabled ? "停用账户" : "启用账户"}
                  </Button>
                  <Button
                    variant="outline"
                    disabled={Boolean(working) || summary.user.role === "admin"}
                    onClick={() => void refreshUser()}
                  >
                    <RefreshCwIcon
                      className={
                        working === "refresh" ? "motion-safe:animate-spin" : ""
                      }
                    />
                    强制刷新
                  </Button>
                  <Button
                    variant="outline"
                    disabled={Boolean(working) || isCurrent}
                    onClick={() => setPasswordOpen(true)}
                  >
                    <KeyRoundIcon />
                    重置密码
                  </Button>
                  <Button
                    variant="destructive"
                    disabled={Boolean(working) || isCurrent}
                    onClick={() => setDeleteOpen(true)}
                  >
                    <Trash2Icon />
                    删除用户
                  </Button>
                </div>
              </>
            ) : (
              <p className="py-10 text-center text-sm text-muted-foreground">
                暂时无法读取用户详情。
              </p>
            )}
          </div>
        </SheetContent>
      </Sheet>

      <Dialog
        open={passwordOpen}
        onOpenChange={(next) => {
          setPasswordOpen(next)
          if (!next) setTemporaryPassword("")
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {temporaryPassword ? "临时密码已生成" : "重置用户密码"}
            </DialogTitle>
            <DialogDescription>
              {temporaryPassword
                ? "临时密码只显示这一次。请安全地交给用户，用户下次登录后会持续收到修改密码提醒。"
                : "系统将生成随机临时密码并撤销该用户所有现有登录会话，操作会写入审计日志。"}
            </DialogDescription>
          </DialogHeader>
          {temporaryPassword && (
            <div className="flex items-center gap-2 rounded-lg border bg-muted/35 p-3">
              <code className="min-w-0 flex-1 font-mono text-sm font-semibold break-all select-all">
                {temporaryPassword}
              </code>
              <Button
                size="icon-sm"
                variant="outline"
                aria-label="复制临时密码"
                onClick={() => void copyTemporaryPassword()}
              >
                <CopyIcon />
              </Button>
            </div>
          )}
          <DialogFooter>
            {temporaryPassword ? (
              <Button onClick={() => setPasswordOpen(false)}>
                <CheckIcon />
                我已保存
              </Button>
            ) : (
              <>
                <Button
                  variant="outline"
                  onClick={() => setPasswordOpen(false)}
                >
                  取消
                </Button>
                <Button
                  disabled={working === "password"}
                  onClick={() => void resetPassword()}
                >
                  {working === "password" && (
                    <LoaderCircleIcon className="animate-spin" />
                  )}
                  生成临时密码
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除用户“{summary?.user.username}”？</DialogTitle>
            <DialogDescription>
              账户、设备、凭据和登录会话都会被删除。审计日志会保留，操作无法撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={working === "delete"}
              onClick={() => void deleteUser()}
            >
              <Trash2Icon />
              {working === "delete" ? "正在删除…" : "确认删除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
