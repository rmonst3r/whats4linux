import { defineConfig } from "vite"
import tailwindcss from "@tailwindcss/vite"
import path from "path"

export default defineConfig({
  plugins: [tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return
          if (id.includes("@emoji-mart")) return "emoji"
          if (id.includes("gsap")) return "animation"
          if (id.includes("zustand") || id.includes("immer")) return "state"
          if (id.includes("react-virtuoso") || id.includes("react-resizable-panels")) {
            return "ui"
          }
          if (id.includes("react") || id.includes("scheduler")) return "react"
          return "vendor"
        },
      },
    },
  },
})
