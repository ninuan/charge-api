import type { PortStatus } from "@/types/dashboard";

export type PortTone = "idle" | "charging" | "offline";
export type PortIcon = "CircleCheck" | "Zap" | "WifiOff";

export function getPortPresentation(status: PortStatus): {
  label: string;
  tone: PortTone;
  icon: PortIcon;
} {
  if (status === "in_use") {
    return { label: "充电中", tone: "charging", icon: "Zap" };
  }
  if (status === "offline") {
    return { label: "离线", tone: "offline", icon: "WifiOff" };
  }
  return { label: "空闲", tone: "idle", icon: "CircleCheck" };
}
