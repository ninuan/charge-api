"use client"

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"

import { RequestError, requestEmpty, requestJSON } from "@/lib/http"
import type { CurrentUser, SessionView } from "@/lib/types"

type AuthContextValue = {
  currentUser: CurrentUser | null
  loading: boolean
  ready: boolean
  isAdmin: boolean
  clearSession: () => void
  fetchMe: () => Promise<CurrentUser | null>
  login: (
    username: string,
    password: string,
    captchaId: string,
    captchaAnswer: string
  ) => Promise<CurrentUser>
  register: (
    username: string,
    password: string,
    captchaId: string,
    captchaAnswer: string,
    inviteCode: string
  ) => Promise<CurrentUser>
  logout: () => Promise<void>
  changePassword: (
    currentPassword: string,
    newPassword: string
  ) => Promise<CurrentUser>
  acknowledgeUsageGuide: () => Promise<CurrentUser>
  fetchSessions: () => Promise<SessionView[]>
  logoutOtherSessions: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null)
  const [loading, setLoading] = useState(false)
  const [ready, setReady] = useState(false)
  const authEpochRef = useRef(0)
  const currentUserRef = useRef<CurrentUser | null>(null)
  const inFlightFetchMeRef = useRef<{
    epoch: number
    promise: Promise<CurrentUser | null>
  } | null>(null)

  const commitUser = useCallback((user: CurrentUser | null) => {
    currentUserRef.current = user
    setCurrentUser(user)
    setLoading(false)
    setReady(true)
  }, [])

  const clearSession = useCallback(() => {
    authEpochRef.current += 1
    inFlightFetchMeRef.current = null
    commitUser(null)
  }, [commitUser])

  const fetchMe = useCallback(async () => {
    const existing = inFlightFetchMeRef.current
    if (existing && existing.epoch === authEpochRef.current) {
      return existing.promise
    }
    const epoch = ++authEpochRef.current
    setLoading(true)
    const promise = (async () => {
      try {
        const user = await requestJSON<CurrentUser>(
          "/api/auth/me",
          { suppressSessionExpired: true },
          "读取账户信息失败，请稍后重试。"
        )
        if (epoch !== authEpochRef.current) return currentUserRef.current
        commitUser(user)
        return user
      } catch (reason) {
        if (epoch !== authEpochRef.current) return currentUserRef.current
        if (reason instanceof RequestError && reason.status === 401) {
          clearSession()
          return null
        }
        setLoading(false)
        setReady(true)
        throw reason
      } finally {
        if (inFlightFetchMeRef.current?.epoch === epoch) {
          inFlightFetchMeRef.current = null
        }
      }
    })()
    inFlightFetchMeRef.current = { epoch, promise }
    return promise
  }, [clearSession, commitUser])

  const login = useCallback(
    async (
      username: string,
      password: string,
      captchaId: string,
      captchaAnswer: string
    ) => {
      const user = await requestJSON<CurrentUser>(
        "/api/auth/login",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            username,
            password,
            captchaId,
            captchaAnswer,
          }),
        },
        "登录失败，请检查用户名、密码和验证后重试。"
      )
      authEpochRef.current += 1
      inFlightFetchMeRef.current = null
      commitUser(user)
      return user
    },
    [commitUser]
  )

  const register = useCallback(
    async (
      username: string,
      password: string,
      captchaId: string,
      captchaAnswer: string,
      inviteCode: string
    ) => {
      const user = await requestJSON<CurrentUser>(
        "/api/auth/register",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            username,
            password,
            captchaId,
            captchaAnswer,
            inviteCode,
          }),
        },
        "注册失败，请检查填写信息和验证后重试。"
      )
      authEpochRef.current += 1
      inFlightFetchMeRef.current = null
      commitUser(user)
      return user
    },
    [commitUser]
  )

  const logout = useCallback(async () => {
    await requestEmpty("/api/auth/logout", { method: "POST" }, "退出登录失败")
    clearSession()
  }, [clearSession])

  const changePassword = useCallback(
    async (currentPassword: string, newPassword: string) => {
      const user = await requestJSON<CurrentUser>(
        "/api/auth/password",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ currentPassword, newPassword }),
        },
        "修改密码失败"
      )
      authEpochRef.current += 1
      inFlightFetchMeRef.current = null
      commitUser(user)
      return user
    },
    [commitUser]
  )

  const acknowledgeUsageGuide = useCallback(async () => {
    const user = await requestJSON<CurrentUser>(
      "/api/user/usage-guide/ack",
      { method: "POST" },
      "确认使用说明失败"
    )
    authEpochRef.current += 1
    inFlightFetchMeRef.current = null
    commitUser(user)
    return user
  }, [commitUser])

  const fetchSessions = useCallback(
    () => requestJSON<SessionView[]>("/api/auth/sessions", {}, "读取会话失败"),
    []
  )

  const logoutOtherSessions = useCallback(async () => {
    await requestEmpty(
      "/api/auth/sessions/others",
      { method: "DELETE" },
      "退出其他会话失败"
    )
  }, [])

  const value = useMemo<AuthContextValue>(
    () => ({
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
    }),
    [
      acknowledgeUsageGuide,
      changePassword,
      clearSession,
      currentUser,
      fetchMe,
      fetchSessions,
      loading,
      login,
      logout,
      logoutOtherSessions,
      ready,
      register,
    ]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error("useAuth must be used within AuthProvider")
  return value
}
