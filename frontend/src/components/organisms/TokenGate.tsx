import { useCallback, useEffect, useState } from 'react'
import type { Dispatch, ReactNode, SetStateAction } from 'react'

import { clearToken, readToken, tokenSchema, writeToken } from '../../features/auth/token'
import { describeError } from '../../lib/errors'
import { Button, PixelIcon, Spinner } from '../atoms'
import { FormField } from '../molecules'

/**
 * 共有トークンの入口。
 *
 * 到達制御は Tailscale が担い、ここは認可の二段目にあたる。
 * localStorage は XSS に晒されるが、単一利用者かつ tailnet 内という
 * 前提で受け入れ、Go 側の厳格な CSP で緩和している。
 *
 * 認証の失敗はこの画面が自分で扱う。「失敗したらトークンを消して
 * 再読み込み」という共通処理に任せると、入力するたびに画面が
 * 点滅するだけで理由が表示されない。
 */
export type TokenGateProps = {
  /**
   * トークンでサーバーに触れるかを確かめる。
   * 失敗したら例外を投げる。
   */
  verify: (token: string) => Promise<void>
  /**
   * 入力欄の初期値。
   *
   * 公開デモで、入口の画面を見せたうえですぐ入れるようにするために使う。
   * 実運用では渡さない。
   */
  initialToken?: string | undefined
  children: ReactNode
}

type Phase =
  /** 保存済みのトークンを確認している */
  | 'checking'
  /** 入力を待っている */
  | 'asking'
  /** 入力されたトークンを確認している */
  | 'verifying'
  /** 入場済み */
  | 'open'

export function TokenGate({ verify, initialToken = '', children }: TokenGateProps) {
  const [phase, setPhase] = useState<Phase>(() => (readToken() ? 'checking' : 'asking'))
  const [input, setInput] = useState(initialToken)
  const [error, setError] = useState('')

  useStoredTokenCheck(phase, verify, setPhase)

  const submit = useCallback(async () => {
    const parsed = tokenSchema.safeParse(input)
    if (!parsed.success) {
      // 明らかに短いものはサーバーへ送らない。往復する意味がない。
      setError(parsed.error.issues[0]?.message ?? 'トークンが不正です')
      return
    }

    setError('')
    setPhase('verifying')
    try {
      await verify(parsed.data)
    } catch (err) {
      setError(describeError(err))
      setPhase('asking')
      return
    }
    // 通ったものだけ保存する。
    writeToken(parsed.data)
    setPhase('open')
  }, [input, verify])

  if (phase === 'open') {
    return <>{children}</>
  }
  if (phase === 'checking') {
    return <CenteredNotice>保存されたトークンを確認しています…</CenteredNotice>
  }

  return (
    <TokenForm
      value={input}
      onChange={setInput}
      error={error}
      busy={phase === 'verifying'}
      onSubmit={submit}
    />
  )
}

/**
 * 保存済みのトークンを起動時に一度だけ確かめる。
 *
 * .env の ADMIN_TOKEN を入れ替えたときは、ここで失効に気づく。
 */
function useStoredTokenCheck(
  phase: Phase,
  verify: (token: string) => Promise<void>,
  setPhase: Dispatch<SetStateAction<Phase>>,
) {
  useEffect(() => {
    if (phase !== 'checking') {
      return
    }
    let cancelled = false

    void verify(readToken())
      .then(() => {
        if (!cancelled) {
          setPhase('open')
        }
      })
      .catch(() => {
        if (cancelled) {
          return
        }
        // 通らないトークンを残しておく意味はない。
        clearToken()
        setPhase('asking')
      })

    return () => {
      cancelled = true
    }
  }, [phase, verify, setPhase])
}

type TokenFormProps = {
  value: string
  onChange: (value: string) => void
  error: string
  busy: boolean
  onSubmit: () => Promise<void>
}

/** 入口の端末。光るブロックを掲げ、ここから先が管理画面だと分かるようにする。 */
function TokenForm({ value, onChange, error, busy, onSubmit }: TokenFormProps) {
  return (
    <div className="flex min-h-full items-center justify-center p-4">
      <form
        className="hud-panel flex w-full max-w-md flex-col gap-6 p-7"
        onSubmit={(e) => {
          e.preventDefault()
          void onSubmit()
        }}
      >
        <div className="flex items-center gap-4">
          <PixelIcon
            name="block"
            className="size-14 shrink-0 text-emerald drop-shadow-[0_0_16px_var(--color-emerald)]"
          />
          <div className="flex flex-col gap-2">
            <p aria-hidden="true" className="hud-tag">
              ACCESS TERMINAL
            </p>
            <h1 className="font-pixel text-xl leading-snug break-keep text-fg">
              Minecraft サーバー管理コンソール
            </h1>
          </div>
        </div>

        <p className="text-xs text-faint">.env の ADMIN_TOKEN を入力してください。</p>

        <FormField
          id="admin-token"
          label="トークン"
          value={value}
          onChange={onChange}
          error={error}
          hint="openssl rand -hex 32 で生成したもの"
          disabled={busy}
          autoFocus
        />

        <Button type="submit" tone="primary" size="lg" disabled={busy}>
          {busy ? '確認しています…' : '入る'}
        </Button>

        <div aria-hidden="true" className="hud-tag flex justify-between border-t border-line pt-4">
          <span>AUTH // SHARED TOKEN</span>
          <span>TAILNET</span>
        </div>
      </form>
    </div>
  )
}

function CenteredNotice({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-full items-center justify-center gap-3 p-4 text-sm text-dim">
      <Spinner />
      <span>{children}</span>
    </div>
  )
}
