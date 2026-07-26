import type { Metadata } from "next"

import "./globals.css"
import { Providers } from "@/components/providers"

export const metadata: Metadata = {
  title: "Charge Console",
  description: "充电设施运营中心",
}

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body className="min-h-dvh bg-background text-foreground antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  )
}
