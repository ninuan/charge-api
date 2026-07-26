"use client"

import { MonitorIcon, MoonIcon, SunIcon } from "lucide-react"
import { useTheme } from "next-themes"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

const modes = [
  { value: "light", label: "浅色", aria: "使用浅色模式", icon: SunIcon },
  { value: "dark", label: "深色", aria: "使用深色模式", icon: MoonIcon },
  { value: "system", label: "跟随系统", aria: "跟随系统外观", icon: MonitorIcon },
] as const

export function AppearanceSettings() {
  // 按下态用 theme 而非 resolvedTheme：跟随系统时 resolvedTheme 只会是
  // light/dark，用它判断会让"跟随系统"永远显示未选中。
  const { theme, setTheme } = useTheme()

  return <Card className="shadow-none"><CardHeader><CardTitle className="text-base">界面外观</CardTitle><p className="text-xs text-muted-foreground">选择适合当前环境的显示模式，跟随系统会随设备外观自动切换。</p></CardHeader><CardContent className="flex flex-wrap gap-2">{modes.map(({ value, label, aria, icon: Icon }) => <Button key={value} aria-label={aria} aria-pressed={theme === value} variant={theme === value ? "default" : "outline"} onClick={() => setTheme(value)}><Icon />{label}</Button>)}</CardContent></Card>
}
