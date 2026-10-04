import type { NextConfig } from "next";

// The browser talks only to /api/*; Next proxies it to the Go API. This keeps
// the API's internal address out of client bundles and avoids CORS.
const apiUrl = process.env.LEETFORCE_API_URL ?? "http://127.0.0.1:8080";

// Dev only: hostnames other than localhost that may load the dev server's
// scripts (for example a LAN address), comma separated. Without it Next blocks
// them and the page never hydrates, so no button works.
const devOrigins = (process.env.LEETFORCE_DEV_ORIGINS ?? "")
  .split(",")
  .map((h) => h.trim())
  .filter(Boolean);

const nextConfig: NextConfig = {
  allowedDevOrigins: devOrigins,
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiUrl}/:path*` }];
  },
};

export default nextConfig;
