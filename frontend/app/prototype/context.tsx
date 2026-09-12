"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react"
import {
  initialData,
  parseRoute,
  routeHref,
  safeData,
  STORAGE_KEY,
  type PrototypeData,
  type Route,
  type Scenario,
} from "./model"

const INITIAL = JSON.stringify(initialData())
let memory = INITIAL
const STORE_EVENT = "charge-prototype-store"
function getSnapshot() {
  try {
    return window.localStorage.getItem(STORAGE_KEY) || memory
  } catch {
    return memory
  }
}
function subscribeStore(listener: () => void) {
  window.addEventListener(STORE_EVENT, listener)
  window.addEventListener("storage", listener)
  return () => {
    window.removeEventListener(STORE_EVENT, listener)
    window.removeEventListener("storage", listener)
  }
}
function updateStore(recipe: (data: PrototypeData) => void) {
  const next = structuredClone(safeData(getSnapshot()))
  recipe(next)
  memory = JSON.stringify(next)
  try {
    window.localStorage.setItem(STORAGE_KEY, memory)
  } catch {
    /* Memory-only preview when storage is unavailable. */
  }
  window.dispatchEvent(new Event(STORE_EVENT))
}
function subscribeHash(listener: () => void) {
  window.addEventListener("hashchange", listener)
  return () => window.removeEventListener("hashchange", listener)
}
function subscribeOnline(listener: () => void) {
  window.addEventListener("online", listener)
  window.addEventListener("offline", listener)
  return () => {
    window.removeEventListener("online", listener)
    window.removeEventListener("offline", listener)
  }
}
function subscribeDark(listener: () => void) {
  const media = window.matchMedia("(prefers-color-scheme: dark)")
  media.addEventListener("change", listener)
  return () => media.removeEventListener("change", listener)
}

export type ModalState = {
  kind:
    | "add"
    | "edit"
    | "remove"
    | "watch"
    | "search"
    | "preview"
    | "guide"
    | "scan"
    | "cookie"
    | "wx-bind"
    | "unlink"
    | "password"
    | "sessions"
    | "logout"
    | "create-user"
    | "reset-password"
    | "delete-user"
    | "invite"
    | "clear-notices"
  id?: string
  returnTo?: "add"
}
type ToastState = { message: string; tone: "success" | "error" | "info" } | null
type Context = {
  data: PrototypeData
  route: Route
  go: (route: Route) => void
  mutate: (recipe: (data: PrototypeData) => void) => void
  commit: (
    recipe: (data: PrototypeData) => void,
    success: string,
    tone?: "success" | "error" | "info"
  ) => Promise<boolean>
  scenario: Scenario
  setScenario: (scenario: Scenario) => void
  offline: boolean
  theme: "light" | "dark"
  modal: ModalState | null
  openModal: (modal: ModalState | null) => void
  toast: ToastState
  notify: (message: string, tone?: "success" | "error" | "info") => void
  dismissToast: () => void
  busy: string
  reset: () => void
  addAudit: (data: PrototypeData, action: string, target: string) => void
}
const PrototypeContext = createContext<Context | null>(null)

export function PrototypeProvider({ children }: { children: ReactNode }) {
  const serialized = useSyncExternalStore(
    subscribeStore,
    getSnapshot,
    () => INITIAL
  )
  const data = useMemo(() => safeData(serialized), [serialized])
  const hash = useSyncExternalStore(
    subscribeHash,
    () => window.location.hash,
    () => "#/piles"
  )
  const route = useMemo(() => parseRoute(hash), [hash])
  const online = useSyncExternalStore(
    subscribeOnline,
    () => navigator.onLine,
    () => true
  )
  const systemDark = useSyncExternalStore(
    subscribeDark,
    () => matchMedia("(prefers-color-scheme: dark)").matches,
    () => false
  )
  const [scenario, setScenario] = useState<Scenario>("ready")
  const [modal, openModal] = useState<ModalState | null>(null)
  const [toast, setToast] = useState<ToastState>(null)
  const [busy, setBusy] = useState("")
  const lock = useRef(false)
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const offline = !online || scenario === "offline"
  const theme =
    data.theme === "system" ? (systemDark ? "dark" : "light") : data.theme

  const notify = useCallback(
    (message: string, tone: "success" | "error" | "info" = "success") => {
      if (toastTimer.current) clearTimeout(toastTimer.current)
      setToast({ message, tone })
      toastTimer.current = setTimeout(() => setToast(null), 5500)
    },
    []
  )
  const go = useCallback((next: Route) => {
    window.location.hash = routeHref(next)
    openModal(null)
  }, [])
  const commit = useCallback(
    async (
      recipe: (data: PrototypeData) => void,
      success: string,
      tone: "success" | "error" | "info" = "success"
    ) => {
      if (lock.current) return false
      if (offline) {
        notify("当前离线，更改尚未保存。联网后可重试。", "error")
        return false
      }
      if (scenario === "error") {
        notify("这次操作未成功，填写内容已保留，可以再次尝试。", "error")
        setScenario("ready")
        return false
      }
      lock.current = true
      const origin =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null
      setBusy(success)
      await new Promise((resolve) => setTimeout(resolve, 320))
      try {
        updateStore(recipe)
        if (scenario === "empty") setScenario("ready")
        notify(success, tone)
        return true
      } finally {
        lock.current = false
        setBusy("")
        window.requestAnimationFrame?.(() => {
          if (origin?.isConnected && document.activeElement === document.body)
            origin.focus({ preventScroll: true })
        })
      }
    },
    [notify, offline, scenario]
  )
  const reset = useCallback(() => {
    updateStore((d) => Object.assign(d, initialData()))
    setScenario("ready")
    go({ view: "piles" })
    notify("示例数据已恢复。")
  }, [go, notify])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        openModal((current) =>
          current?.kind === "search" ? null : { kind: "search" }
        )
      }
    }
    window.addEventListener("keydown", onKey)
    return () => {
      window.removeEventListener("keydown", onKey)
      if (toastTimer.current) clearTimeout(toastTimer.current)
    }
  }, [])

  function addAudit(d: PrototypeData, action: string, target: string) {
    d.audit.unshift({
      id: `audit-${Date.now()}`,
      action,
      target,
      time: "刚刚",
      result: "success",
    })
  }
  return (
    <PrototypeContext.Provider
      value={{
        data,
        route,
        go,
        mutate: updateStore,
        commit,
        scenario,
        setScenario,
        offline,
        theme,
        modal,
        openModal,
        toast,
        notify,
        dismissToast: () => setToast(null),
        busy,
        reset,
        addAudit,
      }}
    >
      {children}
    </PrototypeContext.Provider>
  )
}

export function usePrototype() {
  const value = useContext(PrototypeContext)
  if (!value) throw new Error("PrototypeProvider is required")
  return value
}
