type AdminUserRefreshTimingProps = {
  bound: boolean
  lastCheckedAt?: string
  lastRemoteAt?: string
}

function formatTime(value: string) {
  return new Date(value).toLocaleString("zh-CN")
}

export function AdminUserRefreshTiming({ bound, lastCheckedAt, lastRemoteAt }: AdminUserRefreshTimingProps) {
  return <p className="mt-1 flex flex-wrap gap-x-1 text-xs text-muted-foreground">
    <span>{bound ? "已完成扫码绑定" : "尚未完成扫码绑定"}</span>
    {lastCheckedAt ? <span>· 凭据最近检查 {formatTime(lastCheckedAt)}</span> : null}
    {lastRemoteAt ? <span>· 设备最近刷新 {formatTime(lastRemoteAt)}</span> : null}
  </p>
}
