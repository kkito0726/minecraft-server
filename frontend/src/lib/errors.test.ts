import { Code, ConnectError } from '@connectrpc/connect'
import { describe, expect, it } from 'vitest'

import { describeError, isUnauthenticated } from './errors'

describe('describeError', () => {
  // バックエンドは利用者が対処できるエラーだけ日本語のメッセージを載せてくる。
  // それを握り潰して定型文に置き換えると、対処できるはずの問題が分からなくなる。
  it('サーバーからのメッセージをそのまま出す', () => {
    const err = new ConnectError('確認の名前が一致していません', Code.InvalidArgument)
    expect(describeError(err)).toBe('確認の名前が一致していません')
  })

  it.each([
    [Code.Unauthenticated, '認証'],
    [Code.PermissionDenied, '認証'],
    [Code.Unavailable, '接続'],
    [Code.DeadlineExceeded, '時間'],
  ])('メッセージが無いときは %s に応じた案内を出す', (code, word) => {
    expect(describeError(new ConnectError('', code))).toContain(word)
  })

  it('Connect 以外のエラーもメッセージを拾う', () => {
    expect(describeError(new Error('ネットワークが切れました'))).toBe('ネットワークが切れました')
  })

  it('文字列でもオブジェクトでも落ちない', () => {
    expect(describeError('こわれた')).toBe('こわれた')
    expect(describeError(null)).not.toBe('')
    expect(describeError({ とても: '変な形' })).not.toBe('')
  })
})

describe('isUnauthenticated', () => {
  it.each([
    ['Unauthenticated', new ConnectError('', Code.Unauthenticated), true],
    ['PermissionDenied', new ConnectError('', Code.PermissionDenied), true],
    ['NotFound', new ConnectError('', Code.NotFound), false],
    ['ただのエラー', new Error('x'), false],
    ['null', null, false],
  ])('%s → %s', (_name, err, want) => {
    expect(isUnauthenticated(err)).toBe(want)
  })
})
