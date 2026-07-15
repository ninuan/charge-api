"use client"

import { EyeIcon, EyeOffIcon, LoaderCircleIcon, RefreshCwIcon, ShieldCheckIcon } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"

import { TurnstileWidget, type TurnstileWidgetHandle } from "@/components/turnstile-widget"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/lib/auth-context"
import { resolveHomeRoute } from "@/lib/routing"

type AuthMode = "login" | "register"

type AuthConfig = {
  turnstileEnabled?: boolean
  turnstileSiteKey?: string
  authConfigVersion?: number
  registerCaptchaEnabled?: boolean
  registrationOpen?: boolean
  inviteRequired?: boolean
}

export function AuthForm({ mode, onSuccess }: { mode: AuthMode; onSuccess: (path: string) => void }) {
  const { login, register } = useAuth()
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [inviteCode, setInviteCode] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)
  const [error, setError] = useState("")
  const [captchaToken, setCaptchaToken] = useState("")
  const [captchaId, setCaptchaId] = useState("")
  const [captchaImage, setCaptchaImage] = useState("")
  const [captchaAnswer, setCaptchaAnswer] = useState("")
  const [captchaLoading, setCaptchaLoading] = useState(false)
  const turnstileRef = useRef<TurnstileWidgetHandle>(null)

  const registrationAvailable = (config?.registrationOpen ?? true) || (config?.inviteRequired ?? false)

  const loadCaptcha = useCallback(async () => {
    if (!config?.registerCaptchaEnabled) return
    setCaptchaLoading(true)
    setCaptchaAnswer("")
    try {
      const response = await fetch("/api/auth/register-captcha", { credentials: "include", cache: "no-store" })
      if (!response.ok) {
        const body = (await response.json().catch(() => ({ error: "验证码加载失败" }))) as { error?: string }
        throw new Error(body.error ?? "验证码加载失败")
      }
      const challenge = (await response.json()) as { id: string; image: string }
      setCaptchaId(challenge.id)
      setCaptchaImage(challenge.image)
    } catch (reason) {
      setCaptchaId("")
      setCaptchaImage("")
      setError((reason as Error).message)
    } finally {
      setCaptchaLoading(false)
    }
  }, [config?.registerCaptchaEnabled])

  const resetTurnstile = useCallback(() => {
    setCaptchaToken("")
    turnstileRef.current?.reset()
  }, [])

  useEffect(() => {
    let active = true
    fetch("/api/auth/config", { cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error("安全配置加载失败")
        return response.json() as Promise<AuthConfig>
      })
      .then((nextConfig) => active && setConfig(nextConfig))
      .catch((reason: Error) => active && setError(reason.message))
    return () => { active = false }
  }, [])

  useEffect(() => {
    resetTurnstile()
    setCaptchaAnswer("")
    if (mode === "register" && config) void loadCaptcha()
  }, [config, loadCaptcha, mode, resetTurnstile])

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const normalizedUsername = username.trim()
    setError("")
    if (normalizedUsername.length < 3 || !password) return setError("请输入至少 3 位用户名和密码")
    if (mode === "register" && password.length < 8) return setError("注册密码至少需要 8 个字符")
    if (mode === "register" && !registrationAvailable) return setError("当前未开放自助注册，请联系管理员开通账户。")
    if (mode === "register" && config?.inviteRequired && !config.registrationOpen && !inviteCode.trim()) return setError("请输入邀请码")
    if (mode === "register" && config?.inviteRequired && !config.registrationOpen && (config.authConfigVersion ?? 0) < 2) return setError("后端服务仍是旧版本，请重启后端服务后再使用邀请码注册")
    if (config?.turnstileEnabled && !captchaToken) return setError("请先完成人机验证")
    if (mode === "register" && config?.registerCaptchaEnabled && !captchaAnswer.trim()) return setError("请输入图片验证码")

    setSubmitting(true)
    try {
      const user = mode === "login"
        ? await login(normalizedUsername, password, captchaToken)
        : await register(normalizedUsername, password, captchaToken, captchaId, captchaAnswer.trim(), inviteCode.trim())
      setPassword("")
      onSuccess(resolveHomeRoute(user.role))
    } catch (reason) {
      const message = (reason as Error).message
      setError(mode === "register" && inviteCode.trim() && message.includes("未开放注册") ? "邀请码已填写，但后端仍在运行旧版本。请重启后端服务后重试" : message)
      if (mode === "register") await loadCaptcha()
    } finally {
      setSubmitting(false)
      resetTurnstile()
    }
  }

  return (
    <form className="w-full" onSubmit={submit}>
      <FieldGroup>
        {mode === "register" && !registrationAvailable && (
          <Alert><ShieldCheckIcon /><AlertTitle>自助注册暂未开放</AlertTitle><AlertDescription>请联系管理员为你创建账户。</AlertDescription></Alert>
        )}
        <Field><FieldLabel htmlFor="username">用户名</FieldLabel><Input id="username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="请输入用户名" /></Field>
        {mode === "register" && config?.inviteRequired && (
          <Field><FieldLabel htmlFor="invite-code">邀请码{!config.registrationOpen ? "（必填）" : "（选填）"}</FieldLabel><Input id="invite-code" autoComplete="off" value={inviteCode} onChange={(event) => setInviteCode(event.target.value)} placeholder="请输入管理员提供的邀请码" /></Field>
        )}
        <Field>
          <FieldLabel htmlFor="password">密码</FieldLabel>
          <div className="relative"><Input id="password" type={showPassword ? "text" : "password"} autoComplete={mode === "login" ? "current-password" : "new-password"} value={password} onChange={(event) => setPassword(event.target.value)} placeholder="请输入密码" className="pr-11" /><Button className="absolute top-1/2 right-1 -translate-y-1/2" type="button" variant="ghost" size="icon-sm" aria-label={showPassword ? "隐藏密码" : "显示密码"} onClick={() => setShowPassword((value) => !value)}>{showPassword ? <EyeOffIcon /> : <EyeIcon />}</Button></div>
          {mode === "register" && <FieldDescription>至少 8 个字符。</FieldDescription>}
        </Field>
        {mode === "register" && config?.registerCaptchaEnabled && (
          <Field>
            <FieldLabel htmlFor="register-captcha">图片验证码</FieldLabel>
            <div className="grid grid-cols-[minmax(0,1fr)_9.25rem] gap-3"><Input id="register-captcha" autoComplete="off" value={captchaAnswer} onChange={(event) => setCaptchaAnswer(event.target.value)} placeholder="输入验证码" /><Button type="button" variant="outline" className="h-9 overflow-hidden p-0" disabled={captchaLoading} onClick={() => void loadCaptcha()}>{captchaImage ? <img src={captchaImage} alt="注册验证码，点击可刷新" className="h-full w-full object-cover" /> : <RefreshCwIcon className={captchaLoading ? "animate-spin" : ""} />}</Button></div>
          </Field>
        )}
        {config?.turnstileEnabled && config.turnstileSiteKey && <TurnstileWidget ref={turnstileRef} siteKey={config.turnstileSiteKey} action={mode} onVerified={setCaptchaToken} onExpired={() => setCaptchaToken("")} onError={() => setCaptchaToken("")} />}
        {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
        <Button className="w-full" size="lg" type="submit" disabled={!config || submitting || captchaLoading || (mode === "register" && !registrationAvailable) || (config?.turnstileEnabled && !captchaToken)}>{submitting && <LoaderCircleIcon className="animate-spin" data-icon="inline-start" />}{submitting ? "正在处理…" : mode === "login" ? "登录" : "注册并进入"}</Button>
      </FieldGroup>
    </form>
  )
}
