import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  root: 'web',
  plugins: [
    react(),
    tailwindcss(),
    {
      name: 'keep-embed-directory',
      generateBundle() {
        this.emitFile({ type: 'asset', fileName: '.gitkeep', source: '' })
      },
    },
  ],
  resolve: { alias: { '@': fileURLToPath(new URL('./web/src', import.meta.url)) } },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          react: ['react', 'react-dom', 'react-router-dom'],
          markdown: ['react-markdown', 'remark-gfm'],
          ui: ['@radix-ui/react-dialog', '@radix-ui/react-dropdown-menu', 'sonner'],
        },
      },
    },
  },
  server: { proxy: { '/api': 'http://127.0.0.1:8080', '/media': 'http://127.0.0.1:8080' } },
})
