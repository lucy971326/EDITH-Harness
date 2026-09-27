import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // 前端独立构建，不让 Turbopack 误选机器上更高层的 lockfile。
  turbopack: { root: process.cwd() },
};

export default nextConfig;
