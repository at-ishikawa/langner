import type { NextConfig } from "next";

const apiBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;
const apiHost = apiBaseUrl ? new URL(apiBaseUrl).hostname : undefined;

const nextConfig: NextConfig = {
  allowedDevOrigins: apiHost ? [apiHost] : [],
};

export default nextConfig;
