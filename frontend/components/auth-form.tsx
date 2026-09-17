"use client"

import {
  EyeIcon,
  EyeOffIcon,
  LoaderCircleIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
} from "lucide-react"
import { useCallback, useEffect, useState } from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/lib/auth-context"
import { RequestError, requestJSON } from "@/lib/http"
import { resolveHomeRoute } from "@/lib/routing"

type AuthMode = "login" | "register"

type AuthConfig = {
  authConfigVersion?: number
  loginCaptchaEnabled?: boolean
  registerCaptchaEnabled?: boolean
  registrationOpen?: boolean
  inviteRequired?: boolean
}

export function AuthForm({
  mode,
  onSuccess,
}: {
  mode: AuthMode
  onSuccess: (path: string) => void
}) {
  const { login, register } = useAuth()
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [inviteCode, setInviteCode] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)
  const [configError, setConfigError] = useState("")
  const [configLoading, setConfigLoading] = useState(false)
  const [error, setError] = useState("")
  const [loginCaptchaRequired, setLoginCaptchaRequired] = useState(false)
  const [captchaId, setCaptchaId] = useState("")
  const [captchaImage, setCaptchaImage] = useState("")
  const [captchaAnswer, setCaptchaAnswer] = useState("")
  const [captchaLoading, setCaptchaLoading] = useState(false)

  const registrationAvailable =
    (config?.registrationOpen ?? true) || (config?.inviteRequired ?? false)
  const captchaVisible =
    (mode === "register" && config?.registerCaptchaEnabled) ||
    (mode === "login" &&
      loginCaptchaRequired &&
      (config?.loginCaptchaEnabled ?? true))

  const loadCaptcha = useCallback(async () => {
    setCaptchaLoading(true)
    setCaptchaAnswer("")
    try {
      const challenge = await requestJSON<{ id: string; image: string }>(
        "/api/auth/captcha",
        { cache: "no-store" },
        "验证码加载失败，请稍后重试。"
      )
      setCaptchaId(challenge.id)
      setCaptchaImage(challenge.image)
    } catch (reason) {
      setCaptchaId("")
      setCaptchaImage("")
      setError((reason as Error).message)
    } finally {
      setCaptchaLoading(false)
    }
  }, [])

  // 配置加载失败时提交按钮会一直禁用，必须给用户一个重试入口，
  // 所以抽成可重复调用的函数，失败状态与表单校验错误分开展示。
  const loadConfig = useCallback(
    () =>
      requestJSON<AuthConfig>(
        "/api/auth/config",
        { cache: "no-store" },
        "安全配置加载失败，请稍后重试。"
      )
        .then((nextConfig) => {
          setConfig(nextConfig)
          setConfigError("")
          if (mode === "register" && nextConfig.registerCaptchaEnabled)
            void loadCaptcha()
        })
        .catch((reason: Error) => setConfigError(reason.message))
        .finally(() => setConfigLoading(false)),
    [loadCaptcha, mode]
  )

  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) void loadConfig()
    })
    return () => {
      active = false
    }
  }, [loadConfig])

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const normalizedUsername = username.trim()
    setError("")
    if (normalizedUsername.length < 3 || !password)
      return setError("请输入至少 3 位用户名和密码")
    if (mode === "register" && password.length < 8)
      return setError("注册密码至少需要 8 个字符")
    if (mode === "register" && !registrationAvailable)
      return setError("当前未开放自助注册，请联系管理员开通账户。")
    if (
      mode === "register" &&
      config?.inviteRequired &&
      !config.registrationOpen &&
      !inviteCode.trim()
    )
      return setError("请输入邀请码")
    if (
      mode === "register" &&
      config?.inviteRequired &&
      !config.registrationOpen &&
      (config.authConfigVersion ?? 0) < 2
    )
      return setError("系统暂未完成更新，请联系管理员后再使用邀请码注册")
    if (captchaVisible && !captchaAnswer.trim())
      return setError("请输入图片验证码")

    setSubmitting(true)
    try {
      const user =
        mode === "login"
          ? await login(
              normalizedUsername,
              password,
              captchaId,
              captchaAnswer.trim()
            )
          : await register(
              normalizedUsername,
              password,
              captchaId,
              captchaAnswer.trim(),
              inviteCode.trim()
            )
      setPassword("")
      onSuccess(resolveHomeRoute(user.role))
    } catch (reason) {
      const message = (reason as Error).message
      const loginNeedsCaptcha =
        mode === "login" &&
        reason instanceof RequestError &&
        (reason.code === "LOGIN_CAPTCHA_REQUIRED" ||
          reason.code === "LOGIN_CAPTCHA_INVALID")
      if (loginNeedsCaptcha) {
        setLoginCaptchaRequired(true)
        await loadCaptcha()
      }
      setError(
        mode === "register" &&
          inviteCode.trim() &&
          message.includes("未开放注册")
          ? "邀请码已填写，但系统暂未完成更新。请联系管理员后重试"
          : message
      )
      if (mode === "register") await loadCaptcha()
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form className="w-full" onSubmit={submit}>
      <FieldGroup>
        {configError && !config && (
          <Alert>
            <ShieldCheckIcon />
            <AlertTitle>安全配置加载失败</AlertTitle>
            <AlertDescription>
              <p>{configError}</p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="mt-2"
                disabled={configLoading}
                onClick={() => {
                  setConfigLoading(true)
                  void loadConfig()
                }}
              >
                <RefreshCwIcon
                  className={configLoading ? "motion-safe:animate-spin" : ""}
                />
                重新加载
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {mode === "register" && !registrationAvailable && (
          <Alert>
            <ShieldCheckIcon />
            <AlertTitle>自助注册暂未开放</AlertTitle>
            <AlertDescription>请联系管理员为你创建账户。</AlertDescription>
          </Alert>
        )}
        <Field>
          <FieldLabel htmlFor="username">用户名</FieldLabel>
          <Input
            id="username"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            placeholder="请输入用户名"
          />
        </Field>
        {mode === "register" &&
          config?.inviteRequired &&
          !config.registrationOpen && (
            <Field>
              <FieldLabel htmlFor="invite-code">邀请码（必填）</FieldLabel>
              <Input
                id="invite-code"
                autoComplete="off"
                value={inviteCode}
                onChange={(event) => setInviteCode(event.target.value)}
                placeholder="请输入邀请码"
              />
            </Field>
          )}
        <Field>
          <FieldLabel htmlFor="password">密码</FieldLabel>
          <div className="relative">
            <Input
              id="password"
              type={showPassword ? "text" : "password"}
              autoComplete={
                mode === "login" ? "current-password" : "new-password"
              }
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="请输入密码"
              className="pr-11"
            />
            <Button
              className="absolute inset-y-0 right-1 my-auto active:not-aria-[haspopup]:translate-y-0"
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={showPassword ? "隐藏密码" : "显示密码"}
              aria-pressed={showPassword}
              onClick={() => setShowPassword((value) => !value)}
            >
              {showPassword ? <EyeOffIcon /> : <EyeIcon />}
            </Button>
          </div>
          {mode === "register" && (
            <FieldDescription>至少 8 个字符。</FieldDescription>
          )}
        </Field>
        {captchaVisible && (
          <Field>
            <FieldLabel htmlFor="auth-captcha">图片验证码</FieldLabel>
            <div className="grid grid-cols-[minmax(0,1fr)_9.25rem] gap-3">
              <Input
                id="auth-captcha"
                autoComplete="off"
                inputMode="numeric"
                maxLength={5}
                value={captchaAnswer}
                onChange={(event) => setCaptchaAnswer(event.target.value)}
                placeholder="输入验证码"
              />
              <Button
                type="button"
                variant="outline"
                className="h-11 overflow-hidden p-0"
                aria-label="刷新图片验证码"
                disabled={captchaLoading}
                onClick={() => void loadCaptcha()}
              >
                {captchaImage ? (
                  // The captcha is a short-lived data URL returned by the API;
                  // Next Image cannot optimize or cache it.
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={captchaImage}
                    alt="图片验证码，点击可刷新"
                    className="h-full w-full object-contain"
                  />
                ) : (
                  <RefreshCwIcon
                    className={captchaLoading ? "motion-safe:animate-spin" : ""}
                  />
                )}
              </Button>
            </div>
            <FieldDescription>看不清时，点击图片换一张。</FieldDescription>
          </Field>
        )}
        {error && (
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
        )}
        <Button
          className="w-full"
          size="lg"
          type="submit"
          disabled={
            !config ||
            submitting ||
            captchaLoading ||
            (mode === "register" && !registrationAvailable)
          }
        >
          {submitting && (
            <LoaderCircleIcon
              className="motion-safe:animate-spin"
              data-icon="inline-start"
            />
          )}
          {submitting ? "正在处理…" : mode === "login" ? "登录" : "注册并进入"}
        </Button>
      </FieldGroup>
    </form>
  )
}
