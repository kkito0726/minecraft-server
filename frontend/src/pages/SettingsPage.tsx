import { useState } from 'react'

import { GameSettingsForm } from '../components/organisms'
import { PanelHeader } from '../components/molecules'
import { useOperation } from '../features/operations'
import { useServerStatus } from '../features/server'
import { useGameSettings, useUpdateGameSettings, valuesFromProto } from '../features/settings'
import type { GameSettingsValues } from '../features/settings'
import { ContainerState } from '../gen/mcadmin/v1/server_pb'
import { describeError } from '../lib/errors'

/**
 * ゲーム設定（難易度・MOTD・人数・距離）。
 *
 * .env に書き込み、反映はサーバーの作り直しで行う。バージョン・サーバー種別・
 * メモリはここでは扱わない。片道のアップグレードや Pi が固まる状態を、
 * 画面のひと押しで起こせてしまうため。
 */
export function SettingsPage() {
  const { isBusy } = useOperation()
  const settings = useGameSettings()
  const status = useServerStatus()
  const { save, notice, error, isPending } = useSaveSettings()

  if (settings.isPending) {
    return <p className="text-sm text-dim">ゲーム設定を読み込んでいます…</p>
  }
  if (settings.error) {
    return <p className="text-sm text-danger-ink">{describeError(settings.error)}</p>
  }
  if (!settings.data.settings) {
    return <p className="text-sm text-danger-ink">ゲーム設定を受け取れませんでした。</p>
  }

  const values = valuesFromProto(settings.data.settings)

  return (
    <div className="flex flex-col gap-5">
      <section className="hud-panel flex flex-col gap-4">
        <PanelHeader title="ゲーム設定" tag="CONFIG // .ENV" />
        <p className="text-xs text-faint">
          .env に書き込みます。反映にはサーバーの作り直しが必要で、接続中の人は切断されます。
          バージョン・サーバー種別・メモリはここでは変えられません。
        </p>

        {error && (
          <p role="alert" className="text-sm text-danger-ink">
            {describeError(error)}
          </p>
        )}
        {notice && <p className="text-sm text-ok-ink">{notice}</p>}

        {/* 保存のあとに読み直した値で入力欄を作り直す。古い入力が残ると「未保存」に見える。 */}
        <GameSettingsForm
          key={JSON.stringify(values)}
          settings={values}
          warnings={settings.data.warnings}
          running={status.data?.containerState === ContainerState.RUNNING}
          onlinePlayers={Math.max(0, status.data?.onlinePlayers ?? 0)}
          disabled={isBusy || isPending}
          onSave={save}
        />
      </section>
    </div>
  )
}

/** 保存したが反映していない、と伝える文言。 */
const SAVED_NOTICE = '保存しました。まだ反映していません。次の起動・再起動で反映されます。'

/**
 * 保存と、その結果の案内。
 *
 * 作り直しが始まったなら、進捗は上の帯が伝える。ここで重ねて言わない。
 */
function useSaveSettings() {
  const update = useUpdateGameSettings()
  const [notice, setNotice] = useState<string | null>(null)

  const save = (next: GameSettingsValues, applyNow: boolean) => {
    setNotice(null)
    update.mutate(
      { settings: next, applyNow },
      { onSuccess: (res) => setNotice(res.operation ? null : SAVED_NOTICE) },
    )
  }

  return { save, notice, error: update.error, isPending: update.isPending }
}
