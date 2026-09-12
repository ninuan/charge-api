"use client"
import { CheckIcon, MonitorIcon, MoonIcon, SunIcon } from "lucide-react"
import { useTheme } from "next-themes"
import { Switch } from "@/components/ui/switch"
import { useReducedMotionPreference } from "@/lib/motion-preference"
import { SectionHeading } from "@/components/workbench/surfaces"

const modes = [
  { value: "light", label: "浅色", aria: "使用浅色模式", icon: SunIcon },
  { value: "dark", label: "深色", aria: "使用深色模式", icon: MoonIcon },
  {
    value: "system",
    label: "跟随系统",
    aria: "跟随系统外观",
    icon: MonitorIcon,
  },
] as const
export function AppearanceSettings() {
  const { theme, setTheme } = useTheme(),
    { reduced, setReduced } = useReducedMotionPreference()
  return (
    <section>
      <SectionHeading
        title="界面外观"
        description="选择适合当前环境的显示模式，设置保存在当前浏览器。"
      />
      <div className="wb-theme-options">
        {modes.map(({ value, label, aria, icon: Icon }) => (
          <button
            key={value}
            aria-label={aria}
            aria-pressed={theme === value}
            onClick={() => setTheme(value)}
          >
            <span
              className={`wb-theme-sample wb-sample-${value}`}
              aria-hidden="true"
            >
              <i />
              <span>
                <b />
                <b />
                <b />
              </span>
            </span>
            <span>
              <Icon size={16} />
              {label}
              {theme === value && <CheckIcon size={15} />}
            </span>
          </button>
        ))}
      </div>
      <div className="wb-setting-row">
        <div>
          <h3>减少动态效果</h3>
          <p>默认跟随系统，也可以始终关闭位移、闪烁和加载动画。</p>
        </div>
        <Switch
          aria-label="始终减少动态效果"
          checked={reduced}
          onCheckedChange={setReduced}
        />
      </div>
    </section>
  )
}
