import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // Matches gateway/internal/controlplane/webui.go's `//go:embed all:webui/dist`.
    outDir: "dist",
  },
  server: {
    // `bun run dev` proxies /admin to a gateway control-plane already
    // running locally (see Makefile's `ui-dev` target), so the SPA talks
    // to the real API during development without a CORS setup — matching
    // how the embedded production build shares the same origin.
    proxy: {
      "/admin": {
        target: "http://localhost:8081",
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./src/test/setup.ts",
  },
});
