import type { Metadata } from "next"
import Script from "next/script"

import "./globals.css"
import { Providers } from "@/components/providers"

export const metadata: Metadata = {
  title: "Charge Console",
  description: "充电设施运营中心",
}

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body className="min-h-dvh bg-background text-foreground antialiased">
        <Providers>{children}</Providers>
      </body>
      <Script
        id="cloudflare-web-analytics"
        type="module"
        src="https://static.cloudflareinsights.com/beacon.min.js"
        data-cf-beacon='{"token":"40a997d22bb446098457e51a3e6cdf84"}'
        strategy="lazyOnload"
      />
    </html>
  )
}
