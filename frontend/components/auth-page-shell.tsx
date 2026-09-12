import Link from "next/link"
import type { ReactNode } from "react"
import { ShieldCheckIcon } from "lucide-react"
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
    <div className="wb-theme wb-auth-page auth-workbench min-h-dvh">
      <section className="wb-auth-story" aria-label="Charge Console">
        <Link href="/login" aria-label="Charge 首页">
          <Brand />
        </Link>
        <div>
          <p className="wb-eyebrow">给日常，留一点余量。</p>
          <h2 className="wb-auth-headline">
            有空闲，
            <br />
            再出发<span>。</span>
          </h2>
          <p>
            把常去的充电桩放在一起。
            <br />
            看一眼状态，或等一个恰好的提醒。
          </p>
          <dl className="wb-auth-features" aria-label="主要功能">
            <div>
              <dt>常用桩</dt>
              <dd>常去的地方，一处查看</dd>
            </div>
            <div>
              <dt>端口状态</dt>
              <dd>查看最近读取的使用情况</dd>
            </div>
            <div>
              <dt>空闲提醒</dt>
              <dd>有空闲时，告诉你一声</dd>
            </div>
          </dl>
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
          <nav className="wb-section-tabs wb-auth-tabs" aria-label="认证方式">
            <Link href="/login" aria-current={!register ? "page" : undefined}>
              登录
            </Link>
            <Link href="/register" aria-current={register ? "page" : undefined}>
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
  )
}
