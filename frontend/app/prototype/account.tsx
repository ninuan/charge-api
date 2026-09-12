"use client"

import { useState, type FormEvent } from "react"
import {
  ArrowRight,
  Check,
  ChevronRight,
  CircleCheck,
  Cookie,
  KeyRound,
  Laptop,
  Link2,
  LockKeyhole,
  MessageCircle,
  Monitor,
  Moon,
  QrCode,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Sun,
} from "lucide-react"
import { usePrototype } from "./context"
import { credentialLabel, type NoticeType } from "./model"
import {
  Badge,
  Brand,
  EmptyState,
  ErrorState,
  Field,
  FormError,
  LoadingState,
  PageHeading,
  PButton,
  PInput,
  SaveButton,
  SectionHeading,
  SettingRow,
  Toggle,
} from "./ui"

export function AccountPage() {
  const { route, data, go, scenario } = usePrototype()
  const allowed =
    data.demoRole === "admin"
      ? ["security", "appearance"]
      : ["connection", "notifications", "security", "appearance"]
  const tab = allowed.includes(route.tab || "") ? route.tab! : allowed[0]
  const labels: Record<string, string> = {
    connection: "平台连接",
    notifications: "通知设置",
    security: "账户安全",
    appearance: "外观",
  }
  return (
    <div className="cp-standard-page cp-account-page">
      <PageHeading
        title="账户与设置"
        description="管理连接、接收方式和你的使用习惯。"
      />
      <div className="cp-profile-line">
        <span className="cp-avatar cp-avatar-large">
          {data.demoRole === "admin" ? "A" : data.username.slice(0, 1)}
        </span>
        <div>
          <h2>{data.demoRole === "admin" ? "admin" : data.username}</h2>
          <p>
            {data.demoRole === "admin" ? "管理员" : "个人账户"} ·
            数据仅对当前账户可见
          </p>
        </div>
        <Badge tone="neutral">
          <ShieldCheck size={12} />
          账户正常
        </Badge>
      </div>
      <nav
        className="cp-section-tabs cp-account-tabs"
        aria-label="账户设置分类"
      >
        {allowed.map((value) => (
          <button
            key={value}
            aria-current={tab === value ? "page" : undefined}
            aria-pressed={tab === value}
            onClick={() => go({ view: "account", tab: value })}
          >
            {labels[value]}
          </button>
        ))}
      </nav>
      {scenario === "loading" ? (
        <LoadingState label="正在读取设置" />
      ) : scenario === "error" ? (
        <ErrorState />
      ) : tab === "connection" ? (
        <ConnectionSettings />
      ) : tab === "notifications" ? (
        <NotificationSettings />
      ) : tab === "security" ? (
        <SecuritySettings />
      ) : (
        <AppearanceSettings />
      )}
    </div>
  )
}

function ConnectionSettings() {
  const { data, scenario, openModal, offline, commit, busy } = usePrototype()
  const state = scenario === "expired" ? "expired" : data.credential
  return (
    <div className="cp-settings-content">
      <SectionHeading
        title="充电平台连接"
        description="连接后，才能读取你添加的充电桩。"
      />
      <div className="cp-connection-line">
        <span className="cp-service-icon">
          <Link2 size={26} />
        </span>
        <div>
          <h3>
            充电平台{" "}
            <Badge
              tone={
                state === "healthy"
                  ? "idle"
                  : state === "expired" || state === "sync_failed"
                    ? "danger"
                    : "neutral"
              }
            >
              {credentialLabel[state]}
            </Badge>
          </h3>
          <p>
            {state === "healthy"
              ? "已绑定扫码账户 · 最近检查：今天 18:42"
              : state === "expired"
                ? "登录凭据已失效，重新扫码后可继续刷新和接收提醒。"
                : state === "waiting_device"
                  ? "扫码绑定已完成，添加一台充电桩后即可同步状态。"
                  : state === "sync_failed"
                    ? "上次同步未成功，可重新同步或扫码连接。"
                    : "先扫码连接，或手动填写访问凭据。"}
          </p>
        </div>
        <PButton
          tone={state === "healthy" ? "default" : "brand"}
          disabled={offline}
          onClick={() => openModal({ kind: "scan" })}
        >
          <QrCode size={16} />
          {state === "healthy" ? "重新扫码" : "扫码连接"}
        </PButton>
      </div>
      <SettingRow
        title="自动恢复登录"
        description="已绑定扫码账户时，凭据过期会尝试自动恢复；失败后再通知你。"
      >
        <span className="cp-small cp-idle-text">
          <CircleCheck size={14} />
          {state === "unbound" ? "连接后可用" : "已支持"}
        </span>
      </SettingRow>
      <SettingRow
        title="手动同步"
        description="使用已绑定账户重新获取访问凭据，不会重新绑定账户。"
      >
        <PButton
          loading={!!busy}
          disabled={offline || state === "unbound"}
          onClick={() =>
            void commit((d) => {
              d.credential = "healthy"
              d.notices
                .filter((n) => n.type === "credential_expired")
                .forEach((n) => {
                  n.resolved = true
                })
            }, "示例凭据已同步。")
          }
        >
          <RefreshCw size={14} />
          同步一次
        </PButton>
      </SettingRow>
      <SettingRow
        title="手动更新 Cookie"
        description="适用于未部署扫码服务的环境。只需更新连接凭据，不影响常用桩。"
      >
        <PButton
          tone="quiet"
          disabled={offline}
          onClick={() => openModal({ kind: "cookie" })}
        >
          <Cookie size={15} />
          手动更新
          <ChevronRight size={14} />
        </PButton>
      </SettingRow>
      {state !== "unbound" && (
        <SettingRow
          title="解除平台绑定"
          description="停止自动恢复登录。已有常用桩和历史记录会保留。"
        >
          <PButton
            tone="quiet"
            disabled={offline}
            onClick={() => openModal({ kind: "unlink", id: "platform" })}
          >
            解除绑定
          </PButton>
        </SettingRow>
      )}
      <div className="cp-settings-note">
        <ShieldCheck size={17} />
        <p>
          凭据仅用于读取你有权访问的充电桩，不在浏览器中展示。此原型不读取或保存真实凭据。
        </p>
      </div>
    </div>
  )
}

function NotificationSettings() {
  const { data, commit, openModal, offline, busy } = usePrototype()
  const [start, setStart] = useState(data.quietStart)
  const [end, setEnd] = useState(data.quietEnd)
  const [error, setError] = useState("")
  const eventLabels: Record<NoticeType, string> = {
    pile_available: "发现空闲口",
    credential_expired: "登录失效",
    pile_offline: "充电桩持续离线",
    pile_recovered: "充电桩恢复连接",
  }
  function saveQuiet(e: FormEvent) {
    e.preventDefault()
    if (!start || !end || start === end) {
      setError("请选择不同的开始和结束时间。")
      return
    }
    setError("")
    void commit((d) => {
      d.quietStart = start
      d.quietEnd = end
    }, "免打扰时段已保存。")
  }
  return (
    <div className="cp-settings-content">
      <SectionHeading
        title="接收方式"
        description="站内通知始终保留，其他渠道由你选择。"
      />
      <SettingRow
        title="站内通知"
        description="空闲提醒、连接问题和恢复消息的主记录。"
      >
        <Badge tone="idle">
          <Check size={12} />
          始终开启
        </Badge>
      </SettingRow>
      <SettingRow
        title="浏览器通知"
        description="网页打开时接收浏览器提醒，需要此浏览器的通知权限。"
      >
        <Toggle
          label="浏览器通知"
          checked={data.browserEnabled}
          disabled={offline}
          onChange={(v) =>
            void commit(
              (d) => {
                d.browserEnabled = v
              },
              v
                ? "原型已模拟开启浏览器通知，不申请真实权限。"
                : "浏览器通知已关闭。"
            )
          }
        />
      </SettingRow>
      <div className="cp-connection-line cp-wx-line">
        <span className="cp-service-icon cp-wx-icon">
          <MessageCircle size={25} />
        </span>
        <div>
          <h3>
            微信提醒{" "}
            <Badge tone={data.wxBound ? "idle" : "neutral"}>
              {data.wxBound ? "已绑定" : "未绑定"}
            </Badge>
          </h3>
          <p>
            {data.wxBound
              ? "通过 WxPusher 接收 · UID 尾号 2A8F（示例）"
              : "绑定后，离开网页也能收到空闲提醒。"}
          </p>
        </div>
        {data.wxBound ? (
          <Toggle
            label="微信提醒"
            checked={data.wxEnabled}
            disabled={offline}
            onChange={(v) =>
              void commit(
                (d) => {
                  d.wxEnabled = v
                },
                v ? "微信提醒已开启。" : "微信提醒已关闭，站内通知仍会保留。"
              )
            }
          />
        ) : (
          <PButton
            disabled={offline}
            onClick={() => openModal({ kind: "wx-bind" })}
          >
            <QrCode size={16} />
            扫码绑定
          </PButton>
        )}
      </div>
      {data.wxBound && (
        <div className="cp-channel-detail">
          <div className="cp-channel-event-list">
            {(Object.entries(eventLabels) as [NoticeType, string][]).map(
              ([type, label]) => (
                <label key={type}>
                  <input
                    type="checkbox"
                    checked={data.wxEvents.includes(type)}
                    disabled={!data.wxEnabled || offline}
                    onChange={(e) =>
                      void commit((d) => {
                        d.wxEvents = e.target.checked
                          ? [...d.wxEvents, type]
                          : d.wxEvents.filter((t) => t !== type)
                      }, "微信消息偏好已保存。")
                    }
                  />
                  {label}
                </label>
              )
            )}
          </div>
          <div className="cp-channel-test">
            <div>
              <strong>
                {data.wxTest === "none"
                  ? "检查是否能收到消息"
                  : data.wxTest === "queued"
                    ? "测试消息已进入发送队列"
                    : data.wxTest === "submitted"
                      ? "测试消息已提交渠道"
                      : "测试消息发送失败"}
              </strong>
              <p>
                {data.wxTest === "submitted"
                  ? "请在手机中确认，平台无法判断是否送达或已读。"
                  : "测试消息不影响已有提醒，查询结果不会重复发送。"}
              </p>
            </div>
            <div className="cp-row-actions">
              <PButton
                disabled={
                  !data.wxEnabled || offline || data.wxTest === "queued"
                }
                loading={!!busy}
                onClick={() =>
                  void commit((d) => {
                    d.wxTest = "queued"
                  }, "测试消息已进入原型发送队列。")
                }
              >
                发送测试
              </PButton>
              {data.wxTest !== "none" && (
                <PButton
                  disabled={offline}
                  onClick={() =>
                    void commit((d) => {
                      d.wxTest = "submitted"
                    }, "已查询原测试消息，没有重复发送。")
                  }
                >
                  查询结果
                </PButton>
              )}
            </div>
          </div>
          <button
            className="cp-text-link cp-muted"
            disabled={offline}
            onClick={() => openModal({ kind: "unlink", id: "wx" })}
          >
            解除微信绑定
            <ChevronRight size={12} />
          </button>
        </div>
      )}
      <SectionHeading
        title="免打扰"
        description="免打扰期间不向外部渠道推送，站内消息仍然记录。"
      />
      <SettingRow title="定时免打扰" description="按上海时间生效，支持跨午夜。">
        <Toggle
          checked={data.quietEnabled}
          label="定时免打扰"
          disabled={offline}
          onChange={(value) =>
            void commit((d) => {
              d.quietEnabled = value
            }, "免打扰设置已保存。")
          }
        />
      </SettingRow>
      {data.quietEnabled && (
        <form className="cp-quiet-form" onSubmit={saveQuiet}>
          <Field label="开始时间">
            <PInput
              type="time"
              value={start}
              onChange={(e) => setStart(e.target.value)}
              required
            />
          </Field>
          <span>至</span>
          <Field label="结束时间">
            <PInput
              type="time"
              value={end}
              onChange={(e) => setEnd(e.target.value)}
              required
            />
          </Field>
          <SaveButton />
          <FormError message={error} />
        </form>
      )}
    </div>
  )
}

function SecuritySettings() {
  const { data, openModal, offline } = usePrototype()
  return (
    <div className="cp-settings-content">
      <SectionHeading title="登录与安全" />
      <SettingRow
        title="登录密码"
        description="修改后保留当前登录，其他设备需要重新登录。"
      >
        <PButton
          disabled={offline}
          onClick={() => openModal({ kind: "password" })}
        >
          <KeyRound size={15} />
          修改密码
        </PButton>
      </SettingRow>
      <SectionHeading
        title="已登录设备"
        description="发现不熟悉的设备时，退出其他会话并修改密码。"
      />
      <div className="cp-session-row">
        <Laptop size={22} />
        <div>
          <h3>
            Chrome · macOS{" "}
            <Badge tone="idle" dot={false}>
              当前设备
            </Badge>
          </h3>
          <p>现在活跃 · 本地网络 · 9 月 10 日登录</p>
        </div>
        <span className="cp-small cp-muted">30 天后到期</span>
      </div>
      {data.otherSessions && (
        <div className="cp-session-row">
          <Smartphone size={22} />
          <div>
            <h3>Safari · iOS</h3>
            <p>今天 12:38 活跃 · 移动网络 · 9 月 8 日登录</p>
          </div>
          <span className="cp-small cp-muted">28 天后到期</span>
        </div>
      )}
      <div className="cp-settings-footer">
        <PButton
          disabled={!data.otherSessions || offline}
          onClick={() => openModal({ kind: "sessions" })}
        >
          退出其他登录设备
        </PButton>
      </div>
      <SettingRow
        title="退出当前账户"
        description="下次进入时需要重新登录。常用桩和设置会保留。"
      >
        <PButton tone="quiet" onClick={() => openModal({ kind: "logout" })}>
          退出登录
          <ArrowRight size={15} />
        </PButton>
      </SettingRow>
    </div>
  )
}

function AppearanceSettings() {
  const { data, mutate } = usePrototype()
  return (
    <div className="cp-settings-content">
      <SectionHeading
        title="让界面适合你的环境"
        description="主题设置只影响当前浏览器中的原型。"
      />
      <div className="cp-theme-options">
        {(
          [
            ["light", "浅色", Sun],
            ["dark", "深色", Moon],
            ["system", "跟随系统", Monitor],
          ] as const
        ).map(([value, label, Icon]) => (
          <button
            key={value}
            aria-pressed={data.theme === value}
            onClick={() =>
              mutate((d) => {
                d.theme = value
              })
            }
          >
            <span
              className={`cp-theme-sample cp-sample-${value}`}
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
              {data.theme === value && <Check size={15} />}
            </span>
          </button>
        ))}
      </div>
      <SettingRow
        title="减少动态效果"
        description="默认跟随系统，也可以始终减少位移、闪烁与加载动画。"
      >
        <Toggle
          label="始终减少动态效果"
          checked={data.reduceMotion}
          onChange={(value) =>
            mutate((d) => {
              d.reduceMotion = value
            })
          }
        />
      </SettingRow>
    </div>
  )
}

export function AuthPage() {
  const { route, go, mutate, openModal, offline, scenario, notify } =
    usePrototype()
  const register = route.view === "register"
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [invite, setInvite] = useState("")
  const [captcha, setCaptcha] = useState("")
  const [captchaCode, setCaptchaCode] = useState("4726")
  const [agreed, setAgreed] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (offline) {
      setError("当前离线，请联网后再登录。")
      return
    }
    if (!/^[\w\u4e00-\u9fff-]{2,32}$/.test(username.trim())) {
      setError("用户名需要 2–32 个字母、数字、中文或下划线。")
      return
    }
    if (password.length < 8) {
      setError("请填写至少 8 位演示密码，不要使用真实密码。")
      return
    }
    if (captcha !== captchaCode) {
      setError("演示验证码不正确，请重新输入。")
      return
    }
    if (register && invite !== "GARDEN-2026") {
      setError("演示邀请码为 GARDEN-2026。")
      return
    }
    if (register && !agreed) {
      setError("请先确认使用范围。")
      return
    }
    if (scenario === "error") {
      setError("暂时无法登录，输入内容已保留，请稍后重试。")
      return
    }
    setError("")
    setLoading(true)
    await new Promise((resolve) => setTimeout(resolve, 400))
    mutate((d) => {
      d.demoRole = username === "admin" ? "admin" : "user"
      d.username = username === "admin" ? "林一" : username
    })
    setLoading(false)
    go({ view: username === "admin" ? "admin" : "piles" })
    notify(register ? "演示账户已创建；没有创建真实账户。" : "已进入演示账户。")
    if (register) openModal({ kind: "guide" })
  }
  return (
    <div className="cp-auth-page">
      <section className="cp-auth-story">
        <Brand />
        <div>
          <span className="cp-eyebrow">给日常，留一点余量。</span>
          <h2 className="cp-auth-headline">
            有空闲，
            <br />
            再出发<span>。</span>
          </h2>
          <p>
            把常去的充电桩放在一起。
            <br />
            看一眼状态，或等一个恰好的提醒。
          </p>
          <div className="cp-auth-visual" aria-hidden="true">
            <div className="cp-auth-power-line" />
            <div className="cp-auth-socket">
              <i />
              <i />
              <i />
              <span>READY</span>
            </div>
            <div className="cp-auth-caption">
              <span className="cp-inline-status" />
              一切就绪，随时出发。
            </div>
          </div>
        </div>
        <span className="cp-small cp-muted">
          Charge Console · 只关注你常去的地方
        </span>
      </section>
      <main id="prototype-main" tabIndex={-1} className="cp-auth-form-section">
        <div className="cp-auth-form-wrap">
          <div className="cp-auth-topline">
            <span className="cp-example-caption">
              交互原型 · 不连接真实账户
            </span>
            <PButton
              tone="quiet"
              onClick={() => openModal({ kind: "preview" })}
            >
              预览设置
            </PButton>
          </div>
          <h1>{register ? "创建你的账户" : "欢迎回来"}</h1>
          <p>
            {register
              ? "使用邀请，开始整理常用的充电桩。"
              : "登录后，接着看看常去的地方。"}
          </p>
          {scenario === "empty" && register ? (
            <EmptyState
              icon={LockKeyhole}
              title="暂未开放注册"
              description="请联系管理员获取账户或邀请码。"
              action={
                <PButton onClick={() => go({ view: "login" })}>
                  返回登录
                </PButton>
              }
            />
          ) : (
            <form onSubmit={submit}>
              <Field label="用户名">
                <PInput
                  autoComplete="off"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="试试 linyi 或 admin"
                  required
                />
              </Field>
              <Field label="演示密码" hint="至少 8 位，不要输入真实密码。">
                <PInput
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="仅用于预览"
                  required
                />
              </Field>
              {register && (
                <Field label="邀请码">
                  <PInput
                    value={invite}
                    onChange={(e) => setInvite(e.target.value)}
                    placeholder="GARDEN-2026"
                  />
                </Field>
              )}
              <Field label="演示验证码">
                <span className="cp-captcha">
                  <PInput
                    inputMode="numeric"
                    value={captcha}
                    onChange={(e) => setCaptcha(e.target.value)}
                    maxLength={4}
                    placeholder="输入右侧数字"
                  />
                  <button
                    type="button"
                    aria-label="更换演示验证码"
                    onClick={() =>
                      setCaptchaCode(captchaCode === "4726" ? "6813" : "4726")
                    }
                  >
                    {captchaCode}
                    <RefreshCw size={13} />
                  </button>
                </span>
              </Field>
              {register && (
                <label className="cp-check-label">
                  <input
                    type="checkbox"
                    checked={agreed}
                    onChange={(e) => setAgreed(e.target.checked)}
                  />
                  仅接入我有权访问的设备，不进行高频采集。
                </label>
              )}
              <FormError message={error} />
              <PButton
                className="cp-auth-submit"
                tone="brand"
                loading={loading}
                disabled={offline}
                type="submit"
              >
                {register ? "创建演示账户" : "进入演示账户"}
                <ArrowRight size={16} />
              </PButton>
            </form>
          )}
          <div className="cp-auth-switch">
            {register ? "已经有账户？" : "还没有账户？"}
            <button
              onClick={() => {
                go({ view: register ? "login" : "register" })
                setError("")
              }}
            >
              {register ? "返回登录" : "使用邀请注册"}
            </button>
          </div>
          <div className="cp-auth-security">
            <LockKeyhole size={13} />
            所有数据和登录操作仅在原型中生效
          </div>
        </div>
      </main>
    </div>
  )
}
