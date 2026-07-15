"use client"

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react"

import { requestEmpty, requestJSON } from "@/lib/http"
import type { CurrentUser, SessionView } from "@/lib/types"

type AuthContextValue = {
  currentUser: CurrentUser | null
  loading: boolean
  ready: boolean
  isAdmin: boolean
  clearSession: () => void
  fetchMe: () => Promise<CurrentUser | null>
  login: (username: string, password: string, captchaToken: string) => Promise<CurrentUser>
  register: (username: string, password: string, captchaToken: string, captchaId: string, captchaAnswer: string, inviteCode: string) => Promise<CurrentUser>
  logout: () => Promise<void>
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>
  acknowledgeUsageGuide: () => Promise<CurrentUser>
  fetchSessions: () => Promise<SessionView[]>
  logoutOtherSessions: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null)
  const [loading, setLoading] = useState(false)
  const [ready, setReady] = useState(false)

  const clearSession = useCallback(() => {
    setCurrentUser(null)
    setReady(true)
  }, [])

  const fetchMe = useCallback(async () => {
    setLoading(true)
    try {
      const response = await fetch("/api/auth/me", { credentials: "include" })
      if (response.status === 401) {
        clearSession()
        return null
      }
      if (!response.ok) throw new Error(`Load user failed: ${response.status}`)

      const user = (await response.json()) as CurrentUser
      setCurrentUser(user)
      return user
    } finally {
      setLoading(false)
      setReady(true)
    }
  }, [clearSession])

  const login = useCallback(async (username: string, password: string, captchaToken: string) => {
    const user = await requestJSON<CurrentUser>(
      "/api/auth/login",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, captchaToken }),
      },
      "login failed"
    )
    setCurrentUser(user)
    setReady(true)
    return user
  }, [])

  const register = useCallback(async (
    username: string,
    password: string,
    captchaToken: string,
    captchaId: string,
    captchaAnswer: string,
    inviteCode: string
  ) => {
    const user = await requestJSON<CurrentUser>(
      "/api/auth/register",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, captchaToken, captchaId, captchaAnswer, inviteCode }),
      },
      "register failed"
    )
    setCurrentUser(user)
    setReady(true)
    return user
  }, [])

  const logout = useCallback(async () => {
    await requestEmpty("/api/auth/logout", { method: "POST" }, "退出登录失败")
    clearSession()
  }, [clearSession])

  const changePassword = useCallback(async (currentPassword: string, newPassword: string) => {
    await requestJSON<unknown>(
      "/api/auth/password",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ currentPassword, newPassword }),
      },
      "修改密码失败"
    )
  }, [])

  const acknowledgeUsageGuide = useCallback(async () => {
    const user = await requestJSON<CurrentUser>(
      "/api/user/usage-guide/ack",
      { method: "POST" },
      "确认使用说明失败"
    )
    setCurrentUser(user)
    setReady(true)
    return user
  }, [])

  const fetchSessions = useCallback(() => requestJSON<SessionView[]>("/api/auth/sessions", {}, "读取会话失败"), [])

  const logoutOtherSessions = useCallback(async () => {
    await requestEmpty("/api/auth/sessions/others", { method: "DELETE" }, "退出其他会话失败")
  }, [])

  const value = useMemo<AuthContextValue>(() => ({
    currentUser,
    loading,
    ready,
    isAdmin: currentUser?.role === "admin",
    clearSession,
    fetchMe,
    login,
    register,
    logout,
    changePassword,
    acknowledgeUsageGuide,
    fetchSessions,
    logoutOtherSessions,
  }), [acknowledgeUsageGuide, changePassword, clearSession, currentUser, fetchMe, fetchSessions, loading, login, logout, logoutOtherSessions, ready, register])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error("useAuth must be used within AuthProvider")
  return value
}
