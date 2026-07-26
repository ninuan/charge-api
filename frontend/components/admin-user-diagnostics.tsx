"use client"

import { useState } from "react"

import { Button } from "@/components/ui/button"
import type { RecoveryDiagnostic } from "@/lib/types"

const actionableCodes = new Set([
  "pile_identifier_required",
  "pile_id_invalid",
  "pile_number_invalid",
  "pile_fields_invalid",
  "pile_port_count_invalid",
  "add_pile_failed",
  "pile_update_failed",
  "pile_delete_failed",
  "refresh_failed",
  "cookie_required",
  "cookie_too_large",
  "cookie_update_failed",
  "qr_create_failed",
  "qr_poll_failed",
  "qr_confirm_failed",
  "qr_session_invalid",
  "scan_service_unavailable",
  "binding_save_failed",
  "credential_sync_failed",
  "device_id_invalid",
  "auth_rate_limited",
  "recovery_unavailable",
  "binding_missing",
  "yyb_get_code_failed",
  "yyb_account_refresh_failed",
  "yyb_get_code_retry_failed",
  "mocele_autologin_missing_info",
  "mocele_autologin_missing_wxopenid",
  "mocele_autologin_failed",
  "new_cookie_validation_failed",
  "recovery_failed",
])

const operationLabel: Record<RecoveryDiagnostic["operation"], string> = {
  credential_recovery: "自动恢复",
  add_pile: "添加充电桩",
  refresh: "刷新状态",
  update_cookie: "更新凭据",
  scan_login: "扫码登录",
  sync_cookie: "同步凭据",
  auth_protection: "登录防护",
}

export function AdminUserDiagnostics({
  diagnostics,
}: {
  diagnostics: RecoveryDiagnostic[]
}) {
  const [expanded, setExpanded] = useState(false)
  const failures = diagnostics.filter((item) => actionableCodes.has(item.code))

  if (!failures.length) return null

  return (
    <div className="border-t pt-3">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={() => setExpanded((value) => !value)}
      >
        {expanded
          ? `收起诊断（${failures.length}）`
          : `查看诊断（${failures.length}）`}
      </Button>
      {expanded ? (
        <ul className="mt-2 flex flex-col gap-2">
          {failures.map((item) => (
            <li
              key={`${item.code}-${item.at}`}
              className="rounded-md bg-muted/40 p-2 text-xs"
            >
              <p className="font-medium">{item.message}</p>
              <p className="mt-1 text-muted-foreground">
                {operationLabel[item.operation]} ·{" "}
                {new Date(item.at).toLocaleString("zh-CN")}
                {item.deviceSuffix ? ` · 设备尾号 ${item.deviceSuffix}` : ""}
                {item.statusCode ? ` · 状态码 ${item.statusCode}` : ""}
              </p>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
