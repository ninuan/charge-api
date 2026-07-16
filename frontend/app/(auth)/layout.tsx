import type { ReactNode } from "react"

import { AuthPortal } from "@/components/auth-portal"

export default function AuthLayout({ children }: { children: ReactNode }) {
  return <><AuthPortal />{children}</>
}
