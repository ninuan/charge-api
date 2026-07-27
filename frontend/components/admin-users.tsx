import { Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { AdminUserDiagnostics } from "@/components/admin-user-diagnostics"
import { AdminUserFilters } from "@/components/admin-user-filters"
import { AdminUserRefreshTiming } from "@/components/admin-user-refresh-timing"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { adminApi } from "@/lib/admin-api"
import type {
  AdminUserSummary,
  AdminUserListQuery,
  AdminUserPage,
  CredentialState,
} from "@/lib/types"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

const credentialText: Record<CredentialState, string> = {
  unbound: "未绑定扫码",
  waiting_device: "等待添加设备",
  healthy: "凭据正常",
  sync_failed: "凭据同步失败",
  expired: "凭据已失效",
}

function credentialVariant(summary: AdminUserSummary) {
  return summary.user.role === "admin" ||
    summary.dashboard.pileCount === 0 ||
    summary.credential.state === "healthy" ||
    summary.credential.state === "waiting_device"
    ? "secondary"
    : "destructive"
}

function credentialLabel(summary: AdminUserSummary) {
  if (summary.user.role === "admin") return "无需扫码"
  if (summary.dashboard.pileCount === 0) return "待添加设备"
  return credentialText[summary.credential.state]
}

function hasRisk(summary: AdminUserSummary) {
  if (!summary.user.enabled) return true
  if (summary.user.role === "admin") return false

  return (
    (summary.dashboard.pileCount > 0 &&
      ["unbound", "sync_failed", "expired"].includes(
        summary.credential.state
      )) ||
    summary.lastRefresh.failedDevices > 0 ||
    summary.dashboard.offlinePorts > 0
  )
}

type AdminUsersProps = {
  page: AdminUserPage | null
  query: AdminUserListQuery
  load: (query?: AdminUserListQuery) => Promise<void>
  currentUserId?: string
}

export function AdminUsers({
  page,
  query,
  load,
  currentUserId,
}: AdminUsersProps) {
  // 开关与目标分离：关闭时保留 deleteTarget，退出动画期间标题不会闪空。
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string
    username: string
  } | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  async function update(
    id: string,
    payload: Parameters<typeof adminApi.updateUser>[1]
  ) {
    try {
      await adminApi.updateUser(id, payload)
      toast.success("用户已更新")
      await load()
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await adminApi.removeUser(deleteTarget.id)
      toast.success(`用户 ${deleteTarget.username} 已删除`)
      setDeleteOpen(false)
      await load()
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setDeleting(false)
    }
  }

  function renderStatus(summary: AdminUserSummary) {
    const risk = hasRisk(summary)

    return (
      <div className="flex flex-wrap items-center gap-1.5">
        <Badge variant={summary.user.enabled ? "secondary" : "destructive"}>
          {summary.user.enabled ? "已启用" : "已停用"}
        </Badge>
        <Badge variant={credentialVariant(summary)}>
          {credentialLabel(summary)}
        </Badge>
        {risk && <Badge variant="destructive">需要关注</Badge>}
      </div>
    )
  }

  function renderActions(summary: AdminUserSummary) {
    const isCurrent = summary.user.id === currentUserId

    if (isCurrent) {
      return <p className="text-xs text-muted-foreground">当前账户受保护</p>
    }

    return (
      <div className="flex flex-wrap gap-2 md:justify-end">
        <Button
          size="sm"
          variant="outline"
          aria-label={`${summary.user.enabled ? "停用" : "启用"}用户 ${summary.user.username}`}
          onClick={() =>
            void update(summary.user.id, {
              enabled: !summary.user.enabled,
            })
          }
        >
          {summary.user.enabled ? "停用" : "启用"}
        </Button>
        {summary.user.role === "user" && (
          <Button
            size="sm"
            variant="outline"
            aria-label={`${summary.user.refreshEnabled ? "关闭" : "开启"}用户 ${summary.user.username} 的设备刷新`}
            onClick={() =>
              void update(summary.user.id, {
                refreshEnabled: !summary.user.refreshEnabled,
              })
            }
          >
            {summary.user.refreshEnabled ? "关闭刷新" : "开启刷新"}
          </Button>
        )}
        <Button
          size="sm"
          variant="destructive"
          aria-label={`删除用户 ${summary.user.username}`}
          onClick={() => {
            setDeleteTarget({
              id: summary.user.id,
              username: summary.user.username,
            })
            setDeleteOpen(true)
          }}
        >
          <Trash2Icon />
          删除
        </Button>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <AdminUserFilters query={query} onApply={load} />
      <div className="flex items-center justify-between px-1">
        <p className="text-xs text-muted-foreground">
          {page ? `共找到 ${page.total} 位用户` : "正在加载用户目录"}
        </p>
        <p className="text-xs text-muted-foreground">
          风险账户包含凭据异常、刷新失败、离线端口或已停用账户。
        </p>
      </div>
      <div className="grid gap-3 md:hidden">
        {!page && (
          <>
            <Skeleton className="h-36 w-full" />
            <Skeleton className="h-36 w-full" />
            <Skeleton className="h-36 w-full" />
          </>
        )}
        {page?.items.map((summary) => {
          const isCurrent = summary.user.id === currentUserId

          return (
            <Card key={summary.user.id} className="shadow-xs">
              <CardContent className="flex flex-col gap-4 p-4">
                <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-medium">{summary.user.username}</p>
                      {isCurrent && <Badge>当前账号</Badge>}
                      {renderStatus(summary)}
                    </div>
                    <p className="mt-2 text-xs leading-5 text-muted-foreground">
                      {summary.user.role === "admin"
                        ? "管理员账户 · 不参与扫码凭据和设备刷新"
                        : `普通用户 · 设备额度 ${summary.user.deviceLimit} · 已添加 ${summary.dashboard.pileCount} 台设备 · 离线端口 ${summary.dashboard.offlinePorts} 个`}
                    </p>
                    {summary.user.role === "user" && (
                      <AdminUserRefreshTiming
                        bound={summary.credential.bound}
                        lastCheckedAt={summary.credential.lastCheckedAt}
                        lastRemoteAt={summary.lastRefresh.lastRemoteAt}
                      />
                    )}
                  </div>
                  {renderActions(summary)}
                </div>
                {summary.user.role === "user" && (
                  <AdminUserDiagnostics
                    diagnostics={summary.recoveryDiagnostics ?? []}
                  />
                )}
              </CardContent>
            </Card>
          )
        })}
        {page && !page.items.length && (
          <Card className="border-dashed shadow-xs">
            <CardContent className="p-10 text-center text-sm text-muted-foreground">
              没有匹配的用户。可以重置筛选后重新查看。
            </CardContent>
          </Card>
        )}
      </div>
      <Card className="hidden shadow-xs md:block">
        <CardContent className="p-0">
          <Table aria-label="用户目录">
            <TableHeader>
              <TableRow>
                <TableHead>账户</TableHead>
                <TableHead>角色与设备</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>刷新与诊断</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!page &&
                Array.from({ length: 5 }, (_, index) => (
                  <TableRow key={index}>
                    <TableCell colSpan={5}>
                      <Skeleton className="h-10 w-full" />
                    </TableCell>
                  </TableRow>
                ))}
              {page?.items.map((summary) => {
                const isCurrent = summary.user.id === currentUserId

                return (
                  <TableRow key={summary.user.id}>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <span className="font-medium">
                          {summary.user.username}
                        </span>
                        {isCurrent && <Badge>当前账号</Badge>}
                      </div>
                    </TableCell>
                    <TableCell>
                      {summary.user.role === "admin" ? (
                        <div>
                          <p>管理员</p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            不参与设备刷新
                          </p>
                        </div>
                      ) : (
                        <div>
                          <p>
                            {summary.dashboard.pileCount}/
                            {summary.user.deviceLimit} 台设备
                          </p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            离线端口 {summary.dashboard.offlinePorts} 个
                          </p>
                        </div>
                      )}
                    </TableCell>
                    <TableCell>{renderStatus(summary)}</TableCell>
                    <TableCell className="max-w-64">
                      {summary.user.role === "admin" ? (
                        <p className="text-xs text-muted-foreground">
                          无需扫码凭据
                        </p>
                      ) : (
                        <>
                          <AdminUserRefreshTiming
                            bound={summary.credential.bound}
                            lastCheckedAt={summary.credential.lastCheckedAt}
                            lastRemoteAt={summary.lastRefresh.lastRemoteAt}
                          />
                          <AdminUserDiagnostics
                            diagnostics={summary.recoveryDiagnostics ?? []}
                          />
                        </>
                      )}
                    </TableCell>
                    <TableCell>{renderActions(summary)}</TableCell>
                  </TableRow>
                )
              })}
              {page && !page.items.length && (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className="h-28 text-center text-muted-foreground"
                  >
                    没有匹配的用户。可以重置筛选后重新查看。
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {page && (
        <div className="flex items-center justify-between">
          <p className="text-xs text-muted-foreground">
            第 {page.page}/{page.totalPages} 页
          </p>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={page.page <= 1}
              onClick={() => void load({ ...query, page: page.page - 1 })}
            >
              上一页
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={page.page >= page.totalPages}
              onClick={() => void load({ ...query, page: page.page + 1 })}
            >
              下一页
            </Button>
          </div>
        </div>
      )}
      <Dialog
        open={deleteOpen}
        onOpenChange={(next) => {
          if (!next && !deleting) setDeleteOpen(false)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除用户“{deleteTarget?.username}”？</DialogTitle>
            <DialogDescription>
              该账户及其绑定的扫码凭据、已添加的充电桩记录都会被移除，操作无法撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={deleting}
              onClick={() => setDeleteOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={deleting}
              onClick={() => void confirmDelete()}
            >
              <Trash2Icon />
              {deleting ? "正在删除…" : "确认删除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
