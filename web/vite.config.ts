/// <reference types="vitest" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Port 15173 must match CORS_ORIGINS in .env / deploy/compose.env.
export default defineConfig({
  plugins: [react()],
  server: { host: "127.0.0.1", port: 15173, strictPort: true },
  preview: { host: "127.0.0.1", port: 15173, strictPort: true },
  test: { environment: "node" },
});
