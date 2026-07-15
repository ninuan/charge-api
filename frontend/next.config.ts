import type { NextConfig } from "next"

const nextConfig: NextConfig = {
  trailingSlash: true,
  ...(process.env.NODE_ENV !== "development" ? { output: "export" } : {}),
  ...(process.env.NODE_ENV === "development" && process.env.NEXT_PUBLIC_API_TARGET
    ? {
        async rewrites() {
          return [
            { source: "/api/:path*", destination: `${process.env.NEXT_PUBLIC_API_TARGET}/api/:path*` },
            { source: "/healthz", destination: `${process.env.NEXT_PUBLIC_API_TARGET}/healthz` },
          ]
        },
      }
    : {}),
}

export default nextConfig
