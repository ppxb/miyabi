import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) }
  },
  test: {
    environment: 'node',
    include: ['tests/**/*.test.{mjs,ts,tsx}'],
    setupFiles: ['./tests/setup.mjs'],
    restoreMocks: true,
    unstubGlobals: true
  }
})
