import type { NextConfig } from "next";

const codeflowApi = process.env.CODEFLOW_API_URL ?? "http://localhost:8742";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/codeflow/:path*",
        destination: `${codeflowApi}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
