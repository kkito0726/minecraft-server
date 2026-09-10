import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  // 生成物とビルド成果物は対象外
  { ignores: ['dist', 'src/gen', 'coverage', 'playwright-report', 'test-results'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],

      // グローバル開発ルール由来
      'no-console': 'error',
      'max-lines': ['error', { max: 800, skipBlankLines: true, skipComments: true }],
      'max-lines-per-function': ['error', { max: 50, skipBlankLines: true, skipComments: true }],
      'max-depth': ['error', 4],

      // イミュータブル方針: 引数を書き換えない
      'no-param-reassign': ['error', { props: true }],
    },
  },
  {
    // テストは記述の都合上、関数長の制限を緩める
    files: ['**/*.test.{ts,tsx}', 'e2e/**/*.ts', 'src/test/**/*.ts'],
    rules: { 'max-lines-per-function': 'off' },
  },
  {
    // E2E は Node 上で動く。mcadmind を起動し、プロジェクトの
    // ディレクトリを直接覗くので、ブラウザの大域変数だけでは足りない。
    files: ['e2e/**/*.ts'],
    languageOptions: { globals: { ...globals.node } },
  },
)
