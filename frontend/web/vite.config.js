import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
  },
  // Vitest: jsdom dipakai karena yang diuji adalah hook & komponen React
  // (PermissionContext), bukan modul murni saja. `globals` dimatikan supaya
  // describe/it/expect selalu di-import eksplisit -- lebih jelas dibaca dan
  // tidak menambah global yang tidak ada di kode produksi.
  test: {
    environment: 'jsdom',
    globals: false,
    include: ['src/**/*.test.{js,jsx}'],
  },
})
