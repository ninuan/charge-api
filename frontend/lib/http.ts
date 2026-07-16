const publicErrorMessages: Record<string, string> = {
  AUTH_INPUT_INVALID: "用户名或密码格式无效",
  AUTH_INVALID_CREDENTIALS: "用户名或密码错误",
  REGISTER_INPUT_INVALID: "用户名需要 3-64 个字符，密码需要 8-128 个字符",
  REGISTER_CAPTCHA_INVALID: "图片验证码错误或已过期，请重新获取。",
  TURNSTILE_INVALID: "人机验证失败，请重试。",
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
}

export async function responseErrorMessage(response: Response, fallback: string) {
  const body = (await response.json().catch(() => null)) as { code?: string } | null
  if (body?.code && publicErrorMessages[body.code]) return publicErrorMessages[body.code]

  if (response.status === 401) return "登录已失效，请重新登录"

  return fallback
}

export async function requestJSON<T>(path: string, init: RequestInit = {}, fallback: string): Promise<T> {
  const response = await fetch(path, { credentials: "include", ...init })

  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, fallback))
  }

  return response.json() as Promise<T>
}

export async function requestEmpty(path: string, init: RequestInit = {}, fallback: string) {
  const response = await fetch(path, { credentials: "include", ...init })

  if (!response.ok && response.status !== 204) {
    throw new Error(await responseErrorMessage(response, fallback))
  }
}
