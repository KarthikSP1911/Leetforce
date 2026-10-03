import type { NextConfig } from "next";

// The browser talks only to /api/*; Next proxies it to the Go API. This keeps
// the API's internal address out of client bundles and avoids CORS.
const apiUrl = process.env.LEETFORCE_API_URL ?? "http://127.0.0.1:8080";

const nextConfig: NextConfig = {
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiUrl}/:path*` }];
  },
};

export default nextConfig;
