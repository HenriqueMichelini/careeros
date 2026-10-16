import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { resolve } from "node:path"

export default defineConfig({
  define: { "process.env.NODE_ENV": JSON.stringify("production") },
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "lab/web",
    emptyOutDir: true,
    cssCodeSplit: false,
    lib: {
      entry: resolve(import.meta.dirname, "lab/ui/main.tsx"),
      formats: ["es"],
      fileName: () => "lab.js",
    },
    rollupOptions: { output: { assetFileNames: "lab.[ext]" } },
  },
})
