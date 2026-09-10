import { useState } from 'react'

import {
  QuarantineList,
  WorldDeleteDialog,
  WorldNameDialog,
  WorldTable,
} from '../components/organisms'
import { Button } from '../components/atoms'
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
    return <p className="text-sm text-gray-600">ワールドを読み込んでいます…</p>
  }
  if (error) {
    return <p className="text-sm text-danger-700">{describeError(error)}</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <section className="flex flex-col gap-3 rounded border border-gray-200 bg-white p-4">
        <Header disabled={isBusy} onCreate={() => setDialog({ kind: 'create' })} />

        {(command.error || purge.error) && (
          <p className="text-sm text-danger-700">
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
    <div className="flex items-center gap-2">
      <h2 className="text-sm font-semibold text-gray-900">ワールド</h2>
      <div className="ml-auto">
        <Button tone="primary" disabled={disabled} onClick={onCreate}>
          新規作成
        </Button>
      </div>
    </div>
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
        <WorldNameDialog
          title="ワールドの新規作成"
          description="新しいワールドを作ります。サーバーを再起動して生成するため、数分かかることがあります。"
          submitLabel="作成する"
          withSeed
          disabled={disabled}
          onCancel={onCancel}
          onSubmit={(name, seed) => onSend({ kind: 'create', name, seed })}
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
