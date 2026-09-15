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
      <TokenGate verify={verifyToken} {...(__DEMO__ ? { initialToken: DEMO_TOKEN } : {})}>
        <OperationProvider>
          {/* 公開デモは /<リポジトリ名>/ の下に置かれる。基点を合わせないと再読み込みで 404 になる。 */}
          <Router basename={import.meta.env.BASE_URL}>
            <AppShell
              brand={<BrandMark title="Minecraft サーバー管理コンソール" />}
              nav={<MainNav />}
              status={<StatusStrip />}
              footer={<ShellFooter />}
              banner={
                <>
                  {__DEMO__ && <DemoBanner />}
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
 * 公開デモの入口に入れておくトークン。
 *
 * 何を入れても通るが、入口の画面そのものも見てもらいたいので、
 * 画面は残したまま初期値だけ埋めておく。
 */
const DEMO_TOKEN = 'demo'.repeat(16)

/**
 * デモであることを常に見せる帯。
 *
 * 本物の管理画面と見分けがつかないまま操作されると、
 * 「サーバーを止めた」と誤解させる。閉じられないようにしておく。
 */
function DemoBanner() {
  return (
    <div className="border-b border-diamond/40 bg-diamond/10 px-4 py-2.5 text-xs text-diamond lg:px-8">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-3 gap-y-1">
        <span className="badge badge-neutral text-diamond">DEMO</span>
        <span>
          これはデモです。実際のサーバーには接続していません。
          操作の結果はブラウザの中だけに残り、再読み込みで最初の状態に戻ります。
        </span>
      </div>
    </div>
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
