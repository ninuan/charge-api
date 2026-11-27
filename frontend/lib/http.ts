const publicErrorMessages: Record<string, string> = {
  YYB_RECOVERY_FAILED: "已保留平台绑定，登录状态暂时无法恢复，请稍后重试。",
  ANNOUNCEMENT_CONFLICT: "公告已更新或状态已变化，请重新加载后操作。",
  ANNOUNCEMENT_NOT_FOUND: "公告不存在、已撤下或当前不可操作。",
  ANNOUNCEMENT_INVALID: "请检查公告内容、链接和有效时间。",
  ANNOUNCEMENT_UNAVAILABLE: "公告暂时不可用，请稍后重试。",
  YYB_BINDING_BUSY: "平台连接正在更新，请稍后重试。",
  QR_SESSION_INVALID: "二维码会话已失效，请重新生成。",
  QR_SESSION_UNAVAILABLE: "暂时无法创建扫码会话，请稍后重试。",
  QR_NOT_AUTHORIZED: "请先在微信中确认授权。",
  QR_CONFIRM_IN_PROGRESS: "正在确认绑定，请检查扫码状态。",
  YYB_RESCAN_REQUIRED: "平台登录需要重新确认，请重新扫码绑定。",
  AUTH_INPUT_INVALID: "用户名或密码格式无效",
  AUTH_INVALID_CREDENTIALS: "用户名或密码错误",
  LOGIN_CAPTCHA_REQUIRED: "用户名或密码错误，请完成图片验证码后重试。",
  LOGIN_CAPTCHA_INVALID: "图片验证码错误或已过期，请重新获取。",
  REGISTER_INPUT_INVALID: "用户名需要 3-64 个字符，密码需要 8-128 个字符",
  REGISTER_CAPTCHA_INVALID: "图片验证码错误或已过期，请重新获取。",
  REGISTER_USERNAME_TAKEN: "该用户名已被注册，请换一个用户名。",
  REGISTER_CLOSED: "当前未开放自助注册，请联系管理员开通账户。",
  REGISTER_INVITE_DISABLED: "当前未开放邀请码注册。",
  REGISTER_INVITE_REQUIRED: "请输入有效邀请码。",
  REGISTER_INVITE_INVALID: "邀请码无效或已过期，请核对后重试。",
  RATE_LIMITED: "请求过于频繁，请稍后再试",
  YYB_BINDING_REQUIRED: "请先完成扫码登录绑定，再添加充电桩",
  PILE_IDENTIFIER_REQUIRED: "请输入桩号或设备长ID",
  PILE_ID_INVALID: "设备ID必须是 6-64 位数字",
  PILE_NUMBER_INVALID: "桩号必须是 6-64 位数字",
  PILE_FIELDS_INVALID: "充电桩字段长度超出限制",
  PILE_PORT_COUNT_INVALID: "充电口数量必须在 1-20 之间",
  COOKIE_REQUIRED: "请输入 Cookie",
  COOKIE_TOO_LARGE: "Cookie 内容过长",
  DEVICE_ID_INVALID: "设备 ID 格式无效",
  PORT_ID_INVALID: "充电口编号格式无效",
  HISTORY_QUERY_INVALID: "历史范围或时区参数无效",
  HISTORY_NOT_FOUND: "未找到对应的历史记录",
  HISTORY_RANGE_TOO_LARGE: "该范围内历史变化过多，请缩短查询范围",
  HISTORY_REMOVED: "使用历史已下线，请查看充电桩最近状态。",
  HISTORY_UNAVAILABLE: "历史数据暂时不可用，请稍后重试",
  ADMIN_TREND_QUERY_INVALID: "趋势范围或时区参数无效",
  ADMIN_TRENDS_UNAVAILABLE: "运营趋势暂时不可用，请稍后重试",
  WATCH_RULE_INVALID: "空闲提醒设置无效",
  WATCH_TARGET_NOT_FOUND: "未找到当前账户下的充电桩",
  WATCH_RULE_NOT_FOUND: "未找到这条空闲提醒",
  WATCH_RULE_CONFLICT: "该充电桩已设置空闲提醒",
  WATCH_PILE_LIMIT_REACHED: "已达到当前账户的提醒充电桩上限",
  WATCH_UNAVAILABLE: "空闲提醒功能暂时不可用，请稍后重试",
  NOTIFICATION_PREFERENCE_INVALID: "免打扰设置无效",
  NOTIFICATION_UNAVAILABLE: "通知设置暂时不可用，请稍后重试",
  WXPUSHER_NOT_CONFIGURED: "管理员暂未启用微信提醒",
  WXPUSHER_ALREADY_BOUND: "请先解除当前微信绑定",
  WXPUSHER_BIND_SESSION_ACTIVE: "已有等待扫码的二维码，请稍后再试",
  WXPUSHER_BIND_SESSION_NOT_FOUND: "绑定二维码已失效，请重新获取",
  WXPUSHER_UID_CONFLICT: "这个微信接收账号已绑定其他账户",
  WXPUSHER_NOT_BOUND: "请先绑定微信提醒",
  WXPUSHER_CHANNEL_DISABLED: "请先开启微信提醒",
  WXPUSHER_INVALID: "微信提醒设置无效",
  WXPUSHER_TEST_NOT_FOUND: "未找到可查询的测试消息",
  WXPUSHER_RATE_LIMITED: "操作过于频繁，请稍后再试",
  WXPUSHER_UNAVAILABLE: "微信提醒服务暂时不可用，请稍后重试",
  WATCH_POWER_OFF_ACTIVE: "当前处于计划断电时段，请在恢复供电后再开启提醒",
  WATCH_CREDENTIAL_EXPIRED: "个人 Cookie 已过期，请先在账号页更新后再开启提醒",
  WATCH_RECURRING_DISABLED: "固定时段提醒已停用，请开启临时提醒",
  NOTIFICATION_QUERY_INVALID: "通知筛选或分页参数无效",
  NOTIFICATION_NOT_FOUND: "未找到通知",
  PASSWORD_CHANGE_REQUIRED: "请先修改临时密码后再继续操作",
  REFRESH_QUOTA_EXHAUSTED:
    "今日免费刷新次数与奖励额度已用完，上传个人 Cookie 并加入互助池可不限次刷新，或录入全站新桩赚取额外次数。",
  PUBLIC_POOL_UNAVAILABLE:
    "公共查询通道暂无可用 Cookie。请前往「平台连接」上传个人 Cookie 并加入互助池，或联系管理员补充公共池。",
  DAILY_PROBE_LIMIT_REACHED: "今日录桩校验次数已达上限，请明天再试。",
  PERSONAL_COOKIE_REQUIRED:
    "此操作需要配置有效个人 Cookie 并加入互助公共池（请前往「平台连接」开启）。",
  COOKIE_TEST_FAILED: "测试个人 Cookie 失败，请检查网络或先添加至少 1 台充电桩后重试。",
  JOIN_POOL_FAILED: "加入互助公共池失败，请确认当前个人 Cookie 有效后重试。",
  PUBLIC_POOL_INVALID: "公共 Cookie 备注名或内容无效，请检查后重试。",
  PUBLIC_POOL_COOKIE_DUPLICATE:
    "该 Cookie 已在公共池中，请勿与其他账户共用同一个凭据。",
  PUBLIC_POOL_NOT_FOUND: "未找到对应的公共 Cookie。",
  PUBLIC_POOL_TEST_FAILED: "测试公共 Cookie 失败，请检查网络或确认公共桩库中至少有 1 台充电桩。",
  SHARED_PILE_NOT_FOUND: "未找到对应的公共充电桩。",
  SHARED_PILE_INVALID: "公共充电桩名称或位置格式无效。",
  SHARED_PILE_REFRESH_FAILED: "刷新公共桩失败，请确认公共 Cookie 池有可用凭据。",
  USER_QUOTA_INVALID: "永久奖励刷新次数需要在 0 到 10000 之间。",
}

export async function responseErrorMessage(
  response: Response,
  fallback: string
) {
  const body = (await response.json().catch(() => null)) as {
    code?: string
  } | null
  if (body?.code && publicErrorMessages[body.code])
    return publicErrorMessages[body.code]

  if (response.status === 401) return "登录已失效，请重新登录"

  return fallback
}

async function responseError(response: Response, fallback: string) {
  const body = (await response.json().catch(() => null)) as {
    code?: string
  } | null
  return {
    code: body?.code,
    message:
      (body?.code && publicErrorMessages[body.code]) ||
      (response.status === 401 ? "登录已失效，请重新登录" : fallback),
  }
}

export class RequestError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string
  ) {
    super(message)
    this.name = "RequestError"
  }
}

const defaultTimeoutMs = 30_000
export const sessionExpiredEvent = "charge:session-expired"

export type RequestOptions = RequestInit & {
  // 需要串联远端设备请求的长操作（刷新、添加桩等）可放宽超时。
  timeoutMs?: number
  // 静默鉴权探测（如 AuthProvider.fetchMe）收到 401 时不派发全局过期事件。
  suppressSessionExpired?: boolean
}

export async function request<T>(
  path: string,
  init: RequestOptions = {},
  fallback: string
): Promise<T> {
  const {
    timeoutMs = defaultTimeoutMs,
    suppressSessionExpired = false,
    ...rest
  } = init
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  const external = rest.signal
  const forwardAbort = () => controller.abort()
  if (external?.aborted) controller.abort()
  else external?.addEventListener("abort", forwardAbort, { once: true })

  let response: Response
  try {
    response = await fetch(path, {
      credentials: "include",
      ...rest,
      signal: controller.signal,
    })
  } catch (error) {
    if (controller.signal.aborted && !external?.aborted)
      throw new RequestError("请求超时，请检查网络后重试", 0)
    throw error
  } finally {
    clearTimeout(timer)
    external?.removeEventListener("abort", forwardAbort)
  }

  if (!response.ok && response.status !== 204) {
    if (
      response.status === 401 &&
      !suppressSessionExpired &&
      path !== "/api/auth/login" &&
      path !== "/api/auth/register" &&
      typeof window !== "undefined"
    ) {
      window.dispatchEvent(new Event(sessionExpiredEvent))
    }
    const error = await responseError(response, fallback)
    throw new RequestError(error.message, response.status, error.code)
  }

  if (response.status === 204) return undefined as T

  return response.json() as Promise<T>
}

export const requestJSON = request

export async function requestEmpty(
  path: string,
  init: RequestOptions = {},
  fallback: string
) {
  await request<void>(path, init, fallback)
}
