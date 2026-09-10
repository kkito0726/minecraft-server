import { QueryClientProvider } from '@tanstack/react-query'
import { NavLink, Navigate, Route, BrowserRouter as Router, Routes } from 'react-router-dom'
import { useMemo } from 'react'

import { AppShell } from './components/templates'
import { TokenGate } from './components/organisms'
import { verifyToken } from './features/auth/verify'
import { createQueryClient } from './lib/queryClient'
import { BackupsPage, ServerPage, WorldsPage } from './pages/placeholders'

const NAV = [
  { to: '/', label: 'サーバー' },
  { to: '/worlds', label: 'ワールド' },
  { to: '/backups', label: 'バックアップ' },
] as const

/**
 * アプリケーションのルート。
 *
 * 入れ子の順序に意味がある。TokenGate を Router の外に置くのは、
 * 認証が通るまではどのルートも描画させないため。QueryClientProvider が
 * 最も外側なのは、ゲート自身の確認も同じ設定で動かすため。
 *
 * 操作の進捗購読（OperationProvider）はフェーズ 14 で AppShell の
 * banner に差し込む。
 */
export function App() {
  const queryClient = useMemo(() => createQueryClient(), [])

  return (
    <QueryClientProvider client={queryClient}>
      <TokenGate verify={verifyToken}>
        <Router>
          <AppShell brand="Minecraft サーバー管理コンソール" nav={<MainNav />}>
            <Routes>
              <Route path="/" element={<ServerPage />} />
              <Route path="/worlds" element={<WorldsPage />} />
              <Route path="/backups" element={<BackupsPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </AppShell>
        </Router>
      </TokenGate>
    </QueryClientProvider>
  )
}

function MainNav() {
  return (
    <>
      {NAV.map(({ to, label }) => (
        <NavLink
          key={to}
          to={to}
          end={to === '/'}
          className={({ isActive }) =>
            [
              'rounded px-2 py-1 text-sm',
              isActive ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-100',
            ].join(' ')
          }
        >
          {label}
        </NavLink>
      ))}
    </>
  )
}
