import type {
  WxPusherBindSession,
  WxPusherChannelState,
} from "@/lib/api/generated"
import { request, requestEmpty, type RequestOptions } from "@/lib/http"

const channelPath = "/api/notification-channels/wxpusher"

export function getWxPusherChannel(options?: RequestOptions) {
  return request<WxPusherChannelState>(
    channelPath,
    options,
    "暂时无法读取微信提醒状态"
  )
}

export function createWxPusherBindSession(options?: RequestOptions) {
  return request<WxPusherBindSession>(
    `${channelPath}/bind-sessions`,
    { method: "POST", ...options },
    "暂时无法获取绑定二维码"
  )
}

export function pollWxPusherBindSession(
  sessionId: string,
  options?: RequestOptions
) {
  return request<WxPusherBindSession>(
    `${channelPath}/bind-sessions/${encodeURIComponent(sessionId)}`,
    options,
    "暂时无法确认扫码结果"
  )
}

export function deleteWxPusherChannel(options?: RequestOptions) {
  return requestEmpty(
    channelPath,
    { method: "DELETE", ...options },
    "暂时无法解除微信提醒绑定"
  )
}
