/**
 * SystemService のデモ実装。
 *
 * 公開デモはブラウザの中で完結するので、本物の /proc は読めない。
 * 値は Pi 5 (4GB) の実機に近い形で作り、取得のたびに少し揺らす。
 * 動かないままの数字を出すと、取得が止まっているように見える。
 */
import type { ServiceImpl } from '@connectrpc/connect'

import { SystemService } from '../gen/mcadmin/v1/system_pb'

const TOTAL_MEMORY = 4 * 1024 ** 3
const TOTAL_STORAGE = 128 * 1024 ** 3

/** 中心値の周りで揺らす。デモなので乱数でよい。 */
function jitter(center: number, spread: number): number {
  return center + (Math.random() - 0.5) * spread
}

export const systemImpl: Partial<ServiceImpl<typeof SystemService>> = {
  getMetrics() {
    const cpuPercent = Math.min(100, Math.max(0, jitter(28, 18)))
    const memoryPercent = Math.min(100, Math.max(0, jitter(72, 6)))
    const usedMemory = Math.round((TOTAL_MEMORY * memoryPercent) / 100)
    const usedStorage = Math.round(TOTAL_STORAGE * 0.31)

    return {
      cpu: {
        available: true,
        usedPercent: cpuPercent,
        windowSeconds: 5,
        cores: 4,
        load1: Number(jitter(1.1, 0.4).toFixed(2)),
        load5: 0.92,
        load15: 0.78,
        unavailableReason: '',
      },
      memory: {
        available: true,
        totalBytes: BigInt(TOTAL_MEMORY),
        availableBytes: BigInt(TOTAL_MEMORY - usedMemory),
        usedBytes: BigInt(usedMemory),
        usedPercent: memoryPercent,
        // スワップは無効にする前提（docs/raspberry-pi.md）。
        swapTotalBytes: 0n,
        swapUsedBytes: 0n,
        unavailableReason: '',
      },
      storage: {
        available: true,
        path: '/home/pi/minecraft-server',
        totalBytes: BigInt(TOTAL_STORAGE),
        availableBytes: BigInt(TOTAL_STORAGE - usedStorage),
        usedBytes: BigInt(usedStorage),
        usedPercent: (usedStorage / TOTAL_STORAGE) * 100,
        unavailableReason: '',
      },
    }
  },
}
