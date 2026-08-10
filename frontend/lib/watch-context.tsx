"use client"

import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from "react"

import type {
  NotificationPreference,
  NotificationPreferenceUpdateRequest,
  WatchOverview,
  WatchRule,
  WatchRuleCreateRequest,
  WatchRuleUpdateRequest,
} from "@/lib/api/generated"
import { watchApi } from "@/lib/watch-api"

type WatchContextValue = {
  rules: WatchRule[]
  overview: WatchOverview | null
  preference: NotificationPreference | null
  loading: boolean
  loaded: boolean
  load: () => Promise<void>
  createRule: (payload: WatchRuleCreateRequest) => Promise<WatchRule>
  updateRule: (
    ruleId: string,
    payload: WatchRuleUpdateRequest
  ) => Promise<WatchRule>
  deleteRule: (ruleId: string) => Promise<void>
  updatePreference: (
    payload: NotificationPreferenceUpdateRequest
  ) => Promise<NotificationPreference>
}

const WatchContext = createContext<WatchContextValue | null>(null)

function reminderPileCount(rules: WatchRule[]) {
  return new Set(
    rules
      .filter((rule) => rule.enabled && rule.notifyIdle && rule.portId != null)
      .map((rule) => rule.deviceId)
  ).size
}

export function WatchProvider({ children }: { children: ReactNode }) {
  const [rules, setRules] = useState<WatchRule[]>([])
  const [overview, setOverview] = useState<WatchOverview | null>(null)
  const [preference, setPreference] = useState<NotificationPreference | null>(
    null
  )
  const [loading, setLoading] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const rulesRef = useRef<WatchRule[]>([])
  const loadPromiseRef = useRef<Promise<void> | null>(null)

  const applyRules = useCallback((next: WatchRule[]) => {
    const sorted = next.toSorted((left, right) =>
      right.updatedAt.localeCompare(left.updatedAt)
    )
    rulesRef.current = sorted
    setRules(sorted)
    setOverview((current) =>
      current
        ? {
            ...current,
            ruleCount: sorted.length,
            reminderPileCount: reminderPileCount(sorted),
          }
        : current
    )
  }, [])

  const load = useCallback(() => {
    if (loadPromiseRef.current) return loadPromiseRef.current
    setLoading(true)
    const pending = Promise.all([
      watchApi.rules(),
      watchApi.overview(),
      watchApi.preference(),
    ])
      .then(([nextRules, nextOverview, nextPreference]) => {
        rulesRef.current = nextRules
        setRules(nextRules)
        setOverview(nextOverview)
        setPreference(nextPreference)
        setLoaded(true)
      })
      .finally(() => {
        setLoading(false)
        loadPromiseRef.current = null
      })
    loadPromiseRef.current = pending
    return pending
  }, [])

  const createRule = useCallback(
    async (payload: WatchRuleCreateRequest) => {
      const created = await watchApi.createRule(payload)
      applyRules([
        created,
        ...rulesRef.current.filter((rule) => rule.id !== created.id),
      ])
      return created
    },
    [applyRules]
  )

  const updateRule = useCallback(
    async (ruleId: string, payload: WatchRuleUpdateRequest) => {
      const updated = await watchApi.updateRule(ruleId, payload)
      applyRules(
        rulesRef.current.map((rule) =>
          rule.id === updated.id ? updated : rule
        )
      )
      return updated
    },
    [applyRules]
  )

  const deleteRule = useCallback(
    async (ruleId: string) => {
      await watchApi.deleteRule(ruleId)
      applyRules(rulesRef.current.filter((rule) => rule.id !== ruleId))
    },
    [applyRules]
  )

  const updatePreference = useCallback(
    async (payload: NotificationPreferenceUpdateRequest) => {
      const updated = await watchApi.updatePreference(payload)
      setPreference(updated)
      return updated
    },
    []
  )

  const value = useMemo<WatchContextValue>(
    () => ({
      rules,
      overview,
      preference,
      loading,
      loaded,
      load,
      createRule,
      updateRule,
      deleteRule,
      updatePreference,
    }),
    [
      rules,
      overview,
      preference,
      loading,
      loaded,
      load,
      createRule,
      updateRule,
      deleteRule,
      updatePreference,
    ]
  )

  return <WatchContext.Provider value={value}>{children}</WatchContext.Provider>
}

export function useWatch() {
  const value = useContext(WatchContext)
  if (!value) throw new Error("useWatch must be used inside WatchProvider")
  return value
}
