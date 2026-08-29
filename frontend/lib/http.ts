const publicErrorMessages: Record<string, string> = {
  AUTH_INPUT_INVALID: "用户名或密码格式无效",
  AUTH_INVALID_CREDENTIALS: "用户名或密码错误",
  LOGIN_CAPTCHA_REQUIRED: "用户名或密码错误，请完成图片验证码后重试。",
  LOGIN_CAPTCHA_INVALID: "图片验证码错误或已过期，请重新获取。",
  REGISTER_INPUT_INVALID: "用户名需要 3-64 个字符，密码需要 8-128 个字符",
  REGISTER_CAPTCHA_INVALID: "图片验证码错误或已过期，请重新获取。",
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
  WXPUSHER_RATE_LIMITED: "操作过于频繁，请稍后再试",
  WXPUSHER_UNAVAILABLE: "微信提醒服务暂时不可用，请稍后重试",
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

export type RequestOptions = RequestInit & {
  // 需要串联远端设备请求的长操作（刷新、添加桩等）可放宽超时。
  timeoutMs?: number
}

export async function request<T>(
  path: string,
  init: RequestOptions = {},
  fallback: string
): Promise<T> {
  const { timeoutMs = defaultTimeoutMs, ...rest } = init
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  const external = rest.signal
  if (external?.aborted) controller.abort()
  else
    external?.addEventListener("abort", () => controller.abort(), {
      once: true,
    })

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
  }

  if (!response.ok && response.status !== 204) {
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
