"use client"

import { CheckCircle2Icon, Link2OffIcon, LoaderCircleIcon, QrCodeIcon, RefreshCwIcon, ShieldCheckIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useDashboard } from "@/lib/dashboard-context"
import { requestEmpty, requestJSON } from "@/lib/http"

type Binding = { bound: boolean; openidSuffix?: string; nickname?: string; message?: string; cookieSynced?: boolean }
type QR = { sessionId: string; imageUrl?: string; imageBase64?: string }
type Poll = { sessionId: string; status: string; message?: string }

export function YybLoginDialog() {
  const { updateCookie } = useDashboard()
  const [open, setOpen] = useState(false)
  const [advanced, setAdvanced] = useState(false)
  const [binding, setBinding] = useState<Binding>({ bound: false })
  const [qr, setQr] = useState<QR | null>(null)
  const [poll, setPoll] = useState<Poll | null>(null)
  const [loading, setLoading] = useState(false)
  const [polling, setPolling] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [cookie, setCookie] = useState("")

  useEffect(() => { if (open) void requestJSON<Binding>("/api/session/yyb-binding", {}, "暂时无法读取扫码绑定状态，请稍后重试。").then(setBinding).catch((reason) => toast.error(reason.message)) }, [open])
  const status = binding.bound ? `${binding.nickname || "微信账号"} 已绑定${binding.openidSuffix ? `（尾号 ${binding.openidSuffix}）` : ""}` : poll?.message || ({ pending: "等待扫码，请使用微信扫描左侧二维码。", scanned: "已扫码，请在微信中确认登录。", authorized: "扫码已确认，可以点击确认绑定。", confirmed: "扫码已确认，可以点击确认绑定。", expired: "二维码已过期，请重新生成。", cancelled: "扫码已取消，请重新生成二维码。" }[poll?.status ?? ""] || (qr ? "请使用微信扫码，扫码完成后点击确认。" : "生成二维码后，扫码结果会通过 Charge 后端确认并保存到当前账户。"))

  async function createQr() { setLoading(true); try { const next = await requestJSON<QR>("/api/session/yyb-qr", { method: "POST" }, "二维码暂时无法生成，请稍后重试。"); setQr(next); setPoll(null); toast.success("二维码已生成，扫码后请点击检查扫码状态") } catch (reason) { toast.error((reason as Error).message) } finally { setLoading(false) } }
  async function checkStatus() { if (!qr) return; setPolling(true); try { const next = await requestJSON<Poll>(`/api/session/yyb-qr/${encodeURIComponent(qr.sessionId)}/poll`, {}, "暂时无法获取扫码状态，请稍后重试。"); setPoll(next); toast.success(next.message || "扫码状态已更新") } catch (reason) { toast.error((reason as Error).message) } finally { setPolling(false) } }
  async function confirm() { if (!qr) return; setConfirming(true); try { const next = await requestJSON<Binding>(`/api/session/yyb-qr/${encodeURIComponent(qr.sessionId)}/confirm`, { method: "POST" }, "暂时无法确认扫码结果，请稍后重试。"); setBinding(next); setQr(null); setPoll(null); toast.success(next.message || (next.cookieSynced ? "扫码登录已生效" : "扫码登录已完成")) } catch (reason) { toast.error((reason as Error).message) } finally { setConfirming(false) } }
  async function clearBinding() { try { await requestEmpty("/api/session/yyb-binding", { method: "DELETE" }, "解除扫码绑定失败，请稍后重试。"); setBinding({ bound: false }); setQr(null); setPoll(null); toast.success("扫码绑定已解除") } catch (reason) { toast.error((reason as Error).message) } }
  async function saveCookie(event: React.FormEvent) { event.preventDefault(); try { await updateCookie(cookie); setCookie(""); toast.success("Cookie 已更新") } catch (reason) { toast.error((reason as Error).message) } }

  return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger render={<Button variant="outline"><QrCodeIcon />扫码登录</Button>} /><DialogContent className="max-h-[calc(100dvh-2rem)] w-[min(48rem,calc(100%-2rem))] max-w-none overflow-y-auto sm:max-w-none"><DialogHeader><DialogTitle className="flex items-center gap-2"><ShieldCheckIcon className="size-5" />扫码登录远端账号</DialogTitle><DialogDescription>微信扫码后会保存当前账户的登录绑定；以后添加充电桩时会自动维护访问凭据。</DialogDescription></DialogHeader><Alert><ShieldCheckIcon /><AlertTitle>手机端使用提醒</AlertTitle><AlertDescription>二维码仅支持微信扫一扫摄像头识别，不能通过截图、长按图片或相册读取。手机端请在另一台设备打开本页面，再用当前手机微信扫码。</AlertDescription></Alert><div className="grid gap-5 sm:grid-cols-2"><section className="rounded-lg border bg-muted/30 p-4 text-center">{qr?.imageBase64 || qr?.imageUrl ? <img src={qr.imageBase64 || qr.imageUrl} alt="微信扫码登录二维码" className="mx-auto aspect-square w-full max-w-56 rounded bg-white p-2" /> : <div className="grid aspect-square w-full place-items-center text-muted-foreground"><div><QrCodeIcon className="mx-auto size-10" /><p className="mt-2 text-sm">二维码尚未生成</p></div></div>}<Button className="mt-4 w-full" disabled={loading} onClick={() => void createQr()}>{loading ? <LoaderCircleIcon className="animate-spin" /> : <QrCodeIcon />}{loading ? "生成中…" : "生成扫码二维码"}</Button></section><section className="space-y-3"><CardStatus bound={binding.bound} status={status} /><div className="grid gap-2"><Button variant="outline" disabled={!qr || polling} onClick={() => void checkStatus()}>{polling ? <LoaderCircleIcon className="animate-spin" /> : <RefreshCwIcon />}检查扫码状态</Button><Button disabled={!qr || confirming} onClick={() => void confirm()}>{confirming ? <LoaderCircleIcon className="animate-spin" /> : <CheckCircle2Icon />}确认绑定</Button></div><p className="rounded-lg bg-muted p-3 text-xs leading-5 text-muted-foreground">确认绑定后会立即尝试同步凭据；如果还没有设备，后续通过“添加充电桩”入口添加时会自动生效。</p><Button variant="ghost" className="px-0" onClick={() => setAdvanced((value) => !value)}>{advanced ? "收起高级设置" : "高级设置：手动 Cookie 与解绑"}</Button>{advanced && <div className="space-y-3 rounded-lg border p-3"><form onSubmit={saveCookie}><FieldGroup><Field><FieldLabel htmlFor="manual-cookie">手动更新 Cookie</FieldLabel><Input id="manual-cookie" value={cookie} onChange={(event) => setCookie(event.target.value)} placeholder="粘贴 Cookie" /></Field><Button className="w-full" type="submit">更新 Cookie</Button></FieldGroup></form><Button variant="destructive" className="w-full" disabled={!binding.bound} onClick={() => void clearBinding()}><Link2OffIcon />解除扫码绑定</Button></div>}</section></div><DialogFooter><Button variant="outline" onClick={() => setOpen(false)}>关闭</Button></DialogFooter></DialogContent></Dialog>
}

function CardStatus({ bound, status }: { bound: boolean; status: string }) { return <div className="rounded-lg border p-3"><div className="flex gap-2"><>{bound ? <CheckCircle2Icon className="mt-0.5 size-5" /> : <ShieldCheckIcon className="mt-0.5 size-5 text-muted-foreground" />}</><div><p className="text-sm font-semibold">{bound ? "已绑定扫码登录" : "尚未绑定"}</p><p className="mt-1 text-xs leading-5 text-muted-foreground">{status}</p></div></div></div> }
