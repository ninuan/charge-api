import type {
  DeviceHistoryResponse,
  operations,
  PortHistoryResponse,
} from "@/lib/api/generated"
import { request } from "@/lib/http"

export type HistoryQuery = NonNullable<
  operations["getDeviceHistory"]["parameters"]["query"]
>

function historyPath(path: string, query: HistoryQuery = {}) {
  const search = new URLSearchParams()
  if (query.range) search.set("range", query.range)
  if (query.timezone) search.set("timezone", query.timezone)

  const suffix = search.toString()
  return suffix ? `${path}?${suffix}` : path
}

export const historyApi = {
  device: (deviceId: string, query: HistoryQuery = {}) =>
    request<DeviceHistoryResponse>(
      historyPath(`/api/piles/${encodeURIComponent(deviceId)}/history`, query),
      {},
      "加载充电桩历史失败"
    ),
  port: (deviceId: string, portId: number, query: HistoryQuery = {}) =>
    request<PortHistoryResponse>(
      historyPath(
        `/api/piles/${encodeURIComponent(deviceId)}/ports/${portId}/history`,
        query
      ),
      {},
      "加载充电口历史失败"
    ),
}
