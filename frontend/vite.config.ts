import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: true,
    // Accept any host (needed for ngrok / LAN / Tailscale).
    // Vite 5.x: an array of allowed hostnames; `true` disables the check entirely.
    allowedHosts: true,
    proxy: {
      "/api": "http://localhost:8787",
    },
    hmr: {
      // Allow HMR over the tunnel without forcing wss config; safe default for ngrok https.
      clientPort: 443,
    },
  },
});
