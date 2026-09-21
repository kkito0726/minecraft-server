import { useState } from 'react'

import { DIFFICULTY_OPTIONS, GAME_MODE_OPTIONS } from '../../features/settings'
import { isValidWorldName, worldNameError } from '../../features/worlds'
import { Difficulty, GameMode } from '../../gen/mcadmin/v1/server_pb'
import { Button } from '../atoms'
import { CheckboxField, ChoiceGroup, ConfirmInput, FormField } from '../molecules'
import { isConfirmed } from '../molecules/confirmation'

/**
 * ワールドの新規作成。
 *
 * 名前とシードに加えて、ゲームモード・難易度・ハードコアを決める。
 * ここで受け取るのは、この 3 つが効くのが生成の瞬間だから。生成後に
 * 変えても、モードは新しく接続した人にしか効かず、ハードコアは
 * level.dat に焼かれた値と食い違う。
 *
 * **いずれもサーバー全体の設定で、ワールドごとの属性ではない。**
 * 切り替えても付いてこないことを、選ばせる場所で必ず伝える。
 */
export type WorldCreateInput = {
  name: string
  seed: string
  mode: GameMode
  difficulty: Difficulty
  hardcore: boolean
}

export type WorldCreateDialogProps = {
  /** いま .env に入っている値。初期選択にする。 */
  currentMode: GameMode
  currentDifficulty: Difficulty
  disabled?: boolean | undefined
  onCancel: () => void
  onSubmit: (input: WorldCreateInput) => void
}

const DESCRIPTION =
  '新しいワールドを作ります。サーバーを再起動して生成するため、数分かかることがあります。'

export function WorldCreateDialog({
  currentMode,
  currentDifficulty,
  disabled,
  onCancel,
  onSubmit,
}: WorldCreateDialogProps) {
  const f = useCreateForm(currentMode, currentDifficulty)

  return (
    <form
      className={`hud-inset flex flex-col gap-4 ${f.hardcore ? 'hud-inset-danger' : ''}`}
      aria-label="ワールドの新規作成"
      onSubmit={(e) => {
        e.preventDefault()
        if (f.ready) {
          onSubmit(f.values())
        }
      }}
    >
      <p className="text-sm text-dim">{DESCRIPTION}</p>

      <Fields form={f} disabled={disabled} />

      <Actions
        hardcore={f.hardcore}
        disabled={!f.ready || disabled}
        cancelDisabled={disabled}
        onCancel={onCancel}
      />
    </form>
  )
}

function Actions({
  hardcore,
  disabled,
  cancelDisabled,
  onCancel,
}: {
  hardcore: boolean
  disabled: boolean | undefined
  cancelDisabled: boolean | undefined
  onCancel: () => void
}) {
  return (
    <div className="flex gap-2">
      <Button type="submit" tone={hardcore ? 'danger' : 'primary'} disabled={disabled}>
        作成する
      </Button>
      <Button onClick={onCancel} disabled={cancelDisabled}>
        やめる
      </Button>
    </div>
  )
}

/** 入力欄をまとめる。ダイアログ本体は骨組みだけを持つ。 */
function Fields({
  form: f,
  disabled,
}: {
  form: ReturnType<typeof useCreateForm>
  disabled: boolean | undefined
}) {
  return (
    <>
      <NameAndSeed
        name={f.name}
        seed={f.seed}
        disabled={disabled}
        onName={f.setName}
        onSeed={f.setSeed}
      />
      <ModeAndDifficulty
        mode={f.mode}
        difficulty={f.difficulty}
        disabled={disabled}
        onMode={f.setMode}
        onDifficulty={f.setDifficulty}
      />
      <HardcoreField
        checked={f.hardcore}
        disabled={disabled}
        name={f.name}
        typed={f.typed}
        onToggle={f.toggleHardcore}
        onTyped={f.setTyped}
      />
    </>
  )
}

/**
 * 入力の状態。
 *
 * ハードコアを外したら確認の入力も捨てる。残しておくと、入れ直した
 * ときに確認済みの状態から始まってしまう。
 */
function useCreateForm(currentMode: GameMode, currentDifficulty: Difficulty) {
  const [name, setName] = useState('')
  const [seed, setSeed] = useState('')
  const [mode, setMode] = useState(currentMode)
  const [difficulty, setDifficulty] = useState(currentDifficulty)
  const [hardcore, setHardcore] = useState(false)
  const [typed, setTyped] = useState('')

  // ハードコアは後から外せない。削除と同じく、名前の完全一致を要求する。
  const confirmed = !hardcore || isConfirmed(name, typed)

  return {
    name,
    seed,
    mode,
    difficulty,
    hardcore,
    typed,
    ready: isValidWorldName(name) && confirmed,
    setName,
    setSeed,
    setMode,
    setDifficulty,
    setTyped,
    toggleHardcore: (next: boolean) => {
      setHardcore(next)
      setTyped('')
    },
    values: (): WorldCreateInput => ({ name, seed, mode, difficulty, hardcore }),
  }
}

function NameAndSeed({
  name,
  seed,
  disabled,
  onName,
  onSeed,
}: {
  name: string
  seed: string
  disabled: boolean | undefined
  onName: (v: string) => void
  onSeed: (v: string) => void
}) {
  return (
    <>
      <FormField
        id="world-name"
        label="ワールド名"
        value={name}
        onChange={onName}
        error={worldNameError(name)}
        hint="英数字で始まる 1〜32 文字（英数字・_・-）"
        disabled={disabled}
        autoFocus
      />
      <FormField
        id="world-seed"
        label="シード（任意）"
        value={seed}
        onChange={onSeed}
        hint="空にすると Paper が決めます"
        disabled={disabled}
      />
    </>
  )
}

/**
 * ゲームモードと難易度。
 *
 * サーバー全体の設定であることを必ず添える。ここで選んだ値は .env に
 * 書かれ、別のワールドに切り替えても戻らない。
 */
function ModeAndDifficulty({
  mode,
  difficulty,
  disabled,
  onMode,
  onDifficulty,
}: {
  mode: GameMode
  difficulty: Difficulty
  disabled: boolean | undefined
  onMode: (v: GameMode) => void
  onDifficulty: (v: Difficulty) => void
}) {
  return (
    <>
      <ChoiceGroup
        name="create-mode"
        legend="ゲームモード"
        value={String(mode)}
        disabled={disabled}
        onChange={(v) => onMode(Number(v) as GameMode)}
        choices={GAME_MODE_OPTIONS.map((o) => ({
          value: String(o.value),
          label: o.label,
          hint: o.hint,
        }))}
      />
      <ChoiceGroup
        name="create-difficulty"
        legend="難易度"
        value={String(difficulty)}
        disabled={disabled}
        onChange={(v) => onDifficulty(Number(v) as Difficulty)}
        choices={DIFFICULTY_OPTIONS.map((o) => ({
          value: String(o.value),
          label: o.label,
          hint: o.hint,
        }))}
      />
      <p className="text-xs text-faint">
        ゲームモードと難易度はサーバー全体の設定です。ここで選んだ値は .env
        に書かれ、別のワールドに切り替えても戻りません。
      </p>
    </>
  )
}

type HardcoreFieldProps = {
  checked: boolean
  disabled: boolean | undefined
  name: string
  typed: string
  onToggle: (checked: boolean) => void
  onTyped: (value: string) => void
}

/**
 * ハードコアの選択。
 *
 * 生成されたワールドの level.dat に焼かれるため、後から外せない。
 * 押し間違いがそのまま取り返しのつかない設定になるので、削除と同じく
 * 名前の完全一致を要求する（チェックを入れたときだけ出す）。
 */
function HardcoreField({ checked, disabled, name, typed, onToggle, onTyped }: HardcoreFieldProps) {
  return (
    <div className="flex flex-col gap-2.5">
      <CheckboxField
        id="world-hardcore"
        label="ハードコアにする"
        checked={checked}
        disabled={disabled}
        onChange={onToggle}
      />
      {checked && (
        <>
          <p className="text-sm leading-relaxed text-danger-ink">
            死亡すると復帰できません。難易度はハードに固定され、
            <strong className="font-semibold text-fg">この設定は後から外せません</strong>
            （生成時にワールドへ書き込まれます）。
          </p>
          <ConfirmInput
            id="hardcore-confirm"
            expected={name}
            value={typed}
            onChange={onTyped}
            disabled={disabled || name === ''}
          />
        </>
      )}
    </div>
  )
}
