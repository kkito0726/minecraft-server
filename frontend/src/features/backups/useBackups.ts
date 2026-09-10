import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { UseQueryResult } from '@tanstack/react-query'

import type {
  BackupMode,
  GetRetentionPolicyResponse,
  ListBackupsResponse,
  PreflightRestoreResponse,
  RestoreTarget,
} from '../../gen/mcadmin/v1/backup_pb'
import { queryKeys } from '../../lib/queryKeys'
import { useOperation } from '../operations'
import { backupClient } from './client'
import type { RetentionInput } from './retention'

export function useBackups(): UseQueryResult<ListBackupsResponse> {
  return useQuery({
    queryKey: queryKeys.backups,
    queryFn: () => backupClient.listBackups({}),
  })
}

export function useRetentionPolicy(): UseQueryResult<GetRetentionPolicyResponse> {
  return useQuery({
    queryKey: queryKeys.retention,
    queryFn: () => backupClient.getRetentionPolicy({}),
  })
}

export type CreateBackupInput = { mode: BackupMode; note: string }

/** 取得は操作になる。返ってきた操作をその場で購読へ差し込む。 */
export function useCreateBackup() {
  const { begin } = useOperation()

  return useMutation({
    mutationFn: (input: CreateBackupInput) => backupClient.createBackup(input),
    onSuccess: (res) => {
      if (res.operation) {
        begin(res.operation)
      }
    },
  })
}

/**
 * 事前確認（REQ-008）。副作用を持たないので、何度呼んでも構わない。
 *
 * 一覧と一緒にキャッシュしてはいけない。ダイアログを開いたまま
 * .env やワールドが変われば判定も変わる。古い判定を見せたまま
 * 承諾させると、承諾の意味が無くなる。
 */
export function usePreflightRestore(
  backupId: string | null,
): UseQueryResult<PreflightRestoreResponse> {
  return useQuery({
    queryKey: [...queryKeys.preflight, backupId],
    queryFn: () => backupClient.preflightRestore({ backupId: backupId ?? '' }),
    enabled: backupId !== null,
    staleTime: 0,
    gcTime: 0,
    refetchOnMount: 'always',
  })
}

export type RestoreInput = {
  backupId: string
  target: RestoreTarget
  acknowledgeVersionWarning: boolean
  confirmLevelName: string
}

export function useRestoreBackup() {
  const { begin } = useOperation()

  return useMutation({
    mutationFn: (input: RestoreInput) => backupClient.restoreBackup(input),
    onSuccess: (res) => {
      if (res.operation) {
        begin(res.operation)
      }
    },
  })
}

/**
 * 削除・保持設定・手動適用は操作にならない。
 *
 * どれも一瞬で終わり、ワールドの実体を触らない。操作にすると
 * 「同時に 1 つだけ」の枠を消費して、他の操作を待たせてしまう。
 */
export function useDeleteBackup() {
  const invalidate = useInvalidateBackups()

  return useMutation({
    mutationFn: (backupId: string) => backupClient.deleteBackup({ backupId }),
    onSuccess: invalidate,
  })
}

export function useSetRetentionPolicy() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (policy: RetentionInput) => backupClient.setRetentionPolicy({ policy }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.retention }),
  })
}

export function usePruneBackups() {
  const invalidate = useInvalidateBackups()

  return useMutation({
    mutationFn: (dryRun: boolean) => backupClient.pruneBackups({ dryRun }),
    onSuccess: (_res, dryRun) => (dryRun ? undefined : invalidate()),
  })
}

function useInvalidateBackups() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: queryKeys.backups })
}
