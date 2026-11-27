/**
 * 校验准备渲染到 <a href> 的链接地址，拦截 javascript:、data:、// 协议相对地址等危险输入。
 * 默认仅放行标准 https: URL；公告等站内跳转可通过 allowRelative 放行安全的单斜杠相对路径。
 */
export function safeHref(
  raw: string | null | undefined,
  options: { allowRelative?: boolean } = {}
): string | null {
  const trimmed = (raw ?? "").trim()
  if (!trimmed) return null
  if (/[\u0000-\u001f\u007f\s]/.test(trimmed)) return null

  if (options.allowRelative && trimmed.startsWith("/")) {
    if (trimmed.startsWith("//") || trimmed.includes("\\")) return null
    return trimmed
  }

  try {
    const parsed = new URL(trimmed)
    if (parsed.protocol !== "https:") return null
    if (!parsed.hostname) return null
    return parsed.toString()
  } catch {
    return null
  }
}
