import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// ビルド成果物は Go の embed パッケージへ直接出す。
// Go の //go:embed は親ディレクトリを参照できないため、backend 側に置く必要がある。
const GO_EMBED_DIR = '../backend/internal/presentation/webui/dist'

// 開発時は Vite の dev サーバーが /rpc を mcadmind へ中継する。
// 本番では mcadmind 自身が静的ファイルと /rpc の両方を提供するので中継は不要。
const DEV_BACKEND = 'http://127.0.0.1:8787'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: GO_EMBED_DIR,
    emptyOutDir: true,
    // Pi 5 のディスクを無駄に使わないため、本番ではソースマップを出さない
    sourcemap: false,
  },
  server: {
    proxy: {
      '/rpc': {
        target: DEV_BACKEND,
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    // E2E は Playwright が担当するので Vitest の対象から外す
    exclude: ['node_modules/**', 'e2e/**', 'dist/**'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      // 生成コードと E2E はカバレッジの分母に入れない
      exclude: [
        'src/gen/**',
        'src/main.tsx',
        'src/test/**',
        'e2e/**',
        '**/*.config.ts',
      ],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 80,
        statements: 80,
      },
    },
  },
})
