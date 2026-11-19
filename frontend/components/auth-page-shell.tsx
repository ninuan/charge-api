import Link from "next/link"
import type { ReactNode } from "react"
import { BellRingIcon, PlugZapIcon, ShieldCheckIcon } from "lucide-react"
import { ThemeToggle } from "@/components/theme-toggle"
import { Brand } from "@/components/workbench/brand"

export function AuthPageShell({
  mode,
  children,
}: {
  mode: "login" | "register"
  children: ReactNode
}) {
  const register = mode === "register"
  return (
    <div className="wb-theme wb-auth-page wb-auth-centered auth-workbench min-h-dvh">
      <div className="wb-auth-shell-card">
        <section className="wb-auth-story" aria-label="Charge Console">
          <Link href="/login" aria-label="Charge 首页" className="wb-auth-brand">
            <Brand />
          </Link>
          <div className="wb-auth-story-body">
            <h2 className="wb-auth-headline">下楼之前，先看清哪儿有空口</h2>
            <p>
              把常去的车棚固定在一张画布里，每个插口的空闲状态与预计剩余时间一目了然。
            </p>
            <ul className="wb-auth-bullets" aria-label="主要功能">
              <li>
                <PlugZapIcon size={16} />
                <span>常去的充电桩集中查看，插口空闲一目了然</span>
              </li>
              <li>
                <BellRingIcon size={16} />
                <span>全满时开启临时空闲提醒，发现空位通知一次即停</span>
              </li>
              <li>
                <ShieldCheckIcon size={16} />
                <span>独立账户隔离，凭据加密存储，遵循夜间计划断电</span>
              </li>
            </ul>
          </div>
          <p className="wb-small wb-muted">
            Charge Console · 常用桩状态与空闲提醒
          </p>
        </section>

        <main className="wb-auth-form-section">
          <div className="wb-auth-form-wrap">
            <div className="wb-auth-topline">
              <Link href="/login" aria-label="Charge Console">
                <Brand small />
              </Link>
              <ThemeToggle />
            </div>
            <h1>{register ? "创建你的账户" : "欢迎回来"}</h1>
            <p>
              {register
                ? "创建独立账户，开始整理常用充电桩。"
                : "登录后，接着看看常去的地方。"}
            </p>
            <nav
              className="wb-seg-rail wb-auth-seg"
              aria-label="认证方式"
            >
              <Link
                href="/login"
                className={`wb-seg-btn ${!register ? "wb-seg-active" : ""}`}
                aria-current={!register ? "page" : undefined}
              >
                登录
              </Link>
              <Link
                href="/register"
                className={`wb-seg-btn ${register ? "wb-seg-active" : ""}`}
                aria-current={register ? "page" : undefined}
              >
                注册
              </Link>
            </nav>
            <div className="wb-auth-fields">{children}</div>
            <p className="wb-auth-security">
              <ShieldCheckIcon size={14} />
              公共设备使用后，请及时退出账户。
            </p>
          </div>
        </main>
      </div>
    </div>
  )
}
