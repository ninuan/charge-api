"use client"

import { KeyRoundIcon, LaptopIcon, LoaderCircleIcon, LogOutIcon, ShieldCheckIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/lib/auth-context"
import type { SessionView } from "@/lib/types"

export function SecurityDialog() {
  const { changePassword, fetchSessions, logoutOtherSessions } = useAuth()
  const [open, setOpen] = useState(false)
  const [sessions, setSessions] = useState<SessionView[]>([])
  const [currentPassword, setCurrentPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [saving, setSaving] = useState(false)
  const loadSessions = async () => { try { setSessions(await fetchSessions()) } catch (reason) { toast.error((reason as Error).message) } }
  useEffect(() => { if (open) void loadSessions() }, [open])
  async function submit(event: React.FormEvent) { event.preventDefault(); if (newPassword.length < 8) return toast.error("新密码至少需要 8 个字符"); setSaving(true); try { await changePassword(currentPassword, newPassword); setCurrentPassword(""); setNewPassword(""); await loadSessions(); toast.success("密码已修改，其他设备已退出") } catch (reason) { toast.error((reason as Error).message) } finally { setSaving(false) } }
  async function logoutOthers() { try { await logoutOtherSessions(); await loadSessions(); toast.success("其他设备已退出") } catch (reason) { toast.error((reason as Error).message) } }
  return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger render={<Button variant="outline"><KeyRoundIcon />安全中心</Button>} /><DialogContent className="max-h-[calc(100dvh-2rem)] max-w-3xl overflow-y-auto"><DialogHeader><div className="flex items-center gap-3"><span className="rounded-lg bg-muted p-2"><ShieldCheckIcon className="size-5" /></span><div><DialogTitle>账户安全</DialogTitle><DialogDescription className="mt-1">管理密码和有效登录会话。</DialogDescription></div></div></DialogHeader><div className="grid gap-6 md:grid-cols-2"><form onSubmit={submit}><FieldGroup><div><h3 className="flex items-center gap-2 text-sm font-semibold"><KeyRoundIcon className="size-4" />修改密码</h3><p className="mt-1 text-xs text-muted-foreground">保留当前会话，退出其他设备。</p></div><Field><FieldLabel htmlFor="current-password">当前密码</FieldLabel><Input id="current-password" type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} /></Field><Field><FieldLabel htmlFor="new-password">新密码</FieldLabel><Input id="new-password" type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} /></Field><Button className="w-full" type="submit" disabled={saving}>{saving && <LoaderCircleIcon className="animate-spin" />}{saving ? "保存中…" : "更新密码"}</Button></FieldGroup></form><section className="border-t pt-6 md:border-t-0 md:border-l md:pt-0 md:pl-6"><h3 className="flex items-center gap-2 text-sm font-semibold"><LaptopIcon className="size-4" />登录会话</h3><p className="mt-1 text-xs text-muted-foreground">查看当前账户的有效登录。</p><div className="mt-4 space-y-2">{sessions.map((session) => <div key={session.id} className="rounded-lg border p-3"><p className="flex items-center gap-2 text-sm font-medium"><LaptopIcon className="size-4" />{session.current ? "当前会话" : "其他会话"}</p><p className="mt-1 text-xs text-muted-foreground">登录于 {new Date(session.createdAt).toLocaleString("zh-CN")}</p></div>)}{!sessions.length && <p className="text-sm text-muted-foreground">暂无有效会话。</p>}</div><Button className="mt-4 w-full" variant="outline" onClick={() => void logoutOthers()}><LogOutIcon />退出其他设备</Button></section></div><p className="border-t pt-4 text-xs text-muted-foreground">登录会话最长保留 7 天。请勿在公共设备上保持登录。</p></DialogContent></Dialog>
}
