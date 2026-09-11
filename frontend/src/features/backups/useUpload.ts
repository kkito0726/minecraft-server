import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { queryKeys } from '../../lib/queryKeys'
import { uploadBackup } from './upload'
import type { UploadResult } from './upload'

/**
 * zip の取り込み。
 *
 * 操作（Operation）にはしない。ワールドを触らないので排他の枠を
 * 消費する理由が無く、待たせると取り込み中に他の操作ができなくなる。
 * 進捗はサーバーの進捗ストリームではなく、送信そのものから取る。
 */
export function useUploadBackup() {
  const queryClient = useQueryClient()
  const [progress, setProgress] = useState<number | null>(null)

  const mutation = useMutation<UploadResult, Error, File>({
    mutationFn: (file) => {
      setProgress(0)
      return uploadBackup(file, { onProgress: setProgress })
    },
    onSettled: () => setProgress(null),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.backups }),
  })

  return { ...mutation, progress }
}

/** 取り込みの結果を 1 行の案内にする。 */
export function uploadNotice(result: UploadResult | undefined): string | undefined {
  if (!result) {
    return undefined
  }
  const version = result.version ? `（${result.version}）` : '（バージョン不明）'
  // 包み直したことは伝える。利用者が渡した zip とは中身の並びが変わるため。
  const wrapped = result.rewrapped ? 'data/ 配下へ包み直しました。' : ''
  return `${result.level}${version}を取り込みました。${wrapped}一覧から復元できます。`
}
