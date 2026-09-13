/// <reference types="vite/client" />

/**
 * デモ用のビルドかどうか。
 *
 * vite.config.ts の define がビルド時に真偽値へ置き換える。定数に
 * 畳まれるので、通常のビルドではデモの実装ごと成果物から落ちる。
 */
declare const __DEMO__: boolean
