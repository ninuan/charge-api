import type { Metadata } from "next"
import Script from "next/script"

import "./globals.css"
import { Providers } from "@/components/providers"

export const metadata: Metadata = {
  title: "Charge Console",
  description: "充电设施运营中心",
}

const cfBeaconToken =
  process.env.NODE_ENV === "production"
    ? process.env.NEXT_PUBLIC_CF_BEACON_TOKEN?.trim() || ""
    : ""

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body className="min-h-dvh bg-background text-foreground antialiased">
        <Providers>{children}</Providers>
        {cfBeaconToken ? (
          <Script
            id="cloudflare-web-analytics"
            src="https://static.cloudflareinsights.com/beacon.min.js"
            data-cf-beacon={JSON.stringify({ token: cfBeaconToken })}
            strategy="afterInteractive"
          />
        ) : null}
      </body>
    </html>
  )
}
