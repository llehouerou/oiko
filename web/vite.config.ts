import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), babel({ presets: [reactCompilerPreset()] }), tailwindcss()],
  server: { port: 5180, strictPort: true, proxy: { '/api': 'http://localhost:8080' } },
  // The CSP allows no data: URL (ADR 0034). The license file lists what the bundle holds, for
  // scripts/notices.
  build: { assetsInlineLimit: 0, license: true },
})
