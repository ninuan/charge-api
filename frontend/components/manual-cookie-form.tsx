"use client"

import { useId, useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useDashboard } from "@/lib/dashboard-context"
import { useOnline } from "@/lib/browser-state"

export function ManualCookieForm({ onSaved }: { onSaved?: () => void }) {
  const { updateCookie } = useDashboard()
  const id = useId()
  const online = useOnline()
  const lock = useRef(false)
  const [cookie, setCookie] = useState("")
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState("")
  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (lock.current || !cookie.trim() || !navigator.onLine) return
    lock.current = true
    setBusy(true)
    setMessage("")
    try {
      await updateCookie(cookie.trim())
      setCookie("")
      setMessage("Cookie 已更新")
      onSaved?.()
    } catch (reason) {
      setMessage(
        reason instanceof Error ? reason.message : "更新失败，请重试。"
      )
    } finally {
      lock.current = false
      setBusy(false)
    }
  }
  return (
    <form onSubmit={save} className="space-y-3 border-t pt-4">
      <label htmlFor={id} className="text-sm font-medium">
        手动更新 Cookie
      </label>
      <Input
        id={id}
        type="password"
        autoComplete="off"
        value={cookie}
        onChange={(event) => setCookie(event.target.value)}
        placeholder="粘贴 Cookie"
      />
      <Button type="submit" disabled={!online || busy || !cookie.trim()}>
        {busy ? "更新中…" : "更新 Cookie"}
      </Button>
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
    </form>
  )
}
