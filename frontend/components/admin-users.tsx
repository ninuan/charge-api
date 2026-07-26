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
  AdminUserListQuery,
  AdminUserPage,
  CredentialState,
} from "@/lib/types"

const credentialText: Record<CredentialState, string> = {
  unbound: "未绑定扫码",
  waiting_device: "等待添加设备",
  healthy: "凭据正常",
  sync_failed: "凭据同步失败",
  expired: "凭据已失效",
}

function credentialVariant(state: CredentialState) {
  return state === "healthy" || state === "waiting_device"
    ? "secondary"
    : "destructive"
}

type AdminUsersProps = {
  page: AdminUserPage | null
  query: AdminUserListQuery
  load: (query?: AdminUserListQuery) => Promise<void>
}

export function AdminUsers({ page, query, load }: AdminUsersProps) {
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
      <div className="grid gap-3">
        {!page && (
          <>
            <Skeleton className="h-36 w-full" />
            <Skeleton className="h-36 w-full" />
            <Skeleton className="h-36 w-full" />
          </>
        )}
        {page?.items.map((summary) => {
          const risk =
            !summary.user.enabled ||
            summary.credential.state === "unbound" ||
            summary.credential.state === "sync_failed" ||
            summary.credential.state === "expired" ||
            summary.lastRefresh.failedDevices > 0 ||
            summary.dashboard.offlinePorts > 0

          return (
            <Card key={summary.user.id} className="shadow-none">
              <CardContent className="flex flex-col gap-4 p-4">
                <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-medium">{summary.user.username}</p>
                      <Badge
                        variant={
                          summary.user.enabled ? "secondary" : "destructive"
                        }
                      >
                        {summary.user.enabled ? "已启用" : "已停用"}
                      </Badge>
                      <Badge
                        variant={credentialVariant(summary.credential.state)}
                      >
                        {credentialText[summary.credential.state]}
                      </Badge>
                      {risk && <Badge variant="destructive">需要关注</Badge>}
                    </div>
                    <p className="mt-2 text-xs leading-5 text-muted-foreground">
                      {summary.user.role === "admin"
                        ? "管理员账户"
                        : "普通用户"}{" "}
                      · 设备额度 {summary.user.deviceLimit} · 已添加{" "}
                      {summary.dashboard.pileCount} 台设备 · 离线端口{" "}
                      {summary.dashboard.offlinePorts} 个
                    </p>
                    <AdminUserRefreshTiming
                      bound={summary.credential.bound}
                      lastCheckedAt={summary.credential.lastCheckedAt}
                      lastRemoteAt={summary.lastRefresh.lastRemoteAt}
                    />
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        void update(summary.user.id, {
                          enabled: !summary.user.enabled,
                        })
                      }
                    >
                      {summary.user.enabled ? "停用" : "启用"}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        void update(summary.user.id, {
                          refreshEnabled: !summary.user.refreshEnabled,
                        })
                      }
                    >
                      {summary.user.refreshEnabled ? "关闭刷新" : "开启刷新"}
                    </Button>
                    <Button
                      size="sm"
                      variant="destructive"
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
                </div>
                <AdminUserDiagnostics
                  diagnostics={summary.recoveryDiagnostics ?? []}
                />
              </CardContent>
            </Card>
          )
        })}
        {page && !page.items.length && (
          <Card className="border-dashed shadow-none">
            <CardContent className="p-10 text-center text-sm text-muted-foreground">
              没有匹配的用户。可以重置筛选后重新查看。
            </CardContent>
          </Card>
        )}
      </div>
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
