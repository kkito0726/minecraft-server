import { useState } from 'react'

import {
  DIFFICULTY_OPTIONS,
  PI_RECOMMENDED,
  SETTINGS_LIMITS,
  isChanged,
  parseGameSettings,
  piLoadNotes,
  toForm,
} from '../../features/settings'
import type { GameSettingsForm as FormValues, GameSettingsValues } from '../../features/settings'
import type { Difficulty } from '../../gen/mcadmin/v1/server_pb'
import { Button } from '../atoms'
import { ChoiceGroup, FormField } from '../molecules'

/**
 * ゲーム設定の入力。
 *
 * 保存と反映を分ける。反映はサーバーの作り直しを伴い、接続中の人を切断する。
 * 夜のうちに設定だけ入れておく、という使い方を塞がないため、
 * 「保存だけ」と「今すぐ反映」を並べて選ばせる。
 */
export type GameSettingsFormProps = {
  settings: GameSettingsValues
  /** .env に読めない値があったときの説明。 */
  warnings: string[]
  /** サーバーが動いているか。止まっていれば今すぐ反映の口を出さない。 */
  running: boolean
  onlinePlayers: number
  disabled?: boolean | undefined
  onSave: (values: GameSettingsValues, applyNow: boolean) => void
}

export function GameSettingsForm({
  settings,
  warnings,
  running,
  onlinePlayers,
  disabled,
  onSave,
}: GameSettingsFormProps) {
  const { form, parsed, canSave, confirming, update, save, cancel } = useSettingsForm(
    settings,
    onlinePlayers,
    disabled,
    onSave,
  )

  return (
    <form
      aria-label="ゲーム設定"
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault()
        save(false)
      }}
    >
      <Warnings warnings={warnings} />
      <DifficultyField value={form.difficulty} disabled={disabled} onChange={(difficulty) => update({ difficulty })} />
      <TextFields form={form} disabled={disabled} onChange={update} />
      {!parsed.ok && (
        <p role="alert" className="text-sm text-danger-ink">
          {parsed.message}
        </p>
      )}
      {parsed.ok && <LoadNotes notes={piLoadNotes(parsed.value)} />}
      <Actions
        running={running}
        onlinePlayers={onlinePlayers}
        confirming={confirming}
        canSave={canSave}
        onApply={() => save(true)}
        onCancel={cancel}
      />
    </form>
  )
}

/**
 * 入力と確認の状態。
 *
 * 人がいる状態での作り直しは接続を切るので、今すぐ反映は一度確かめる。
 * 入力を変えたら確認は取り消す。確かめた内容と送る内容がずれるため。
 */
function useSettingsForm(
  settings: GameSettingsValues,
  onlinePlayers: number,
  disabled: boolean | undefined,
  onSave: (values: GameSettingsValues, applyNow: boolean) => void,
) {
  const original = toForm(settings)
  const [form, setForm] = useState(original)
  const [confirming, setConfirming] = useState(false)
  const parsed = parseGameSettings(form)

  return {
    form,
    parsed,
    confirming,
    canSave: !disabled && parsed.ok && isChanged(form, original),
    update: (patch: Partial<FormValues>) => {
      setForm({ ...form, ...patch })
      setConfirming(false)
    },
    cancel: () => setConfirming(false),
    save: (applyNow: boolean) => {
      if (!parsed.ok) {
        return
      }
      if (applyNow && onlinePlayers > 0 && !confirming) {
        setConfirming(true)
        return
      }
      setConfirming(false)
      onSave(parsed.value, applyNow)
    },
  }
}

function Warnings({ warnings }: { warnings: string[] }) {
  if (warnings.length === 0) {
    return null
  }
  return (
    <div className="notice notice-warn flex flex-col gap-1.5">
      <p className="font-semibold">.env の一部を読めませんでした</p>
      <ul className="list-disc pl-5 text-xs">
        {warnings.map((w) => (
          <li key={w}>{w}</li>
        ))}
      </ul>
    </div>
  )
}

function DifficultyField({
  value,
  disabled,
  onChange,
}: {
  value: Difficulty
  disabled: boolean | undefined
  onChange: (value: Difficulty) => void
}) {
  return (
    <ChoiceGroup
      name="difficulty"
      legend="難易度"
      value={String(value)}
      disabled={disabled}
      onChange={(v) => onChange(Number(v) as Difficulty)}
      choices={DIFFICULTY_OPTIONS.map((o) => ({ value: String(o.value), label: o.label, hint: o.hint }))}
    />
  )
}

function TextFields({
  form,
  disabled,
  onChange,
}: {
  form: FormValues
  disabled: boolean | undefined
  onChange: (patch: Partial<FormValues>) => void
}) {
  const { minMaxPlayers, maxMaxPlayers, minDistance, maxDistance, maxMotdLength } = SETTINGS_LIMITS

  return (
    <>
      <FormField
        id="settings-motd"
        label="MOTD（サーバー一覧に出る説明）"
        value={form.motd}
        onChange={(motd) => onChange({ motd })}
        disabled={disabled}
        hint={`${maxMotdLength} 文字まで。§ の色コードは使えますが、" $ \` \\ と改行は使えません`}
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <FormField
          id="settings-max-players"
          label="最大人数"
          value={form.maxPlayers}
          onChange={(maxPlayers) => onChange({ maxPlayers })}
          disabled={disabled}
          hint={`${minMaxPlayers}〜${maxMaxPlayers}。Pi 5 の目安は ${PI_RECOMMENDED.maxPlayers} 人まで`}
        />
        <FormField
          id="settings-view-distance"
          label="描画距離"
          value={form.viewDistance}
          onChange={(viewDistance) => onChange({ viewDistance })}
          disabled={disabled}
          hint={`${minDistance}〜${maxDistance} チャンク。目安は ${PI_RECOMMENDED.viewDistance} まで`}
        />
        <FormField
          id="settings-simulation-distance"
          label="シミュレーション距離"
          value={form.simulationDistance}
          onChange={(simulationDistance) => onChange({ simulationDistance })}
          disabled={disabled}
          hint={`描画距離以下。目安は ${PI_RECOMMENDED.simulationDistance} まで`}
        />
      </div>
    </>
  )
}

function LoadNotes({ notes }: { notes: string[] }) {
  if (notes.length === 0) {
    return null
  }
  return (
    <ul className="notice notice-warn flex list-disc flex-col gap-1 pl-8 text-xs">
      {notes.map((n) => (
        <li key={n}>{n}</li>
      ))}
    </ul>
  )
}

type ActionsProps = {
  running: boolean
  onlinePlayers: number
  confirming: boolean
  canSave: boolean
  onApply: () => void
  onCancel: () => void
}

function Actions({ running, onlinePlayers, confirming, canSave, onApply, onCancel }: ActionsProps) {
  return (
    <div className="flex flex-col gap-3 border-t border-line pt-4">
      {confirming && (
        <div className="notice notice-warn flex flex-col gap-3">
          <p>{onlinePlayers} 人が接続しています。反映するとサーバーを作り直すため、全員が切断されます。</p>
          <div className="flex gap-2">
            <Button tone="danger" onClick={onApply}>
              切断して反映する
            </Button>
            <Button onClick={onCancel}>やめる</Button>
          </div>
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        {running && (
          <Button tone="primary" disabled={!canSave} onClick={onApply}>
            保存して今すぐ反映
          </Button>
        )}
        <Button type="submit" disabled={!canSave}>
          {running ? '保存だけする' : '保存する'}
        </Button>
      </div>
      <p className="text-xs text-faint">
        {running
          ? '「保存だけする」と、次の起動・再起動で反映されます。今すぐ反映するとサーバーを作り直します。'
          : 'サーバーは停止中です。保存した設定は次の起動で反映されます。'}
      </p>
    </div>
  )
}
