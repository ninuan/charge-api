import type { Metadata } from "next"
import { ChargePrototype } from "./prototype"

export const metadata: Metadata = {
  title: "Charge · 交互原型",
  description: "查看常用充电桩，找到空闲充电口。独立示例数据，不连接真实账户。",
  robots: { index: false, follow: false },
}

export default function PrototypePage() {
  return <ChargePrototype />
}
