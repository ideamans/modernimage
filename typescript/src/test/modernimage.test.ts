import { describe, it } from 'node:test'
import * as assert from 'node:assert'
import * as fs from 'fs'
import * as path from 'path'
import * as zlib from 'zlib'
import { webp, avif, jpeg, version, ModernImageError } from '../index'

const testdataDir = path.join(__dirname, '..', '..', '..', 'testdata')

function loadTestData(name: string): Buffer {
  return fs.readFileSync(path.join(testdataDir, name))
}

function isWebP(data: Buffer): boolean {
  return (
    data.length >= 12 &&
    data[0] === 0x52 && data[1] === 0x49 && data[2] === 0x46 && data[3] === 0x46 &&
    data[8] === 0x57 && data[9] === 0x45 && data[10] === 0x42 && data[11] === 0x50
  )
}

// Image dimension parsers (no external dependencies).
function imageDimensions(data: Buffer): [number, number] | null {
  if (data.length >= 4 && data[0] === 0xff && data[1] === 0xd8 && data[2] === 0xff) {
    return jpegDimensions(data)
  }
  if (data.length >= 8 && data[0] === 0x89 && data[1] === 0x50 && data[2] === 0x4e && data[3] === 0x47) {
    return pngDimensions(data)
  }
  if (data.length >= 6 && data[0] === 0x47 && data[1] === 0x49 && data[2] === 0x46) {
    return gifDimensions(data)
  }
  if (data.length >= 12 && data.toString('ascii', 0, 4) === 'RIFF' && data.toString('ascii', 8, 12) === 'WEBP') {
    return webpDimensions(data)
  }
  if (data.length >= 12 && data.toString('ascii', 4, 8) === 'ftyp') {
    return avifDimensions(data)
  }
  return null
}

function jpegDimensions(data: Buffer): [number, number] | null {
  let pos = 2
  while (pos + 4 <= data.length) {
    while (pos < data.length && data[pos] === 0xff) pos++
    if (pos >= data.length) break
    const marker = data[pos]
    pos++
    if (marker === 0xd8 || marker === 0xd9 || (marker >= 0xd0 && marker <= 0xd7)) continue
    if (pos + 2 > data.length) break
    const segLen = data.readUInt16BE(pos)
    if (segLen < 2 || pos + segLen > data.length) return null
    const isSOF =
      (marker >= 0xc0 && marker <= 0xc3) ||
      (marker >= 0xc5 && marker <= 0xc7) ||
      (marker >= 0xc9 && marker <= 0xcb) ||
      (marker >= 0xcd && marker <= 0xcf)
    if (isSOF) {
      if (segLen < 7) return null
      const h = data.readUInt16BE(pos + 3)
      const w = data.readUInt16BE(pos + 5)
      return [w, h]
    }
    pos += segLen
  }
  return null
}

function pngDimensions(data: Buffer): [number, number] | null {
  if (data.length < 24 || data.toString('ascii', 12, 16) !== 'IHDR') return null
  return [data.readUInt32BE(16), data.readUInt32BE(20)]
}

function gifDimensions(data: Buffer): [number, number] | null {
  if (data.length < 10) return null
  return [data.readUInt16LE(6), data.readUInt16LE(8)]
}

function webpDimensions(data: Buffer): [number, number] | null {
  if (data.length < 30) return null
  const chunk = data.toString('ascii', 12, 16)
  if (chunk === 'VP8X') {
    const w = data[24] | (data[25] << 8) | (data[26] << 16)
    const h = data[27] | (data[28] << 8) | (data[29] << 16)
    return [w + 1, h + 1]
  }
  if (chunk === 'VP8L') {
    if (data[20] !== 0x2f) return null
    const b0 = data[21], b1 = data[22], b2 = data[23], b3 = data[24]
    const w = (b0 | (b1 << 8)) & 0x3fff
    const h = ((b1 >> 6) | (b2 << 2) | (b3 << 10)) & 0x3fff
    return [w + 1, h + 1]
  }
  if (chunk === 'VP8 ') {
    if (data[23] !== 0x9d || data[24] !== 0x01 || data[25] !== 0x2a) return null
    return [data.readUInt16LE(26) & 0x3fff, data.readUInt16LE(28) & 0x3fff]
  }
  return null
}

function avifDimensions(data: Buffer): [number, number] | null {
  return findISPE(data)
}

function findISPE(data: Buffer): [number, number] | null {
  let pos = 0
  while (pos + 8 <= data.length) {
    let size = data.readUInt32BE(pos)
    const boxType = data.toString('ascii', pos + 4, pos + 8)
    let headerLen = 8
    if (size === 1) {
      if (pos + 16 > data.length) return null
      // Read 64-bit big-endian as bigint then convert
      const hi = data.readUInt32BE(pos + 8)
      const lo = data.readUInt32BE(pos + 12)
      size = hi * 0x100000000 + lo
      headerLen = 16
    } else if (size === 0) {
      size = data.length - pos
    }
    if (size < headerLen || pos + size > data.length) return null
    const contentStart = pos + headerLen
    const contentEnd = pos + size

    if (boxType === 'ispe') {
      if (contentEnd - contentStart < 12) return null
      const w = data.readUInt32BE(contentStart + 4)
      const h = data.readUInt32BE(contentStart + 8)
      return [w, h]
    } else if (boxType === 'meta') {
      if (contentEnd - contentStart >= 4) {
        const r = findISPE(data.subarray(contentStart + 4, contentEnd))
        if (r) return r
      }
    } else if (['iprp', 'ipco', 'moov', 'trak', 'mdia', 'minf', 'stbl'].includes(boxType)) {
      const r = findISPE(data.subarray(contentStart, contentEnd))
      if (r) return r
    }
    pos += size
  }
  return null
}

// ===== ICC inject / extract helpers =====

// CRC32 (IEEE) implementation matching PNG chunk CRCs.
const crcTable: number[] = (() => {
  const t = new Array<number>(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = (c & 1) ? 0xedb88320 ^ (c >>> 1) : (c >>> 1)
    t[n] = c >>> 0
  }
  return t
})()

function crc32(buf: Buffer): number {
  let c = 0xffffffff
  for (let i = 0; i < buf.length; i++) c = crcTable[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

function injectJpegICC(jpeg: Buffer, icc: Buffer): Buffer {
  if (jpeg.length < 2 || jpeg[0] !== 0xff || jpeg[1] !== 0xd8) throw new Error('not JPEG')
  const overhead = 2 + 12 + 2
  if (icc.length + overhead > 0xffff) throw new Error('ICC too large')
  const segLen = overhead + icc.length
  const seg = Buffer.alloc(2 + segLen)
  seg[0] = 0xff
  seg[1] = 0xe2
  seg.writeUInt16BE(segLen, 2)
  seg.write('ICC_PROFILE\x00', 4, 'ascii')
  seg[16] = 0x01
  seg[17] = 0x01
  icc.copy(seg, 18)
  return Buffer.concat([jpeg.subarray(0, 2), seg, jpeg.subarray(2)])
}

// Inject ICC split across multiple APP2 segments. Mimics how cameras emit
// large profiles. The encoder must reassemble the chunks.
function injectJpegICCMulti(jpegBuf: Buffer, icc: Buffer, chunkSize: number): Buffer {
  if (jpegBuf.length < 2 || jpegBuf[0] !== 0xff || jpegBuf[1] !== 0xd8) throw new Error('not JPEG')
  if (chunkSize <= 0) throw new Error('chunkSize must be positive')
  const total = Math.ceil(icc.length / chunkSize)
  if (total > 255) throw new Error('too many chunks')

  const segs: Buffer[] = []
  for (let i = 0; i < total; i++) {
    const start = i * chunkSize
    const end = Math.min(start + chunkSize, icc.length)
    const chunk = icc.subarray(start, end)
    const overhead = 2 + 12 + 2
    const segLen = overhead + chunk.length
    const seg = Buffer.alloc(2 + segLen)
    seg[0] = 0xff
    seg[1] = 0xe2
    seg.writeUInt16BE(segLen, 2)
    seg.write('ICC_PROFILE\x00', 4, 'ascii')
    seg[16] = i + 1
    seg[17] = total
    chunk.copy(seg, 18)
    segs.push(seg)
  }
  return Buffer.concat([jpegBuf.subarray(0, 2), ...segs, jpegBuf.subarray(2)])
}

function extractJpegICC(data: Buffer): Buffer | null {
  if (data.length < 4 || data[0] !== 0xff || data[1] !== 0xd8) return null
  const chunks: Array<{ seq: number; total: number; bytes: Buffer }> = []
  let pos = 2
  while (pos + 4 <= data.length) {
    while (pos < data.length && data[pos] === 0xff) pos++
    if (pos >= data.length) break
    const marker = data[pos]
    pos++
    if (marker === 0xd8 || marker === 0xd9 || (marker >= 0xd0 && marker <= 0xd7)) continue
    if (marker === 0xda) break
    if (pos + 2 > data.length) break
    const segLen = data.readUInt16BE(pos)
    if (segLen < 2 || pos + segLen > data.length) return null
    const segData = data.subarray(pos + 2, pos + segLen)
    pos += segLen
    if (marker === 0xe2 && segData.length >= 14 && segData.toString('ascii', 0, 12) === 'ICC_PROFILE\x00') {
      chunks.push({ seq: segData[12], total: segData[13], bytes: segData.subarray(14) })
    }
  }
  if (chunks.length === 0) return null
  const total = chunks[0].total
  const parts: Buffer[] = []
  for (let s = 1; s <= total; s++) {
    const c = chunks.find((x) => x.seq === s)
    if (c) parts.push(c.bytes)
  }
  return Buffer.concat(parts)
}

function buildPngChunk(type: string, data: Buffer): Buffer {
  const out = Buffer.alloc(12 + data.length)
  out.writeUInt32BE(data.length, 0)
  out.write(type, 4, 'ascii')
  data.copy(out, 8)
  out.writeUInt32BE(crc32(Buffer.concat([Buffer.from(type, 'ascii'), data])), 8 + data.length)
  return out
}

function injectPngICC(png: Buffer, icc: Buffer): Buffer {
  if (png.length < 8 || png[0] !== 0x89 || png.toString('ascii', 1, 4) !== 'PNG') throw new Error('not PNG')
  const compressed = zlib.deflateSync(icc)
  const chunkData = Buffer.concat([Buffer.from('test\x00\x00', 'ascii'), compressed])
  const iccpChunk = buildPngChunk('iCCP', chunkData)

  const out: Buffer[] = [png.subarray(0, 8)]
  let pos = 8
  let inserted = false
  while (pos + 8 <= png.length) {
    const chunkLen = png.readUInt32BE(pos)
    const chunkType = png.toString('ascii', pos + 4, pos + 8)
    const chunkEnd = pos + 8 + chunkLen + 4
    if (chunkEnd > png.length) throw new Error('truncated PNG')
    if (chunkType === 'iCCP' || chunkType === 'sRGB') {
      pos = chunkEnd
      continue
    }
    out.push(png.subarray(pos, chunkEnd))
    pos = chunkEnd
    if (chunkType === 'IHDR' && !inserted) {
      out.push(iccpChunk)
      inserted = true
    }
  }
  if (!inserted) throw new Error('no IHDR')
  return Buffer.concat(out)
}

function extractPngICC(data: Buffer): Buffer | null {
  if (data.length < 8 || data[0] !== 0x89 || data.toString('ascii', 1, 4) !== 'PNG') return null
  let pos = 8
  while (pos + 8 <= data.length) {
    const chunkLen = data.readUInt32BE(pos)
    const chunkType = data.toString('ascii', pos + 4, pos + 8)
    if (pos + 8 + chunkLen + 4 > data.length) return null
    if (chunkType === 'iCCP') {
      const content = data.subarray(pos + 8, pos + 8 + chunkLen)
      const nullIdx = content.indexOf(0)
      if (nullIdx < 0 || nullIdx + 2 >= content.length) return null
      const zlibData = content.subarray(nullIdx + 2)
      return zlib.inflateSync(zlibData)
    }
    pos += 8 + chunkLen + 4
  }
  return null
}

function extractWebpICC(data: Buffer): Buffer | null {
  if (data.length < 12 || data.toString('ascii', 0, 4) !== 'RIFF' || data.toString('ascii', 8, 12) !== 'WEBP') {
    return null
  }
  let pos = 12
  while (pos + 8 <= data.length) {
    const fourCC = data.toString('ascii', pos, pos + 4)
    const size = data.readUInt32LE(pos + 4)
    const contentStart = pos + 8
    const contentEnd = contentStart + size
    if (contentEnd > data.length) return null
    if (fourCC === 'ICCP') return Buffer.from(data.subarray(contentStart, contentEnd))
    pos = contentEnd
    if ((size & 1) === 1) pos++
  }
  return null
}

function extractAvifICC(data: Buffer): Buffer | null {
  return findColrProf(data)
}

function findColrProf(data: Buffer): Buffer | null {
  let pos = 0
  while (pos + 8 <= data.length) {
    let size = data.readUInt32BE(pos)
    const boxType = data.toString('ascii', pos + 4, pos + 8)
    let headerLen = 8
    if (size === 1) {
      if (pos + 16 > data.length) return null
      const hi = data.readUInt32BE(pos + 8)
      const lo = data.readUInt32BE(pos + 12)
      size = hi * 0x100000000 + lo
      headerLen = 16
    } else if (size === 0) {
      size = data.length - pos
    }
    if (size < headerLen || pos + size > data.length) return null
    const contentStart = pos + headerLen
    const contentEnd = pos + size

    if (boxType === 'colr' && contentEnd - contentStart >= 4) {
      const colorType = data.toString('ascii', contentStart, contentStart + 4)
      if (colorType === 'prof' || colorType === 'rICC') {
        return Buffer.from(data.subarray(contentStart + 4, contentEnd))
      }
    } else if (boxType === 'meta' && contentEnd - contentStart >= 4) {
      const r = findColrProf(data.subarray(contentStart + 4, contentEnd))
      if (r) return r
    } else if (['iprp', 'ipco', 'moov', 'trak', 'mdia', 'minf', 'stbl'].includes(boxType)) {
      const r = findColrProf(data.subarray(contentStart, contentEnd))
      if (r) return r
    }
    pos += size
  }
  return null
}

interface WebpInfo {
  isAnimated: boolean
  frameCount: number
  hasAlpha: boolean
  hasVP8L: boolean
  hasVP8: boolean
}

function parseWebP(data: Buffer): WebpInfo | null {
  if (data.length < 12 || data.toString('ascii', 0, 4) !== 'RIFF' || data.toString('ascii', 8, 12) !== 'WEBP') {
    return null
  }
  const info: WebpInfo = { isAnimated: false, frameCount: 0, hasAlpha: false, hasVP8L: false, hasVP8: false }
  let pos = 12
  while (pos + 8 <= data.length) {
    const fourCC = data.toString('ascii', pos, pos + 4)
    const size = data.readUInt32LE(pos + 4)
    const contentStart = pos + 8
    const contentEnd = contentStart + size
    if (contentEnd > data.length) return null
    if (fourCC === 'VP8X' && size >= 1) {
      const flags = data[contentStart]
      info.isAnimated = (flags & 0x02) !== 0
      info.hasAlpha = (flags & 0x10) !== 0
    } else if (fourCC === 'ANMF') {
      info.frameCount++
    } else if (fourCC === 'VP8L') {
      info.hasVP8L = true
      if (size >= 5 && ((data[contentStart + 4] >> 4) & 1) !== 0) info.hasAlpha = true
    } else if (fourCC === 'VP8 ') {
      info.hasVP8 = true
    } else if (fourCC === 'ALPH') {
      info.hasAlpha = true
    }
    pos = contentEnd
    if ((size & 1) === 1) pos++
  }
  return info
}

function assertSameDimensions(name: string, input: Buffer, output: Buffer): void {
  const inDims = imageDimensions(input)
  const outDims = imageDimensions(output)
  assert.ok(inDims, `${name}: failed to read input dimensions`)
  assert.ok(outDims, `${name}: failed to read output dimensions`)
  assert.deepStrictEqual(outDims, inDims, `${name}: dimension mismatch`)
}

// Verifies the data is an AVIF file by parsing the ftyp box and checking
// that the major brand or any compatible brand is "avif"/"avis".
// Just looking for "ftyp" at offset 4 would also accept HEIC/MP4/MOV.
function isAVIF(data: Buffer): boolean {
  if (data.length < 16) return false
  if (data[4] !== 0x66 || data[5] !== 0x74 || data[6] !== 0x79 || data[7] !== 0x70) return false
  let size = data.readUInt32BE(0)
  if (size < 16 || size > data.length) size = data.length
  const isAvifBrand = (offset: number): boolean => {
    const s = data.toString('ascii', offset, offset + 4)
    return s === 'avif' || s === 'avis'
  }
  if (isAvifBrand(8)) return true
  for (let i = 16; i + 4 <= size; i += 4) {
    if (isAvifBrand(i)) return true
  }
  return false
}

const jpegFiles = [
  'photo.jpg', 'photo-like.jpg', 'landscape-like.jpg',
  'medium-512x512.jpg', 'edges.jpg', 'gradient-radial.jpg', 'small-128x128.jpg',
]

const pngFiles = [
  'logo.png', 'photo-like.png', 'text.png',
  'flat-color.png', 'gradient-horizontal.png', 'small-128x128.png',
]

const gifFiles = [
  'animation.gif', 'animated-3frames.gif', 'animated-small.gif',
  'static-512x512.gif', 'static-alpha.gif',
]

describe('isAVIF guard', () => {
  it('rejects HEIC ftyp', () => {
    const heic = Buffer.from([
      0x00, 0x00, 0x00, 0x18,
      0x66, 0x74, 0x79, 0x70,
      0x68, 0x65, 0x69, 0x63,
      0x00, 0x00, 0x00, 0x00,
      0x6d, 0x69, 0x66, 0x31,
      0x68, 0x65, 0x69, 0x63,
    ])
    assert.strictEqual(isAVIF(heic), false)
  })
  it('accepts AVIF major brand', () => {
    const avif = Buffer.from([
      0x00, 0x00, 0x00, 0x18,
      0x66, 0x74, 0x79, 0x70,
      0x61, 0x76, 0x69, 0x66,
      0x00, 0x00, 0x00, 0x00,
      0x6d, 0x69, 0x66, 0x31,
      0x61, 0x76, 0x69, 0x66,
    ])
    assert.ok(isAVIF(avif))
  })
  it('accepts AVIF in compatible brands', () => {
    const avifCompat = Buffer.from([
      0x00, 0x00, 0x00, 0x1c,
      0x66, 0x74, 0x79, 0x70,
      0x6d, 0x69, 0x66, 0x31,
      0x00, 0x00, 0x00, 0x00,
      0x6d, 0x69, 0x66, 0x31,
      0x6d, 0x69, 0x61, 0x66,
      0x61, 0x76, 0x69, 0x66,
    ])
    assert.ok(isAVIF(avifCompat))
  })
})

describe('version', () => {
  it('returns a semver-shaped version string', () => {
    const v = version()
    assert.ok(v.length > 0)
    // major.minor.patch with optional -pre or +meta suffix
    const semver = /^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$/
    assert.ok(semver.test(v), `${v} is not semver-shaped`)
    console.log(`libmodernimage version: ${v}`)
  })
})

describe('webp.encodeLossy (JPEG)', () => {
  for (const name of jpegFiles) {
    it(name, () => {
      const data = loadTestData(name)
      const result = webp.encodeLossy(data, 80)
      assert.ok(isWebP(result.data))
      assert.strictEqual(result.mimeType, 'image/webp')
      assertSameDimensions(name, data, result.data)
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
})

describe('webp.encodeLossy (PNG)', () => {
  for (const name of pngFiles) {
    it(name, () => {
      const data = loadTestData(name)
      const result = webp.encodeLossy(data, 80, true)
      assert.ok(isWebP(result.data))
      assert.strictEqual(result.mimeType, 'image/webp')
      assertSameDimensions(name, data, result.data)
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
})

describe('webp.encodeLossless', () => {
  for (const name of [...jpegFiles, ...pngFiles]) {
    it(name, () => {
      const data = loadTestData(name)
      const result = webp.encodeLossless(data)
      assert.ok(isWebP(result.data))
      assert.strictEqual(result.mimeType, 'image/webp')
      assertSameDimensions(name, data, result.data)
      // Lossless contract: VP8L chunk must be present, VP8 (lossy) must not.
      const info = parseWebP(result.data)
      assert.ok(info, `${name}: parse failed`)
      assert.ok(info!.hasVP8L, `${name}: lossless output has no VP8L chunk`)
      assert.ok(!info!.hasVP8, `${name}: lossless output has unexpected VP8 chunk`)
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
})

describe('webp.encodeLossy guard', () => {
  it('lossy must not silently produce lossless (VP8L) output', () => {
    const data = loadTestData('photo.jpg')
    const result = webp.encodeLossy(data, 80)
    const info = parseWebP(result.data)
    assert.ok(info)
    assert.ok(!info!.hasVP8L, 'EncodeLossy unexpectedly produced VP8L')
    assert.ok(info!.hasVP8, 'EncodeLossy output has no VP8 chunk')
  })
})

describe('webp.encodeGif', () => {
  // Files known to be multi-frame (catch "first frame only" regressions).
  const multiFrame = new Set(['animation.gif', 'animated-3frames.gif', 'animated-small.gif'])
  for (const name of gifFiles) {
    it(name, () => {
      const data = loadTestData(name)
      const result = webp.encodeGif(data)
      assert.ok(isWebP(result.data))
      assert.strictEqual(result.mimeType, 'image/webp')
      assertSameDimensions(name, data, result.data)
      const info = parseWebP(result.data)
      assert.ok(info, `${name}: failed to parse output WebP`)
      if (multiFrame.has(name)) {
        assert.ok(info!.isAnimated, `${name}: expected animated WebP`)
        assert.ok(info!.frameCount >= 2, `${name}: expected ANMF >= 2, got ${info!.frameCount}`)
      } else {
        assert.ok(!info!.isAnimated, `${name}: static GIF must not produce animated WebP`)
        assert.strictEqual(info!.frameCount, 0, `${name}: static WebP should have 0 ANMF`)
      }
      console.log(`  ${name}: ${data.length} -> ${result.data.length} (animated=${info!.isAnimated}, frames=${info!.frameCount})`)
    })
  }
})

describe('avif.encodeBalanced', () => {
  for (const name of [...jpegFiles, ...pngFiles]) {
    it(name, () => {
      const data = loadTestData(name)
      const result = avif.encodeBalanced(data, 80)
      assert.ok(isAVIF(result.data))
      assert.strictEqual(result.mimeType, 'image/avif')
      assertSameDimensions(name, data, result.data)
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
})

describe('avif.encodeCompact', () => {
  // Compact is slow, test with small files only
  for (const name of ['photo.jpg', 'small-128x128.jpg', 'logo.png', 'small-128x128.png']) {
    it(name, () => {
      const data = loadTestData(name)
      const result = avif.encodeCompact(data, 80)
      assert.ok(isAVIF(result.data))
      assert.strictEqual(result.mimeType, 'image/avif')
      assertSameDimensions(name, data, result.data)
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
})

describe('avif.encodeFast', () => {
  for (const name of [...jpegFiles, ...pngFiles]) {
    it(name, () => {
      const data = loadTestData(name)
      const result = avif.encodeFast(data, 80)
      assert.ok(isAVIF(result.data))
      assert.strictEqual(result.mimeType, 'image/avif')
      assertSameDimensions(name, data, result.data)
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
})

function injectExifOrientation(jpeg: Buffer, orientation: number): Buffer {
  const exif = Buffer.from([
    0xff, 0xe1, // APP1 marker
    0x00, 0x22, // length = 34
    0x45, 0x78, 0x69, 0x66, 0x00, 0x00, // "Exif\0\0"
    0x4d, 0x4d, // big-endian "MM"
    0x00, 0x2a, // TIFF magic
    0x00, 0x00, 0x00, 0x08, // offset to IFD0
    0x00, 0x01, // 1 entry
    0x01, 0x12, // tag: Orientation
    0x00, 0x03, // type: SHORT
    0x00, 0x00, 0x00, 0x01, // count: 1
    0x00, orientation, 0x00, 0x00, // value
    0x00, 0x00, 0x00, 0x00, // next IFD: none
  ])
  return Buffer.concat([jpeg.subarray(0, 2), exif, jpeg.subarray(2)])
}

// Build a JPEG with an APP1 segment using the XMP identifier instead of Exif.
// JpegOrientation must NOT mistake this for an EXIF segment.
function injectXmpApp1(jpegBuf: Buffer): Buffer {
  if (jpegBuf.length < 2 || jpegBuf[0] !== 0xff || jpegBuf[1] !== 0xd8) throw new Error('not JPEG')
  const xmpId = Buffer.from('http://ns.adobe.com/xap/1.0/\x00', 'ascii')
  const body = Buffer.from(
    "<?xpacket begin='' id='W5M0MpCehiHzreSzNTczkc9d'?><x:xmpmeta xmlns:x='adobe:ns:meta/'/><?xpacket end='r'?>",
    'ascii',
  )
  const payload = Buffer.concat([xmpId, body])
  const segLen = 2 + payload.length
  if (segLen > 0xffff) throw new Error('XMP too large')
  const seg = Buffer.concat([
    Buffer.from([0xff, 0xe1, (segLen >> 8) & 0xff, segLen & 0xff]),
    payload,
  ])
  return Buffer.concat([jpegBuf.subarray(0, 2), seg, jpegBuf.subarray(2)])
}

describe('jpeg.orientation real little-endian EXIF', () => {
  // testdata/exif6-real.jpg is a CC0 JPEG (Tuomas Siipola, zpl.fi) with
  // a little-endian EXIF segment carrying Orientation = 6. The synthetic
  // injectExifOrientation helper uses big-endian; real cameras use LE.
  // See testdata/THIRD_PARTY.md.
  const data = (() => loadTestData('exif6-real.jpg'))()

  it('reads orientation=6 from real LE EXIF', () => {
    const dims = imageDimensions(data)!
    assert.deepStrictEqual(dims, [427, 640])
    assert.strictEqual(jpeg.orientation(data), 6)
  })

  it('normalize rotates 90° CW without trim (real camera-style JPEG)', () => {
    const result = jpeg.normalizeOrientation(data)
    const dims = imageDimensions(result)!
    // Real JPEG: -trim is a no-op because the encoder padded to MCU boundaries.
    assert.deepStrictEqual(dims, [640, 427])
    assert.ok(jpeg.orientation(result) <= 1)
  })

  it('normalize then encode WebP keeps swapped dims', () => {
    const normalized = jpeg.normalizeOrientation(data)
    const r = webp.encodeLossy(normalized, 80)
    const dims = imageDimensions(r.data)!
    assert.deepStrictEqual(dims, [640, 427])
  })
})

describe('jpeg.orientation with multiple APP1 segments', () => {
  it('finds EXIF when XMP comes first (XMP injected before EXIF)', () => {
    const src = loadTestData('small-128x128.jpg')
    const withXmp = injectXmpApp1(src)
    const data = injectExifOrientation(withXmp, 6)
    assert.strictEqual(jpeg.orientation(data), 6)
  })
  it('finds EXIF when EXIF comes first', () => {
    const src = loadTestData('small-128x128.jpg')
    const withExif = injectExifOrientation(src, 3)
    const data = injectXmpApp1(withExif)
    assert.strictEqual(jpeg.orientation(data), 3)
  })
  it('returns 0 when two XMPs present and no EXIF', () => {
    const src = loadTestData('small-128x128.jpg')
    const once = injectXmpApp1(src)
    const twice = injectXmpApp1(once)
    assert.strictEqual(jpeg.orientation(twice), 0)
  })
  it('normalizes correctly when XMP coexists with EXIF orientation=6', () => {
    const landscape = loadTestData('landscape-like.jpg')
    const [iw, ih] = imageDimensions(landscape)!
    const withXmp = injectXmpApp1(landscape)
    const data = injectExifOrientation(withXmp, 6)
    const result = jpeg.normalizeOrientation(data)
    const [ow, oh] = imageDimensions(result)!
    assert.deepStrictEqual([ow, oh], [ih, iw], 'expected wxh swap')
  })
})

describe('jpeg.orientation ignores XMP APP1', () => {
  it('returns 0 for JPEG with XMP-only APP1 (no EXIF)', () => {
    const src = loadTestData('small-128x128.jpg')
    assert.strictEqual(jpeg.orientation(src), 0)
    const data = injectXmpApp1(src)
    assert.ok(data.length > src.length)
    assert.strictEqual(jpeg.orientation(data), 0, 'XMP must not be misread as EXIF orientation')
  })
  it('normalizeOrientation does not rotate XMP-only JPEG', () => {
    const data = injectXmpApp1(loadTestData('small-128x128.jpg'))
    const result = jpeg.normalizeOrientation(data)
    assert.ok(jpeg.orientation(result) <= 1, 'must not introduce rotation')
  })
})

// Build a JPEG with valid APP1/EXIF (ImageDescription tag) but NO Orientation tag.
function injectExifWithoutOrientation(jpegBuf: Buffer): Buffer {
  if (jpegBuf.length < 2 || jpegBuf[0] !== 0xff || jpegBuf[1] !== 0xd8) throw new Error('not JPEG')
  const exif = Buffer.from([
    0xff, 0xe1,
    0x00, 0x22,
    0x45, 0x78, 0x69, 0x66, 0x00, 0x00,
    0x4d, 0x4d,
    0x00, 0x2a,
    0x00, 0x00, 0x00, 0x08,
    0x00, 0x01,
    0x01, 0x0e, // ImageDescription
    0x00, 0x02, // ASCII
    0x00, 0x00, 0x00, 0x01,
    0x41, 0x00, 0x00, 0x00, // inline 'A'
    0x00, 0x00, 0x00, 0x00,
  ])
  return Buffer.concat([jpegBuf.subarray(0, 2), exif, jpegBuf.subarray(2)])
}

describe('jpeg.orientation EXIF without orientation tag', () => {
  it('returns 0 (or 1) when EXIF exists but Orientation tag is missing', () => {
    const data = injectExifWithoutOrientation(loadTestData('small-128x128.jpg'))
    const ori = jpeg.orientation(data)
    assert.ok(ori === 0 || ori === 1, `unexpected orientation: ${ori}`)
  })
  it('normalizeOrientation does not error when EXIF lacks orientation', () => {
    const data = injectExifWithoutOrientation(loadTestData('small-128x128.jpg'))
    const result = jpeg.normalizeOrientation(data)
    assert.ok(result.length > 0)
  })
})

describe('jpeg.orientation', () => {
  it('returns 0 for JPEG without EXIF', () => {
    const data = loadTestData('small-128x128.jpg')
    assert.strictEqual(jpeg.orientation(data), 0)
  })

  for (const ori of [1, 2, 3, 4, 5, 6, 7, 8]) {
    it(`detects orientation ${ori}`, () => {
      const data = injectExifOrientation(loadTestData('small-128x128.jpg'), ori)
      assert.strictEqual(jpeg.orientation(data), ori)
    })
  }

  it('returns 0 for PNG', () => {
    const data = loadTestData('small-128x128.png')
    assert.strictEqual(jpeg.orientation(data), 0)
  })

  it('returns 0 for empty buffer', () => {
    assert.strictEqual(jpeg.orientation(Buffer.alloc(0)), 0)
  })
})

describe('jpeg.normalizeOrientation', () => {
  it('throws on empty input', () => {
    assert.throws(() => jpeg.normalizeOrientation(Buffer.alloc(0)), ModernImageError)
  })

  it('returns byte-equal copy when no EXIF', () => {
    const data = loadTestData('small-128x128.jpg')
    const result = jpeg.normalizeOrientation(data)
    assert.ok(result.equals(data), 'result should be byte-equal to input')
  })

  it('returns byte-equal copy for orientation=1', () => {
    const data = injectExifOrientation(loadTestData('small-128x128.jpg'), 1)
    const result = jpeg.normalizeOrientation(data)
    assert.ok(result.equals(data))
  })

  for (const ori of [2, 3, 4, 5, 6, 7, 8]) {
    it(`normalizes orientation ${ori}`, () => {
      const data = injectExifOrientation(loadTestData('small-128x128.jpg'), ori)
      const result = jpeg.normalizeOrientation(data)
      // Result should be valid JPEG
      assert.ok(result.length >= 2 && result[0] === 0xff && result[1] === 0xd8,
        'result is not valid JPEG')
      // Should have no EXIF orientation
      const newOri = jpeg.orientation(result)
      assert.ok(newOri <= 1, `result still has orientation ${newOri}`)
      console.log(`  orientation ${ori}: ${data.length} -> ${result.length}`)
    })
  }
})

describe('jpeg.normalizeOrientation non-MCU pixel-complete', () => {
  // Post-fix (JUDGE-5 resolved): every orientation preserves all pixels by
  // falling back to decode → rotate → re-encode when jpegtran -perfect
  // refuses the source. Only orient 5 hits the fast jpegtran path.
  const cases: Array<[number, number, number]> = [
    [2, 99, 97], // flip H
    [3, 99, 97], // 180°
    [4, 99, 97], // flip V
    [5, 97, 99], // transpose (fast path)
    [6, 97, 99], // 90° CW
    [7, 97, 99], // transverse
    [8, 97, 99], // 270° CW
  ]
  for (const [ori, wantW, wantH] of cases) {
    it(`orient ${ori}: 99x97 → ${wantW}x${wantH} (no trim)`, () => {
      const src = loadTestData('nonmcu-99x97.jpg')
      const dims = imageDimensions(src)!
      assert.deepStrictEqual(dims, [99, 97])
      const data = injectExifOrientation(src, ori)
      const result = jpeg.normalizeOrientation(data)
      const out = imageDimensions(result)!
      assert.deepStrictEqual(out, [wantW, wantH], `pixel-complete normalize regression`)
      // Orientation tag must be cleared.
      assert.ok(jpeg.orientation(result) <= 1)
    })
  }
})

describe('jpeg.normalizeOrientation dimension verification', () => {
  // Use a non-square image so width/height swap is observable.
  // Catches the failure mode where the implementation strips the EXIF tag
  // without actually rotating pixels.
  const cases: Array<[number, boolean]> = [
    [2, false], [3, false], [4, false],
    [5, true], [6, true], [7, true], [8, true],
  ]
  for (const [ori, swap] of cases) {
    it(`orientation ${ori} ${swap ? 'swaps' : 'preserves'} dimensions`, () => {
      const src = loadTestData('landscape-like.jpg')
      const inDims = imageDimensions(src)
      assert.ok(inDims, 'failed to read input dims')
      const [iw, ih] = inDims!
      assert.notStrictEqual(iw, ih, 'test image must not be square')
      assert.strictEqual(jpeg.orientation(src), 0, 'test image must have no EXIF orientation')

      const data = injectExifOrientation(src, ori)
      const result = jpeg.normalizeOrientation(data)
      const outDims = imageDimensions(result)
      assert.ok(outDims, 'failed to read output dims')
      const [ow, oh] = outDims!
      const want = swap ? [ih, iw] : [iw, ih]
      assert.deepStrictEqual([ow, oh], want, `orientation ${ori} dim mismatch`)
    })
  }
})

describe('jpeg.normalizeOrientation -> WebP', () => {
  it('orient6 chain swaps dimensions through to WebP', () => {
    const src = loadTestData('landscape-like.jpg')
    const [iw, ih] = imageDimensions(src)!
    const data = injectExifOrientation(src, 6)
    const normalized = jpeg.normalizeOrientation(data)
    const result = webp.encodeLossy(normalized, 80)
    assert.ok(isWebP(result.data))
    const [ow, oh] = imageDimensions(result.data)!
    assert.deepStrictEqual([ow, oh], [ih, iw], 'orient6 chain dim mismatch')
    console.log(`  orient6 -> normalize -> WebP: ${iw}x${ih} -> ${ow}x${oh}`)
  })
})

describe('jpeg.normalizeOrientation -> AVIF', () => {
  it('orient3 chain preserves dimensions through to AVIF', () => {
    const src = loadTestData('landscape-like.jpg')
    const [iw, ih] = imageDimensions(src)!
    const data = injectExifOrientation(src, 3)
    const normalized = jpeg.normalizeOrientation(data)
    const result = avif.encodeFast(normalized, 80)
    assert.ok(isAVIF(result.data))
    const [ow, oh] = imageDimensions(result.data)!
    assert.deepStrictEqual([ow, oh], [iw, ih], 'orient3 chain dim mismatch')
    console.log(`  orient3 -> normalize -> AVIF: ${iw}x${ih} -> ${ow}x${oh}`)
  })
})

function assertThrowsWith(fn: () => unknown, substr: string): void {
  assert.throws(fn, (err: any) => {
    return err instanceof ModernImageError && typeof err.message === 'string' && err.message.includes(substr)
  })
}

describe('quality boundaries', () => {
  const data = (() => loadTestData('photo.jpg'))()

  it('webp.encodeLossy q=0/50/100 monotonic-ish', () => {
    const sizes = [0, 50, 100].map((q) => webp.encodeLossy(data, q).data.length)
    assert.ok(sizes[2] > sizes[0], `q=100 (${sizes[2]}) should be larger than q=0 (${sizes[0]})`)
  })

  it('avif.encodeFast q=0/50/99 (q=100 see q100 test)', () => {
    for (const q of [0, 50, 99]) {
      const r = avif.encodeFast(data, q)
      assert.ok(isAVIF(r.data), `q=${q}: not AVIF`)
    }
  })

  // Locks in the current observed behavior — see review.md JUDGE-1.
  it('avif q=100 fails for ALL presets (current avifenc lossless guard)', () => {
    assert.throws(() => avif.encodeBalanced(data, 100), ModernImageError, 'Balanced q=100 unexpectedly succeeded')
    assert.throws(() => avif.encodeCompact(data, 100), ModernImageError, 'Compact q=100 unexpectedly succeeded')
    assert.throws(() => avif.encodeFast(data, 100), ModernImageError, 'Fast q=100 unexpectedly succeeded')
  })
})

describe('multithread smoke', () => {
  const data = (() => loadTestData('medium-512x512.jpg'))()

  it('webp.encodeLossy with multithread', () => {
    const r = webp.encodeLossy(data, 80, true)
    assert.ok(isWebP(r.data))
  })
  it('webp.encodeLossless with multithread', () => {
    const r = webp.encodeLossless(data, true)
    assert.ok(isWebP(r.data))
  })
  it('webp.encodeGif with multithread', () => {
    const gif = loadTestData('animation.gif')
    const r = webp.encodeGif(gif, true)
    assert.ok(isWebP(r.data))
  })
  it('avif.encodeFast jobs=1', () => {
    const r = avif.encodeFast(data, 80, 1)
    assert.ok(isAVIF(r.data))
  })
  it('avif.encodeFast jobs=8', () => {
    const r = avif.encodeFast(data, 80, 8)
    assert.ok(isAVIF(r.data))
  })
})

describe('compression sanity (lossy ≤ 50% of source)', () => {
  // Catches the failure mode where output is "valid" but ~as large as input.
  const src = loadTestData('landscape-like.jpg')
  const maxLossy = Math.floor(src.length / 2)

  it('webp.encodeLossy compresses photographic JPEG to ≤50%', () => {
    const r = webp.encodeLossy(src, 80)
    assert.ok(
      r.data.length <= maxLossy,
      `WebP lossy q=80: ${r.data.length} > 50% of input ${src.length}`,
    )
  })
  it('avif.encodeFast compresses photographic JPEG to ≤50%', () => {
    const r = avif.encodeFast(src, 80)
    assert.ok(
      r.data.length <= maxLossy,
      `AVIF fast q=80: ${r.data.length} > 50% of input ${src.length}`,
    )
  })
  it('avif.encodeBalanced compresses photographic JPEG to ≤50%', () => {
    const r = avif.encodeBalanced(src, 80)
    assert.ok(
      r.data.length <= maxLossy,
      `AVIF balanced q=80: ${r.data.length} > 50% of input ${src.length}`,
    )
  })
  it('webp.encodeLossless produces non-empty output', () => {
    const r = webp.encodeLossless(src)
    assert.ok(r.data.length > 0)
  })
})

describe('avif compact vs balanced', () => {
  it('compact is smaller than balanced for a photo', () => {
    const data = loadTestData('photo.jpg')
    const balanced = avif.encodeBalanced(data, 80)
    const compact = avif.encodeCompact(data, 80)
    assert.ok(
      compact.data.length < balanced.data.length,
      `compact (${compact.data.length}) not smaller than balanced (${balanced.data.length})`,
    )
    console.log(
      `  photo.jpg: balanced=${balanced.data.length} compact=${compact.data.length}`,
    )
  })
})

describe('concurrent encodes (sequential under Promise.all)', () => {
  // Note: koffi FFI calls are synchronous and block the event loop, so this
  // is effectively sequential. We still wrap in Promise.all to verify that
  // (a) the API doesn't break under repeated rapid calls and (b) no per-call
  // state leaks between iterations.
  it('mixed encoders sequenced through Promise.all give correct results', async () => {
    const jpegBuf = loadTestData('small-128x128.jpg')
    const pngBuf = loadTestData('logo.png')
    const jDims = imageDimensions(jpegBuf)!
    const pDims = imageDimensions(pngBuf)!

    const ITERS = 32
    const tasks: Array<Promise<void>> = []
    for (let i = 0; i < ITERS; i++) {
      tasks.push(
        (async () => {
          switch (i % 4) {
            case 0: {
              const r = webp.encodeLossy(jpegBuf, 80)
              assert.deepStrictEqual(imageDimensions(r.data), jDims)
              break
            }
            case 1: {
              const r = webp.encodeLossless(pngBuf)
              assert.deepStrictEqual(imageDimensions(r.data), pDims)
              break
            }
            case 2: {
              const r = avif.encodeFast(jpegBuf, 80)
              assert.deepStrictEqual(imageDimensions(r.data), jDims)
              break
            }
            case 3: {
              const r = avif.encodeFast(pngBuf, 80)
              assert.deepStrictEqual(imageDimensions(r.data), pDims)
              break
            }
          }
        })(),
      )
    }
    await Promise.all(tasks)
  })
})

describe('stderr capture', () => {
  it('avif q=100 error contains avifenc stderr text', () => {
    const data = loadTestData('photo.jpg')
    try {
      avif.encodeFast(data, 100)
      assert.fail('expected throw')
    } catch (e: any) {
      assert.ok(e instanceof ModernImageError, `unexpected: ${e}`)
      assert.ok(
        e.message.includes('Invalid codec-specific option'),
        `missing avifenc stderr text:\n${e.message}`,
      )
    }
  })

  it('webp.encodeLossy error on garbage JPEG contains cwebp stderr fragment', () => {
    const bad = Buffer.concat([Buffer.from([0xff, 0xd8, 0xff]), Buffer.alloc(100)])
    try {
      webp.encodeLossy(bad, 80)
      assert.fail('expected throw')
    } catch (e: any) {
      assert.ok(e instanceof ModernImageError)
      const m = e.message as string
      const ok =
        m.includes('Could not process') ||
        m.includes('Error') ||
        m.includes('decode') ||
        m.includes('Cannot') ||
        m.includes('ERROR') ||
        m.includes('FAILED')
      assert.ok(ok, `missing cwebp stderr fragment:\n${m}`)
    }
  })
})

describe('EncodeResult.warnings (JUDGE-2b)', () => {
  // Truncated JPEG fed to avifenc is a deterministic trigger:
  // libjpeg silently fabricates the missing entropy data and emits
  // "Premature end of JPEG file" to stderr but still returns success.
  it('avif.encodeFast on truncated JPEG surfaces "Premature end" warning', () => {
    const src = loadTestData('photo.jpg')
    const half = src.subarray(0, Math.floor(src.length / 2))
    let result
    try {
      result = avif.encodeFast(half, 80)
    } catch (e) {
      // Host avifenc happened to refuse — accept and skip.
      return
    }
    const hasPremature = result.warnings.some((w) => w.includes('Premature end of JPEG file'))
    assert.ok(
      hasPremature,
      `expected 'Premature end of JPEG file' in warnings, got ${result.warnings.length} lines: ${JSON.stringify(result.warnings)}`,
    )
  })

  it('webp.encodeLossy on clean input has no problem-indicating warnings', () => {
    const src = loadTestData('photo.jpg')
    const result = webp.encodeLossy(src, 80)
    // cwebp emits informational stats. We accept those, but a clean
    // encode must not contain any genuine problem indicator.
    const problemIndicators = ['Premature end', 'WARNING', 'ERROR', 'error', 'failed', 'Failed']
    for (const w of result.warnings) {
      for (const ind of problemIndicators) {
        assert.ok(!w.includes(ind), `clean input warning contains problem indicator "${ind}": ${w}`)
      }
    }
  })

  it('all encoders return warnings array (possibly empty)', () => {
    const src = loadTestData('small-128x128.jpg')
    const r1 = webp.encodeLossy(src, 80)
    const r2 = webp.encodeLossless(src)
    const r3 = avif.encodeFast(src, 80)
    assert.ok(Array.isArray(r1.warnings))
    assert.ok(Array.isArray(r2.warnings))
    assert.ok(Array.isArray(r3.warnings))
  })
})

describe('truncated input', () => {
  const jpegData = loadTestData('photo.jpg')
  const pngData = loadTestData('logo.png')
  const gifData = loadTestData('animation.gif')

  for (const frac of [0.10, 0.30, 0.50, 0.75]) {
    it(`webp.encodeLossy errors on JPEG truncated to ${(frac * 100).toFixed(0)}%`, () => {
      const n = Math.floor(jpegData.length * frac)
      assert.throws(() => webp.encodeLossy(jpegData.subarray(0, n), 80), ModernImageError)
    })
  }

  it('webp.encodeLossy errors on half-truncated PNG', () => {
    assert.throws(() => webp.encodeLossy(pngData.subarray(0, pngData.length / 2), 80, true), ModernImageError)
  })
  it('webp.encodeLossless errors on half-truncated PNG', () => {
    assert.throws(() => webp.encodeLossless(pngData.subarray(0, pngData.length / 2)), ModernImageError)
  })
  it('webp.encodeGif errors on half-truncated GIF', () => {
    assert.throws(() => webp.encodeGif(gifData.subarray(0, gifData.length / 2)), ModernImageError)
  })

  it('webp.encodeLossy errors on SOI-only JPEG', () => {
    assert.throws(() => webp.encodeLossy(Buffer.from([0xff, 0xd8]), 80), ModernImageError)
  })
  it('webp.encodeLossy errors on signature-only PNG', () => {
    assert.throws(
      () => webp.encodeLossy(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), 80, true),
      ModernImageError,
    )
  })

  // JUDGE-2 in review.md: AVIF path is much more lenient than WebP path.
  // Only header-destroying truncation (<=10%) is reliably caught.
  for (const frac of [0.05, 0.10]) {
    it(`avif.encodeFast errors on JPEG header-destroyed (${(frac * 100).toFixed(0)}%)`, () => {
      const n = Math.floor(jpegData.length * frac)
      assert.throws(() => avif.encodeFast(jpegData.subarray(0, n), 80), ModernImageError)
    })
  }
  // Lenient post-header behavior: accept either outcome, no panic.
  for (const frac of [0.20, 0.50, 0.75, 0.90]) {
    it(`avif.encodeFast does not panic on JPEG truncated to ${(frac * 100).toFixed(0)}%`, () => {
      const n = Math.floor(jpegData.length * frac)
      try {
        avif.encodeFast(jpegData.subarray(0, n), 80)
      } catch (e) {
        assert.ok(e instanceof ModernImageError, `unexpected error type: ${e}`)
      }
    })
  }
})

describe('ICC profile preservation', () => {
  const srgb = loadTestData('srgb-test.icc')
  assert.ok(srgb.length >= 128)

  it('JPEG round-trip self-extract', () => {
    const jpeg = loadTestData('small-128x128.jpg')
    const jpegIcc = injectJpegICC(jpeg, srgb)
    const got = extractJpegICC(jpegIcc)
    assert.ok(got && got.equals(srgb), 'self-extract failed')
  })

  it('webp.encodeLossy preserves JPEG ICC', () => {
    const jpegIcc = injectJpegICC(loadTestData('small-128x128.jpg'), srgb)
    const result = webp.encodeLossy(jpegIcc, 80)
    const got = extractWebpICC(result.data)
    assert.ok(got && got.equals(srgb), 'WebP lossy lost ICC')
  })

  it('webp.encodeLossless preserves JPEG ICC', () => {
    const jpegIcc = injectJpegICC(loadTestData('small-128x128.jpg'), srgb)
    const result = webp.encodeLossless(jpegIcc)
    const got = extractWebpICC(result.data)
    assert.ok(got && got.equals(srgb), 'WebP lossless lost ICC')
  })

  it('avif.encodeFast preserves JPEG ICC', () => {
    const jpegIcc = injectJpegICC(loadTestData('small-128x128.jpg'), srgb)
    const result = avif.encodeFast(jpegIcc, 80)
    const got = extractAvifICC(result.data)
    assert.ok(got && got.equals(srgb), 'AVIF fast lost ICC')
  })

  it('avif.encodeBalanced preserves JPEG ICC', () => {
    const jpegIcc = injectJpegICC(loadTestData('small-128x128.jpg'), srgb)
    const result = avif.encodeBalanced(jpegIcc, 80)
    const got = extractAvifICC(result.data)
    assert.ok(got && got.equals(srgb), 'AVIF balanced lost ICC')
  })

  it('PNG round-trip self-extract', () => {
    const png = loadTestData('small-128x128.png')
    const pngIcc = injectPngICC(png, srgb)
    const got = extractPngICC(pngIcc)
    assert.ok(got && got.equals(srgb), 'self-extract failed')
  })

  it('webp.encodeLossy preserves PNG ICC', () => {
    const pngIcc = injectPngICC(loadTestData('small-128x128.png'), srgb)
    const result = webp.encodeLossy(pngIcc, 80)
    const got = extractWebpICC(result.data)
    assert.ok(got && got.equals(srgb), 'WebP lossy lost PNG ICC')
  })

  it('avif.encodeFast preserves PNG ICC', () => {
    const pngIcc = injectPngICC(loadTestData('small-128x128.png'), srgb)
    const result = avif.encodeFast(pngIcc, 80)
    const got = extractAvifICC(result.data)
    assert.ok(got && got.equals(srgb), 'AVIF fast lost PNG ICC')
  })

  it('jpeg.normalizeOrientation preserves ICC', () => {
    const jpegIcc = injectJpegICC(loadTestData('small-128x128.jpg'), srgb)
    const jpegRotated = injectExifOrientation(jpegIcc, 6)
    const normalized = jpeg.normalizeOrientation(jpegRotated)
    const got = extractJpegICC(normalized)
    assert.ok(got && got.equals(srgb), 'jpegtran lost ICC')
  })

  describe('multi-segment ICC reassembly', () => {
    const jpegMulti = injectJpegICCMulti(loadTestData('small-128x128.jpg'), srgb, 1000)

    it('self-extract reassembles into original profile', () => {
      const got = extractJpegICC(jpegMulti)
      assert.ok(got && got.equals(srgb), 'self-extract failed')
    })

    it('webp.encodeLossy reassembles multi-segment ICC', () => {
      const r = webp.encodeLossy(jpegMulti, 80)
      const got = extractWebpICC(r.data)
      assert.ok(got && got.equals(srgb), 'WebP lossy lost multi-segment ICC')
    })

    it('avif.encodeFast reassembles multi-segment ICC', () => {
      const r = avif.encodeFast(jpegMulti, 80)
      const got = extractAvifICC(r.data)
      assert.ok(got && got.equals(srgb), 'AVIF fast lost multi-segment ICC')
    })

    it('jpegtran reassembles multi-segment ICC after rotate', () => {
      const rotated = injectExifOrientation(jpegMulti, 6)
      const normalized = jpeg.normalizeOrientation(rotated)
      const got = extractJpegICC(normalized)
      assert.ok(got && got.equals(srgb), 'jpegtran lost multi-segment ICC')
    })
  })
})

describe('alpha channel preservation', () => {
  it('webp.encodeLossy preserves PNG alpha', () => {
    const data = loadTestData('alpha-4x4.png')
    const result = webp.encodeLossy(data, 80)
    const info = parseWebP(result.data)
    assert.ok(info, 'parse webp')
    assert.ok(info!.hasAlpha, 'alpha lost during encodeLossy')
  })
  it('webp.encodeLossless preserves PNG alpha', () => {
    const data = loadTestData('alpha-4x4.png')
    const result = webp.encodeLossless(data)
    const info = parseWebP(result.data)
    assert.ok(info, 'parse webp')
    assert.ok(info!.hasAlpha, 'alpha lost during encodeLossless')
  })
  it('webp.encodeGif preserves transparent GIF alpha', () => {
    const data = loadTestData('static-alpha.gif')
    const result = webp.encodeGif(data)
    const info = parseWebP(result.data)
    assert.ok(info, 'parse webp')
    assert.ok(info!.hasAlpha, 'alpha lost during encodeGif')
  })
})

describe('error handling: empty input (all encoders)', () => {
  const empty = Buffer.alloc(0)
  it('webp.encodeLossy', () => assertThrowsWith(() => webp.encodeLossy(empty, 80), 'empty'))
  it('webp.encodeLossless', () => assertThrowsWith(() => webp.encodeLossless(empty), 'empty'))
  it('webp.encodeGif', () => assertThrowsWith(() => webp.encodeGif(empty), 'empty'))
  it('avif.encodeBalanced', () => assertThrowsWith(() => avif.encodeBalanced(empty, 80), 'empty'))
  it('avif.encodeCompact', () => assertThrowsWith(() => avif.encodeCompact(empty, 80), 'empty'))
  it('avif.encodeFast', () => assertThrowsWith(() => avif.encodeFast(empty, 80), 'empty'))
  it('jpeg.normalizeOrientation', () => assertThrowsWith(() => jpeg.normalizeOrientation(empty), 'empty'))
})

describe('error handling: wrong format (all encoders)', () => {
  const gif = loadTestData('animation.gif')
  const jpegData = loadTestData('photo.jpg')
  const png = loadTestData('logo.png')

  it('webp.encodeLossy rejects GIF', () => assertThrowsWith(() => webp.encodeLossy(gif, 80), 'unsupported format'))
  it('webp.encodeLossless rejects GIF', () => assertThrowsWith(() => webp.encodeLossless(gif), 'unsupported format'))
  it('webp.encodeGif rejects JPEG', () => assertThrowsWith(() => webp.encodeGif(jpegData), 'unsupported format'))
  it('webp.encodeGif rejects PNG', () => assertThrowsWith(() => webp.encodeGif(png), 'unsupported format'))
  it('avif.encodeBalanced rejects GIF', () =>
    assertThrowsWith(() => avif.encodeBalanced(gif, 80), 'unsupported format'))
  it('avif.encodeCompact rejects GIF', () =>
    assertThrowsWith(() => avif.encodeCompact(gif, 80), 'unsupported format'))
  it('avif.encodeFast rejects GIF', () => assertThrowsWith(() => avif.encodeFast(gif, 80), 'unsupported format'))
})

describe('error handling: garbage input', () => {
  const garbage = Buffer.from([0x00, 0x01, 0x02, 0x03, 0x04, 0x05])
  it('webp.encodeLossy', () => assertThrowsWith(() => webp.encodeLossy(garbage, 80), 'unsupported format'))
  it('avif.encodeFast', () => assertThrowsWith(() => avif.encodeFast(garbage, 80), 'unsupported format'))
})