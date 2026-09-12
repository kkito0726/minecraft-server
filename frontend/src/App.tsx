import { QueryClientProvider } from '@tanstack/react-query'
import { NavLink, Navigate, Route, BrowserRouter as Router, Routes } from 'react-router-dom'
import { useMemo } from 'react'

import { PixelIcon } from './components/atoms'
import type { PixelIconName } from './components/atoms'
import { BrandMark } from './components/molecules'
import { AppShell } from './components/templates'
import {
  InterruptedBanner,
  OperationBanner,
  StatusStrip,
  TokenGate,
} from './components/organisms'
import { OperationProvider } from './features/operations'
import { verifyToken } from './features/auth/verify'
import { createQueryClient } from './lib/queryClient'
import { BackupsPage } from './pages/BackupsPage'
import { WorldsPage } from './pages/WorldsPage'
import { ServerPage } from './pages/ServerPage'

type NavEntry = {
  to: string
  label: string
  /** 飾りの略号。読み上げには label だけを渡す。 */
  code: string
  icon: PixelIconName
}

const NAV: readonly NavEntry[] = [
  { to: '/', label: 'サーバー', code: 'SRV', icon: 'server' },
  { to: '/worlds', label: 'ワールド', code: 'WLD', icon: 'world' },
  { to: '/backups', label: 'バックアップ', code: 'BAK', icon: 'backup' },
]

/**
 * アプリケーションのルート。
 *
 * 入れ子の順序に意味がある。TokenGate を Router の外に置くのは、
 * 認証が通るまではどのルートも描画させないため。QueryClientProvider が
 * 最も外側なのは、ゲート自身の確認も同じ設定で動かすため。
 * OperationProvider はゲートの内側に置く。認証が通る前に購読を
 * 始めても Unauthenticated で弾かれるだけになる。
 */
export function App() {
  const queryClient = useMemo(() => createQueryClient(), [])

  return (
    <QueryClientProvider client={queryClient}>
      <TokenGate verify={verifyToken}>
        <OperationProvider>
          <Router>
            <AppShell
              brand={<BrandMark title="Minecraft サーバー管理コンソール" />}
              nav={<MainNav />}
              status={<StatusStrip />}
              footer={<ShellFooter />}
              banner={
                <>
                  <InterruptedBanner />
                  <OperationBanner />
                </>
              }
            >
              <Routes>
                <Route path="/" element={<ServerPage />} />
                <Route path="/worlds" element={<WorldsPage />} />
                <Route path="/backups" element={<BackupsPage />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </AppShell>
          </Router>
        </OperationProvider>
      </TokenGate>
    </QueryClientProvider>
  )
}

/**
 * リンクの名前は label だけにする。アイコンと略号を aria-hidden にするのは、
 * 読み上げで「SRV」と言わせないためと、リンクを名前で探す試験を壊さないため。
 * 選択中の見た目は NavLink が付ける aria-current を CSS が拾う。
 */
function MainNav() {
  return (
    <>
      {NAV.map(({ to, label, code, icon }) => (
        <NavLink key={to} to={to} end={to === '/'} className="nav-item">
          <PixelIcon name={icon} className="size-5 shrink-0" />
          <span>{label}</span>
          <span aria-hidden="true" className="nav-code">
            {code}
          </span>
        </NavLink>
      ))}
    </>
  )
}

const GROUND = ['world', 'world', 'world', 'world', 'world'] as const

/** ナビゲーションの下の地面。飾りなので丸ごと読み上げから外す。 */
function ShellFooter() {
  return (
    <div aria-hidden="true" className="flex flex-col gap-3">
      <div className="flex">
        {GROUND.map((name, i) => (
          <PixelIcon key={i} name={name} className="size-7 text-emerald-deep/70" />
        ))}
      </div>
      <p className="hud-tag leading-relaxed">ACCESS // TAILNET ONLY</p>
    </div>
  )
}
