/**
 * Vite + Vitest.
 *
 * `server.proxy` нужен для работы с реальным API (`localhost:8080`) при
 * `VITE_ENABLE_MSW=false`. В тестах API-база абсолютная, чтобы Node-fetch и
 * MSW согласованно перехватывали запросы (jsdom `fetch` требует полный URL).
 */
import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    host: true,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  preview: {
    port: 4173,
    host: true,
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      output: {
        manualChunks: {
          react: ['react', 'react-dom', 'react-router-dom'],
          antd: ['antd', '@ant-design/icons'],
          query: ['@tanstack/react-query'],
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    environmentOptions: {
      jsdom: { url: 'http://localhost:5173/' },
    },
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['node_modules', 'dist', 'coverage', 'e2e'],
    restoreMocks: true,
    clearMocks: true,
    env: {
      VITE_API_BASE_URL: 'http://localhost/api',
      VITE_ENABLE_MSW: 'false',
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      reportsDirectory: 'coverage',
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/test/**',
        'src/mocks/**',
        'src/**/*.d.ts',
        'src/main.tsx',
      ],
      thresholds: {
        // Осмысленный минимум для Ф1; целевой уровень проекта — 60% (NFR-6).
        lines: 40,
        functions: 40,
        statements: 40,
        branches: 30,
      },
    },
  },
});
