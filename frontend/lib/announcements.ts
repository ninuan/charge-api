import { requestJSON } from "@/lib/http"

export type {
  Announcement,
  AnnouncementPage,
  AnnouncementSummary,
} from "@/lib/api/generated"
export const announcementRequest = <T>(
  path: string,
  init: Parameters<typeof requestJSON>[1] = {}
) => requestJSON<T>(path, init, "公告暂时无法加载，请重试。")
export const announcementDate = (value: string) =>
  new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(value))
