import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// In development, /api (REST and WebSocket) is proxied to the Go API so the
// browser sees a single origin, exactly like behind nginx in production.
export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    port: 3000,
    proxy: { "/api": { target: "http://localhost:8080", ws: true } },
  },
});
