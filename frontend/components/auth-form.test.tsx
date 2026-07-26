import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AuthProvider } from "@/lib/auth-context"
import { AuthForm } from "@/components/auth-form"

describe("AuthForm", () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it("offers a retry after the security config fails to load", async () => {
    const onSuccess = vi.fn()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response("upstream boom", { status: 502 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ turnstileEnabled: false }), {
          status: 200,
        })
      )
    vi.stubGlobal("fetch", fetchMock)
    const user = userEvent.setup()

    render(
      <AuthProvider>
        <AuthForm mode="login" onSuccess={onSuccess} />
      </AuthProvider>
    )

    expect(await screen.findByText("安全配置加载失败")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "登录" })).toBeDisabled()

    await user.click(screen.getByRole("button", { name: "重新加载" }))

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "登录" })).toBeEnabled()
    )
    expect(screen.queryByText("安全配置加载失败")).not.toBeInTheDocument()
  })

  it("loads public security settings and submits the unchanged login fields", async () => {
    const onSuccess = vi.fn()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ turnstileEnabled: false }), {
          status: 200,
        })
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "u1",
            username: "alice",
            role: "user",
            enabled: true,
          }),
          { status: 200 }
        )
      )
    vi.stubGlobal("fetch", fetchMock)
    const user = userEvent.setup()

    render(
      <AuthProvider>
        <AuthForm mode="login" onSuccess={onSuccess} />
      </AuthProvider>
    )

    await user.type(await screen.findByLabelText("用户名"), "alice")
    await user.type(screen.getByLabelText("密码"), "secret")
    await user.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith("/dashboard"))
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/auth/config", {
      cache: "no-store",
      credentials: "include",
      signal: expect.any(AbortSignal),
    })
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/auth/login",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({
          username: "alice",
          password: "secret",
          captchaToken: "",
        }),
      })
    )
  })

  it("loads the existing registration image challenge and submits its answer unchanged", async () => {
    const onSuccess = vi.fn()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            turnstileEnabled: false,
            registerCaptchaEnabled: true,
          }),
          { status: 200 }
        )
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ id: "captcha-1", image: "data:image/svg+xml,test" }),
          { status: 200 }
        )
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "u1",
            username: "alice",
            role: "user",
            enabled: true,
          }),
          { status: 200 }
        )
      )
    vi.stubGlobal("fetch", fetchMock)
    const user = userEvent.setup()

    render(
      <AuthProvider>
        <AuthForm mode="register" onSuccess={onSuccess} />
      </AuthProvider>
    )

    await user.type(await screen.findByLabelText("用户名"), "alice")
    await user.type(screen.getByLabelText("密码"), "password123")
    await user.type(screen.getByLabelText("图片验证码"), "1234")
    await user.click(screen.getByRole("button", { name: "注册并进入" }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith("/dashboard"))
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/auth/register-captcha", {
      credentials: "include",
      cache: "no-store",
      signal: expect.any(AbortSignal),
    })
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/auth/register",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          username: "alice",
          password: "password123",
          captchaToken: "",
          captchaId: "captcha-1",
          captchaAnswer: "1234",
          inviteCode: "",
        }),
      })
    )
  })

  it("does not expose an internal error when loading the registration image challenge fails", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            turnstileEnabled: false,
            registerCaptchaEnabled: true,
          }),
          { status: 200 }
        )
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            error: "dial internal-captcha:8443 with token=secret",
          }),
          { status: 502 }
        )
      )
    vi.stubGlobal("fetch", fetchMock)

    render(
      <AuthProvider>
        <AuthForm mode="register" onSuccess={vi.fn()} />
      </AuthProvider>
    )

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "验证码加载失败，请稍后重试。"
    )
    expect(
      screen.queryByText("dial internal-captcha:8443 with token=secret")
    ).not.toBeInTheDocument()
  })

  it("hides the optional invite field while public registration is open", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({
              turnstileEnabled: false,
              registrationOpen: true,
              inviteRequired: true,
            }),
            { status: 200 }
          )
        )
    )

    render(
      <AuthProvider>
        <AuthForm mode="register" onSuccess={vi.fn()} />
      </AuthProvider>
    )

    await screen.findByLabelText("用户名")
    expect(screen.queryByLabelText("邀请码（选填）")).not.toBeInTheDocument()
  })
})
