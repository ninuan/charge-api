"use client"
import { useEffect, useSyncExternalStore } from "react"

const key = "charge:reduce-motion",
  eventName = "charge:motion-preference"
let memory = false
function snapshot() {
  try {
    return window.localStorage.getItem(key) === "1" || memory
  } catch {
    return memory
  }
}
function subscribe(listener: () => void) {
  window.addEventListener("storage", listener)
  window.addEventListener(eventName, listener)
  return () => {
    window.removeEventListener("storage", listener)
    window.removeEventListener(eventName, listener)
  }
}
export function useReducedMotionPreference() {
  const reduced = useSyncExternalStore(subscribe, snapshot, () => false)
  const setReduced = (value: boolean) => {
    memory = value
    try {
      window.localStorage.setItem(key, value ? "1" : "0")
    } catch {}
    window.dispatchEvent(new Event(eventName))
  }
  return { reduced, setReduced }
}
export function MotionPreference() {
  const { reduced } = useReducedMotionPreference()
  useEffect(() => {
    document.documentElement.dataset.reduceMotion = String(reduced)
    return () => {
      delete document.documentElement.dataset.reduceMotion
    }
  }, [reduced])
  return null
}
