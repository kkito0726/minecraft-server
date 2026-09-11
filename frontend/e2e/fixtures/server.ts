import { spawn } from 'node:child_process'
import type { ChildProcess } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { FAKE_DOCKER, MCADMIND, createProject } from './project'

/** 起動した mcadmind 1 つ。 */
export type TestServer = {
  url: string
  /** プロジェクトディレクトリ。試験から中身を覗くために公開する。 */
  dir: string
}

export type StartOptions = {
  readyDelaySeconds?: number
  worlds?: string[]
}

/**
 * 試験ごとに mcadmind を 1 つ起動する。
 *
 * 使い回さないのは、どの試験も .env とワールドとバックアップを書き換え、
 * 操作の履歴がプロセス内に残るため。前の試験の残骸の上で動く試験は、
 * 落ちたときに原因が読めない。docker を叩かないので起動は一瞬で済む。
 */
export async function startServer(opts: StartOptions = {}): Promise<TestServer & { stop: () => void }> {
  const dir = mkdtempSync(join(tmpdir(), 'mcadmind-e2e-'))
  const port = await freePort()
  const addr = `127.0.0.1:${port}`

  createProject({ dir, addr, ...opts })

  const child = spawn(MCADMIND, ['-project-dir', dir, '-log-level', process.env.E2E_DEBUG ? 'debug' : 'warn'], {
    env: {
      ...process.env,
      FAKE_DOCKER_STATE: join(dir, '.fake-docker'),
      FAKE_DOCKER_READY_DELAY: String(opts.readyDelaySeconds ?? 0),
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })

  // 調査用。E2E_DEBUG=1 で mcadmind の出力をそのまま流す。
  if (process.env.E2E_DEBUG) {
    child.stdout?.on('data', (c: Buffer) => process.stderr.write(`[mcadmind] ${c}`))
    child.stderr?.on('data', (c: Buffer) => process.stderr.write(`[mcadmind] ${c}`))
  }

  const url = `http://${addr}`
  try {
    await waitForHTTP(url, child)
  } catch (err) {
    stopChild(child)
    rmSync(dir, { recursive: true, force: true })
    throw err
  }

  return {
    url,
    dir,
    stop: () => {
      stopChild(child)
      rmSync(dir, { recursive: true, force: true })
    },
  }
}

/** fake-docker のパス。試験から状態を確かめたいときに使う。 */
export { FAKE_DOCKER }

function stopChild(child: ChildProcess) {
  if (child.exitCode === null && child.signalCode === null) {
    child.kill('SIGTERM')
  }
}

/**
 * 空いているポートを Node 側で選ぶ。
 *
 * mcadmind に :0 を渡しても、ログに出るのは設定した文字列であって
 * 実際に割り当てられたポートではないため、こちらで決めて渡す。
 */
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = createServer()
    srv.on('error', reject)
    srv.listen(0, '127.0.0.1', () => {
      const address = srv.address()
      if (address === null || typeof address === 'string') {
        srv.close(() => reject(new Error('ポートを決められませんでした')))
        return
      }
      const { port } = address
      srv.close(() => resolve(port))
    })
  })
}

async function waitForHTTP(url: string, child: ChildProcess): Promise<void> {
  const stderr: string[] = []
  child.stderr?.on('data', (chunk: Buffer) => stderr.push(chunk.toString()))

  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`mcadmind が起動直後に終了しました\n${stderr.join('')}`)
    }
    try {
      const res = await fetch(url)
      if (res.ok) {
        return
      }
    } catch {
      // まだ待ち受けていない
    }
    await sleep(100)
  }
  throw new Error(`mcadmind が起動しませんでした\n${stderr.join('')}`)
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
