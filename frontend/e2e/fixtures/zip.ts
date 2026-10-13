import { deflateRawSync, inflateRawSync } from 'node:zlib'

/**
 * zip を組み立てる。
 *
 * 取り込みの試験で、形の違うアーカイブを何種類も作る必要がある。
 * 依存を増やさずに済ませたいので、必要な範囲だけを自前で書く
 * （無圧縮と deflate、ディレクトリエントリなし）。
 */
export function makeZip(files: Record<string, string | Buffer>): Buffer {
  const locals: Buffer[] = []
  const centrals: Buffer[] = []
  let offset = 0

  for (const [name, content] of Object.entries(files)) {
    const nameBytes = Buffer.from(name, 'utf8')
    const body = Buffer.isBuffer(content) ? content : Buffer.from(content, 'utf8')
    const compressed = deflateRawSync(body)
    const crc = crc32(body)

    const local = Buffer.alloc(30 + nameBytes.length)
    local.writeUInt32LE(0x04034b50, 0)
    local.writeUInt16LE(20, 4) // 展開に必要なバージョン
    local.writeUInt16LE(0x0800, 6) // ファイル名は UTF-8
    local.writeUInt16LE(8, 8) // deflate
    local.writeUInt32LE(crc, 14)
    local.writeUInt32LE(compressed.length, 18)
    local.writeUInt32LE(body.length, 22)
    local.writeUInt16LE(nameBytes.length, 26)
    nameBytes.copy(local, 30)

    const central = Buffer.alloc(46 + nameBytes.length)
    central.writeUInt32LE(0x02014b50, 0)
    central.writeUInt16LE(20, 4)
    central.writeUInt16LE(20, 6)
    central.writeUInt16LE(0x0800, 8)
    central.writeUInt16LE(8, 10)
    central.writeUInt32LE(crc, 16)
    central.writeUInt32LE(compressed.length, 20)
    central.writeUInt32LE(body.length, 24)
    central.writeUInt16LE(nameBytes.length, 28)
    central.writeUInt32LE(offset, 42)
    nameBytes.copy(central, 46)

    locals.push(local, compressed)
    centrals.push(central)
    offset += local.length + compressed.length
  }

  const centralBytes = Buffer.concat(centrals)
  const end = Buffer.alloc(22)
  end.writeUInt32LE(0x06054b50, 0)
  end.writeUInt16LE(centrals.length, 8)
  end.writeUInt16LE(centrals.length, 10)
  end.writeUInt32LE(centralBytes.length, 12)
  end.writeUInt32LE(offset, 16)

  return Buffer.concat([...locals, centralBytes, end])
}

/**
 * zip のエントリ名を並べて返す。
 *
 * 書き出した結果の形そのものを確かめたいので、中央ディレクトリを
 * 自前で読む。makeZip と同じ理由で依存を増やさない。
 */
export function entryNames(zip: Buffer): string[] {
  return centralEntries(zip).map((e) => e.name)
}

/** zip のエントリ 1 つの中身を文字列で返す。 */
export function readEntry(zip: Buffer, name: string): string {
  const entry = centralEntries(zip).find((e) => e.name === name)
  if (!entry) {
    throw new Error(`${name} が無い。あるのは ${entryNames(zip).join(', ')}`)
  }

  // ローカルヘッダーを読み飛ばして本体の先頭を求める。
  const nameLength = zip.readUInt16LE(entry.offset + 26)
  const extraLength = zip.readUInt16LE(entry.offset + 28)
  const start = entry.offset + 30 + nameLength + extraLength
  const body = zip.subarray(start, start + entry.compressedSize)

  // 0 は無圧縮、8 は deflate。管理コンソールは deflate で書く。
  const raw = entry.method === 0 ? body : inflateRawSync(body)
  return raw.toString('utf8')
}

type CentralEntry = {
  name: string
  offset: number
  compressedSize: number
  method: number
}

function centralEntries(zip: Buffer): CentralEntry[] {
  const end = findEndOfCentralDirectory(zip)
  const count = zip.readUInt16LE(end + 10)
  let at = zip.readUInt32LE(end + 16)

  const entries: CentralEntry[] = []
  for (let i = 0; i < count; i++) {
    if (zip.readUInt32LE(at) !== 0x02014b50) {
      throw new Error('中央ディレクトリの並びが壊れている')
    }
    const nameLength = zip.readUInt16LE(at + 28)
    const extraLength = zip.readUInt16LE(at + 30)
    const commentLength = zip.readUInt16LE(at + 32)

    entries.push({
      name: zip.subarray(at + 46, at + 46 + nameLength).toString('utf8'),
      method: zip.readUInt16LE(at + 10),
      compressedSize: zip.readUInt32LE(at + 20),
      offset: zip.readUInt32LE(at + 42),
    })
    at += 46 + nameLength + extraLength + commentLength
  }
  return entries
}

/** 末尾からレコードの印を探す。コメントが付いていても見つけられる。 */
function findEndOfCentralDirectory(zip: Buffer): number {
  for (let at = zip.length - 22; at >= 0; at--) {
    if (zip.readUInt32LE(at) === 0x06054b50) {
      return at
    }
  }
  throw new Error('zip の末尾レコードが見つからない')
}

const CRC_TABLE = buildCrcTable()

function buildCrcTable(): Uint32Array {
  const table = new Uint32Array(256)
  for (let i = 0; i < 256; i++) {
    let c = i
    for (let k = 0; k < 8; k++) {
      c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    }
    table[i] = c >>> 0
  }
  return table
}

function crc32(buf: Buffer): number {
  let c = 0xffffffff
  for (const byte of buf) {
    c = CRC_TABLE[(c ^ byte) & 0xff]! ^ (c >>> 8)
  }
  return (c ^ 0xffffffff) >>> 0
}
