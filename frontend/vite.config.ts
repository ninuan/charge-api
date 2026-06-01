import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig(() => {
  const apiTarget = process.env.VITE_API_TARGET ?? "http://localhost:8080";

  return {
    plugins: [vue()],
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
