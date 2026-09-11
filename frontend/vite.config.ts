import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [svelte(), tailwindcss(), wails("./bindings")],
  build: {
    // Keep every font subset a separate lazy asset; a small one would
    // otherwise be inlined into the stylesheet and loaded on every start.
    assetsInlineLimit: (file) => (file.endsWith(".woff2") ? false : undefined),
  },
});
