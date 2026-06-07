import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import path from "node:path";

export default defineConfig(() => {
  const apiTarget = process.env.VITE_API_TARGET ?? "http://localhost:8080";

  return {
    plugins: [vue()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src")
      }
    },
    server: {
      port: 5173,
      proxy: {
        "/api": {
          target: apiTarget,
          changeOrigin: true
        },
        "/healthz": {
          target: apiTarget,
          changeOrigin: true
        }
      }
    }
  };
});
