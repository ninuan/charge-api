"use client"

import { MoonIcon, SunIcon } from "lucide-react"
import { useTheme } from "next-themes"
import { useSyncExternalStore } from "react"

import { Button } from "@/components/ui/button"

/** A small, keyboard-friendly appearance control that stays useful on every page. */
export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()
  const mounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false
  )
  if (!mounted || !resolvedTheme) {
    return (
      <Button variant="ghost" size="icon" aria-label="切换界面外观" disabled />
    )
  }

  const dark = resolvedTheme === "dark"
  const Icon = dark ? MoonIcon : SunIcon

  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={dark ? "切换到浅色模式" : "切换到深色模式"}
      title={dark ? "切换到浅色模式" : "切换到深色模式"}
      onClick={() => setTheme(dark ? "light" : "dark")}
    >
      <Icon />
      <span className="sr-only">当前为{dark ? "深色" : "浅色"}模式</span>
    </Button>
  )
}
