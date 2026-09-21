import { useState } from 'react'

import {
  QuarantineList,
  WorldCreateDialog,
  WorldDeleteDialog,
  WorldNameDialog,
  WorldTable,
} from '../components/organisms'
import type { WorldCreateInput } from '../components/organisms'
import { Button, Spinner } from '../components/atoms'
import { PanelHeader } from '../components/molecules'
import { useGameSettings } from '../features/settings'
import { useOperation } from '../features/operations'
import { usePurgeQuarantine, useWorldCommand, useWorlds } from '../features/worlds'
import { describeError } from '../lib/errors'
import { useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '../lib/queryKeys'

/** 開いているダイアログ。1 つだけ開く。 */
type Dialog =
  | { kind: 'create' }
  | { kind: 'clone'; source: string }
  | { kind: 'rename'; from: string }
  | { kind: 'delete'; name: string }
  | null

/**
 * ワールドの管理（REQ-011〜REQ-016）。
 *
 * 切替はサーバーの再作成を伴うが、切替前のワールドは 1 バイトも動かない。
 * data/<名前>/ にそのまま残り、いつでも戻せる。
 */
export function WorldsPage() {
  const { isBusy } = useOperation()
  const { data, isPending, error } = useWorlds()
  const command = useWorldCommand()
  const purge = usePurgeQuarantine()
  const queryClient = useQueryClient()
  const [dialog, setDialog] = useState<Dialog>(null)

  const close = () => setDialog(null)
  const send = (c: Parameters<typeof command.mutate>[0]) => {
    command.mutate(c)
    close()
  }

  if (isPending) {
    return <p className="text-sm text-dim">ワールドを読み込んでいます…</p>
  }
  if (error) {
    return <p className="text-sm text-danger-ink">{describeError(error)}</p>
  }

  return (
    <div className="flex flex-col gap-5">
      <section className="hud-panel flex flex-col gap-4">
        <Header disabled={isBusy} onCreate={() => setDialog({ kind: 'create' })} />

        {(command.error || purge.error) && (
          <p className="text-sm text-danger-ink">
            {describeError(command.error ?? purge.error)}
          </p>
        )}

        <WorldTable
          worlds={data.worlds}
          disabled={isBusy}
          onSwitch={(name) => command.mutate({ kind: 'switch', name })}
          onClone={(source) => setDialog({ kind: 'clone', source })}
          onRename={(from) => setDialog({ kind: 'rename', from })}
          onDelete={(name) => setDialog({ kind: 'delete', name })}
        />

        {dialog && <ActiveDialog dialog={dialog} disabled={isBusy} onCancel={close} onSend={send} />}
      </section>

      <QuarantineList
        quarantines={data.quarantines}
        disabled={isBusy}
        onPurge={(name) =>
          purge.mutate(name, {
            onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.worlds }),
          })
        }
      />
    </div>
  )
}

function Header({ disabled, onCreate }: { disabled: boolean; onCreate: () => void }) {
  return (
    <PanelHeader title="ワールド" tag="WORLDS // SAVES">
      <Button tone="primary" disabled={disabled} onClick={onCreate}>
        新規作成
      </Button>
    </PanelHeader>
  )
}

type ActiveDialogProps = {
  dialog: NonNullable<Dialog>
  disabled: boolean
  onCancel: () => void
  onSend: (c: Parameters<ReturnType<typeof useWorldCommand>['mutate']>[0]) => void
}

function ActiveDialog({ dialog, disabled, onCancel, onSend }: ActiveDialogProps) {
  switch (dialog.kind) {
    case 'create':
      return (
        <CreateDialog
          disabled={disabled}
          onCancel={onCancel}
          onSubmit={(input) => onSend({ kind: 'create', ...input })}
        />
      )
    case 'clone':
      return (
        <WorldNameDialog
          title={`${dialog.source} の複製`}
          description={`${dialog.source} を別の名前で複製します。サーバーは止まりません。`}
          submitLabel="複製する"
          disabled={disabled}
          onCancel={onCancel}
          onSubmit={(name) => onSend({ kind: 'clone', source: dialog.source, destination: name })}
        />
      )
    case 'rename':
      return (
        <WorldNameDialog
          title={`${dialog.from} の改名`}
          description={`${dialog.from} の名前を変えます。稼働中の場合はサーバーを再起動します。`}
          submitLabel="改名する"
          disabled={disabled}
          onCancel={onCancel}
          onSubmit={(name) => onSend({ kind: 'rename', from: dialog.from, to: name })}
        />
      )
    case 'delete':
      return (
        <WorldDeleteDialog
          name={dialog.name}
          disabled={disabled}
          onCancel={onCancel}
          onConfirm={(confirmName) => onSend({ kind: 'delete', name: dialog.name, confirmName })}
        />
      )
  }
}

/**
 * 新規作成のダイアログ。
 *
 * 初期選択を .env の現在値にするため、開いたときに設定を読む。
 * 読めるまで出さないのは、既定値で描いてから差し替わると、利用者が
 * 選んだつもりの無い値に切り替わって見えるため。
 */
function CreateDialog({
  disabled,
  onCancel,
  onSubmit,
}: {
  disabled: boolean
  onCancel: () => void
  onSubmit: (input: WorldCreateInput) => void
}) {
  const { data, isPending, error } = useGameSettings()

  if (isPending) {
    return (
      <div className="hud-inset flex items-center gap-2.5 text-sm text-dim">
        <Spinner label="設定を読み込んでいます" />
        <span>設定を読み込んでいます…</span>
      </div>
    )
  }
  if (error || !data.settings) {
    return (
      <p role="alert" className="hud-inset text-sm text-danger-ink">
        {error ? describeError(error) : 'ゲーム設定を受け取れませんでした。'}
      </p>
    )
  }

  return (
    <WorldCreateDialog
      currentMode={data.settings.mode}
      currentDifficulty={data.settings.difficulty}
      disabled={disabled}
      onCancel={onCancel}
      onSubmit={onSubmit}
    />
  )
}
