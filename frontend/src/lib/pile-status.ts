import type { Pile } from "@/types/dashboard";

export type PileTone = "idle" | "charging" | "offline";

export function getPilePresentation(pile: Pile): {
  label: string;
  tone: PileTone;
} {
  if (!pile.online) {
    return { label: "设备离线", tone: "offline" };
  }
  if (pile.ports.some((port) => port.status === "in_use")) {
    return { label: "充电中", tone: "charging" };
  }
  return { label: "全部空闲", tone: "idle" };
}
