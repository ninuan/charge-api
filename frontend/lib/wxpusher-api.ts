import type {
  NotificationDeliverySummary,
  WxPusherBindSession,
  WxPusherChannelState,
  WxPusherChannelUpdateRequest,
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

export function updateWxPusherChannel(
  payload: WxPusherChannelUpdateRequest,
  options?: RequestOptions
) {
  return request<WxPusherChannelState>(
    channelPath,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      ...options,
    },
    "暂时无法保存微信提醒设置"
  )
}

export function testWxPusherChannel(options?: RequestOptions) {
  return request<NotificationDeliverySummary>(
    `${channelPath}/test`,
    { method: "POST", ...options },
    "暂时无法发送测试消息"
  )
}
