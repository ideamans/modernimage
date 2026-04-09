import * as fs from 'fs'
import * as os from 'os'
import * as path from 'path'
import {
  modernimage_context_new,
  modernimage_context_free,
  modernimage_set_stdin,
  modernimage_cwebp,
  modernimage_gif2webp,
  modernimage_avifenc,
  modernimage_jpegtran,
  modernimage_jpeg_orientation,
  modernimage_get_exit_code,
  modernimage_get_stderr_size,
  modernimage_copy_stderr,
  modernimage_version,
} from './ffi'
import { ModernImageError, EncodeResult } from './types'
import { cpus } from 'os'

function detectFormat(data: Buffer): string {
  if (data.length < 4) return ''
  if (data[0] === 0xff && data[1] === 0xd8 && data[2] === 0xff) return 'jpeg'
  if (data[0] === 0x89 && data[1] === 0x50 && data[2] === 0x4e && data[3] === 0x47) return 'png'
  if (data[0] === 0x47 && data[1] === 0x49 && data[2] === 0x46) return 'gif'
  return ''
}

let tmpCounter = 0
function createTempPath(suffix: string): string {
  tmpCounter++
  return path.join(os.tmpdir(), `modernimage-${process.pid}-${Date.now()}-${tmpCounter}${suffix}`)
}

type ToolFunc = (ctx: any, argc: number, argv: string[]) => number

interface CallToolResult {
  data: Buffer
  warnings: string[]
}

function callTool(
  toolFunc: ToolFunc,
  inputData: Buffer,
  argv: string[],
  useStdin: boolean,
): CallToolResult {
  const ctx = modernimage_context_new()
  if (!ctx) {
    throw new ModernImageError('failed to create context')
  }

  try {
    if (useStdin) {
      modernimage_set_stdin(ctx, inputData, inputData.length)
    }

    const rc = toolFunc(ctx, argv.length, argv)

    if (rc !== 0) {
      const stderrSize = modernimage_get_stderr_size(ctx)
      let errMsg = `tool exited with code ${rc}`
      if (stderrSize > 0) {
        const errBuf = Buffer.alloc(Number(stderrSize))
        modernimage_copy_stderr(ctx, errBuf, errBuf.length)
        errMsg += `: ${errBuf.toString('utf8')}`
      }
      throw new ModernImageError(errMsg)
    }

    // Find output path from argv (-o or -outfile <path>)
    let outPath = ''
    for (let i = 0; i < argv.length; i++) {
      if ((argv[i] === '-o' || argv[i] === '-outfile') && i + 1 < argv.length) {
        outPath = argv[i + 1]
        break
      }
    }

    if (!outPath) {
      throw new ModernImageError('no output path in argv')
    }

    const outData = fs.readFileSync(outPath)
    if (outData.length === 0) {
      throw new ModernImageError('encoding produced empty output')
    }

    // Capture non-fatal stderr from the success path. See
    // EncodeResult.warnings doc for what these contain.
    const warnings = captureStderrWarnings(ctx)

    return { data: outData, warnings }
  } finally {
    modernimage_context_free(ctx)
  }
}

function captureStderrWarnings(ctx: any): string[] {
  const stderrSize = modernimage_get_stderr_size(ctx)
  if (stderrSize === 0) return []
  const buf = Buffer.alloc(Number(stderrSize))
  modernimage_copy_stderr(ctx, buf, buf.length)
  return buf
    .toString('utf8')
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
}

/**
 * Encode JPEG/PNG to lossy WebP.
 * ICC profiles are preserved from the source image.
 */
export function encodeLossy(data: Buffer, quality: number = 80, multithread: boolean = false): EncodeResult {
  if (!data || data.length === 0) throw new ModernImageError('empty input data')
  const format = detectFormat(data)
  if (format !== 'jpeg' && format !== 'png') {
    throw new ModernImageError(`unsupported format for encodeLossy (expected JPEG or PNG, got "${format}")`)
  }

  const tmpOut = createTempPath('.webp')
  try {
    const argv = ['cwebp', '-q', String(quality), '-metadata', 'icc']
    if (multithread) argv.push('-mt')
    argv.push('-o', tmpOut, '--', '-')

    const r = callTool(modernimage_cwebp, data, argv, true)
    return { data: r.data, mimeType: 'image/webp', warnings: r.warnings }
  } finally {
    try { fs.unlinkSync(tmpOut) } catch {}
  }
}

/**
 * Encode JPEG/PNG to lossless WebP.
 * ICC profiles are preserved from the source image.
 */
export function encodeLossless(data: Buffer, multithread: boolean = false): EncodeResult {
  if (!data || data.length === 0) throw new ModernImageError('empty input data')
  const format = detectFormat(data)
  if (format !== 'jpeg' && format !== 'png') {
    throw new ModernImageError(`unsupported format for encodeLossless (expected JPEG or PNG, got "${format}")`)
  }

  const tmpOut = createTempPath('.webp')
  try {
    const argv = ['cwebp', '-lossless', '-metadata', 'icc']
    if (multithread) argv.push('-mt')
    argv.push('-o', tmpOut, '--', '-')

    const r = callTool(modernimage_cwebp, data, argv, true)
    return { data: r.data, mimeType: 'image/webp', warnings: r.warnings }
  } finally {
    try { fs.unlinkSync(tmpOut) } catch {}
  }
}

/**
 * Encode GIF to animated WebP.
 */
export function encodeGif(data: Buffer, multithread: boolean = false): EncodeResult {
  if (!data || data.length === 0) throw new ModernImageError('empty input data')
  const format = detectFormat(data)
  if (format !== 'gif') {
    throw new ModernImageError(`unsupported format for encodeGif (expected GIF, got "${format}")`)
  }

  const tmpIn = createTempPath('.gif')
  const tmpOut = createTempPath('.webp')
  try {
    fs.writeFileSync(tmpIn, data)

    const argv = ['gif2webp']
    if (multithread) argv.push('-mt')
    argv.push(tmpIn, '-o', tmpOut)

    const r = callTool(modernimage_gif2webp, data, argv, false)
    return { data: r.data, mimeType: 'image/webp', warnings: r.warnings }
  } finally {
    try { fs.unlinkSync(tmpIn) } catch {}
    try { fs.unlinkSync(tmpOut) } catch {}
  }
}

// AVIF preset parameters matching libnextimage-lite's avifenc_bridge.
interface AvifPreset {
  speed: number
  defaultJobs: number  // 0 = CPU count
  yuv: string          // "444" or "420"
  cicp: string         // "CP/TC/MC"
  depth?: number       // bit depth (omit = default 8)
  autoTile: boolean
  tileRows?: number    // tilerowslog2
  tileCols?: number    // tilecolslog2
}

const presetBalanced: AvifPreset = {
  speed: 6, defaultJobs: 16, yuv: '444', cicp: '1/1/1', autoTile: true,
}
const presetCompact: AvifPreset = {
  speed: 0, defaultJobs: 0, yuv: '444', cicp: '1/1/1', depth: 10, autoTile: true,
}
const presetFast: AvifPreset = {
  speed: 9, defaultJobs: 16, yuv: '420', cicp: '1/13/6',
  autoTile: false, tileRows: 6, tileCols: 6,
}

function encodeAvif(data: Buffer, quality: number, jobs: number, preset: AvifPreset, opName: string): EncodeResult {
  if (!data || data.length === 0) throw new ModernImageError('empty input data')
  const format = detectFormat(data)
  if (format !== 'jpeg' && format !== 'png') {
    throw new ModernImageError(`unsupported format for ${opName} (expected JPEG or PNG, got "${format}")`)
  }

  const tmpOut = createTempPath('.avif')
  try {
    const argv = [
      'avifenc',
      '-q', String(quality),
      '-s', String(preset.speed),
      '--yuv', preset.yuv,
      '--cicp', preset.cicp,
    ]

    if (preset.depth) argv.push('-d', String(preset.depth))

    let effectiveJobs = preset.defaultJobs
    if (jobs > 0) effectiveJobs = jobs
    else if (effectiveJobs === 0) effectiveJobs = cpus().length
    argv.push('-j', String(effectiveJobs))

    if (preset.autoTile) argv.push('--autotiling')
    if (preset.tileRows) argv.push('--tilerowslog2', String(preset.tileRows))
    if (preset.tileCols) argv.push('--tilecolslog2', String(preset.tileCols))

    argv.push('-a', 'tune=ssimulacra2')
    argv.push('--input-format', format, '-o', tmpOut, '--stdin')

    const r = callTool(modernimage_avifenc, data, argv, true)
    return { data: r.data, mimeType: 'image/avif', warnings: r.warnings }
  } finally {
    try { fs.unlinkSync(tmpOut) } catch {}
  }
}

/**
 * Encode JPEG/PNG to AVIF with balanced speed/quality.
 * YUV444, CICP 1/1/1 (BT.709), speed 6, autotiling, tune=ssimulacra2.
 */
export function encodeBalanced(data: Buffer, quality: number = 80, jobs: number = 0): EncodeResult {
  return encodeAvif(data, quality, jobs, presetBalanced, 'encodeBalanced')
}

/**
 * Encode JPEG/PNG to AVIF with best compression (slowest).
 * YUV444, CICP 1/1/1 (BT.709), speed 0, 10-bit depth, autotiling, tune=ssimulacra2.
 */
export function encodeCompact(data: Buffer, quality: number = 80, jobs: number = 0): EncodeResult {
  return encodeAvif(data, quality, jobs, presetCompact, 'encodeCompact')
}

/**
 * Encode JPEG/PNG to AVIF with fastest speed.
 * YUV420, CICP 1/13/6 (BT.709/sRGB/BT.601), speed 9, 64x64 tiling, tune=ssimulacra2.
 */
export function encodeFast(data: Buffer, quality: number = 80, jobs: number = 0): EncodeResult {
  return encodeAvif(data, quality, jobs, presetFast, 'encodeFast')
}

/**
 * Returns the EXIF orientation value (1-8) from JPEG data.
 * Returns 0 if no orientation tag is found or data is not JPEG.
 * This is a pure read-only operation — very fast, no decompression needed.
 */
export function jpegOrientation(data: Buffer): number {
  if (!data || data.length === 0) return 0
  return modernimage_jpeg_orientation(data, data.length)
}

/**
 * Applies rotation based on EXIF orientation, then strips EXIF metadata
 * (preserving ICC profile).
 *
 * Two-stage strategy (matching the Go and Rust bindings):
 *   1. Try jpegtran -perfect (truly lossless rotation). For most JPEGs whose
 *      internal storage is iMCU-aligned (real-camera output, libjpeg-encoded
 *      output), this is fast and pixel-perfect.
 *   2. If -perfect is refused (the source was encoded without iMCU padding),
 *      fall back to a decode → rotate → re-encode path. The re-encode is
 *      JPEG-lossy but **preserves every pixel**: no edge trimming. The
 *      original ICC profile is re-injected so color management round-trips.
 *
 * If orientation is 1 (normal) or not present, returns a copy of the input.
 * Throws ModernImageError on empty input.
 */
export function normalizeJpegOrientation(data: Buffer): Buffer {
  if (!data || data.length === 0) throw new ModernImageError('empty input data')

  const ori = jpegOrientation(data)
  if (ori <= 1) return Buffer.from(data)

  const transformArgs = orientationToJpegtranArgs(ori)
  if (!transformArgs) return Buffer.from(data)

  // Stage 1: try jpegtran -perfect.
  try {
    return jpegtranPerfect(data, transformArgs)
  } catch (e) {
    if (!(e instanceof ModernImageError)) throw e
    // fall through to fallback
  }

  // Stage 2: decode → rotate → re-encode (preserves every pixel).
  return decodeRotateEncode(data, ori)
}

function orientationToJpegtranArgs(ori: number): string[] | null {
  switch (ori) {
    case 2: return ['-flip', 'horizontal']
    case 3: return ['-rotate', '180']
    case 4: return ['-flip', 'vertical']
    case 5: return ['-transpose']
    case 6: return ['-rotate', '90']
    case 7: return ['-transverse']
    case 8: return ['-rotate', '270']
  }
  return null
}

function jpegtranPerfect(data: Buffer, transformArgs: string[]): Buffer {
  const tmpOut = createTempPath('.jpg')
  try {
    const argv = ['jpegtran', '-copy', 'icc', '-perfect', ...transformArgs, '-outfile', tmpOut]
    // normalizeJpegOrientation does not surface stderr (its public API
    // returns Buffer), so warnings are discarded here.
    return callTool(modernimage_jpegtran, data, argv, true).data
  } finally {
    try { fs.unlinkSync(tmpOut) } catch {}
  }
}

function decodeRotateEncode(data: Buffer, ori: number): Buffer {
  // Pure-JS jpeg codec (no native deps). Loaded lazily so package consumers
  // who never call normalizeJpegOrientation don't pay the require cost.
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  const jpegJs = require('jpeg-js') as {
    decode: (buf: Buffer, opts?: { useTArray?: boolean }) => { width: number; height: number; data: Buffer | Uint8Array }
    encode: (img: { width: number; height: number; data: Buffer | Uint8Array }, quality?: number) => { data: Buffer | Uint8Array }
  }

  // Extract ICC for re-injection (jpeg-js encode does not preserve metadata).
  const icc = extractIccFromJpeg(data)

  const decoded = jpegJs.decode(data, { useTArray: true })
  const srcW = decoded.width
  const srcH = decoded.height
  const srcPixels = decoded.data as Uint8Array // RGBA, length = srcW * srcH * 4

  let dstW: number
  let dstH: number
  let mapXY: (x: number, y: number) => [number, number]
  switch (ori) {
    case 2:
      dstW = srcW; dstH = srcH
      mapXY = (x, y) => [srcW - 1 - x, y]
      break
    case 3:
      dstW = srcW; dstH = srcH
      mapXY = (x, y) => [srcW - 1 - x, srcH - 1 - y]
      break
    case 4:
      dstW = srcW; dstH = srcH
      mapXY = (x, y) => [x, srcH - 1 - y]
      break
    case 5:
      dstW = srcH; dstH = srcW
      mapXY = (x, y) => [y, x]
      break
    case 6:
      dstW = srcH; dstH = srcW
      mapXY = (x, y) => [srcH - 1 - y, x]
      break
    case 7:
      dstW = srcH; dstH = srcW
      mapXY = (x, y) => [srcH - 1 - y, srcW - 1 - x]
      break
    case 8:
      dstW = srcH; dstH = srcW
      mapXY = (x, y) => [y, srcW - 1 - x]
      break
    default:
      return Buffer.from(data)
  }

  const dstPixels = Buffer.alloc(dstW * dstH * 4)
  for (let y = 0; y < srcH; y++) {
    for (let x = 0; x < srcW; x++) {
      const [nx, ny] = mapXY(x, y)
      const srcIdx = (y * srcW + x) * 4
      const dstIdx = (ny * dstW + nx) * 4
      dstPixels[dstIdx] = srcPixels[srcIdx]
      dstPixels[dstIdx + 1] = srcPixels[srcIdx + 1]
      dstPixels[dstIdx + 2] = srcPixels[srcIdx + 2]
      dstPixels[dstIdx + 3] = srcPixels[srcIdx + 3]
    }
  }

  const encoded = jpegJs.encode({ width: dstW, height: dstH, data: dstPixels }, 95)
  // jpeg-js returns either Buffer or Uint8Array; normalize to a fresh
  // ArrayBuffer-backed Buffer for type compatibility with the public API.
  const encodedBytes = encoded.data instanceof Buffer ? encoded.data : Buffer.from(encoded.data as Uint8Array)
  let out: Buffer = Buffer.alloc(encodedBytes.length)
  encodedBytes.copy(out)

  if (icc && icc.length > 0) {
    const withIcc = injectIccIntoJpeg(out, icc)
    if (withIcc) out = withIcc
    // Soft failure if ICC too large for one APP2 segment — return without ICC.
  }
  return out
}

// JPEG segment helpers used by the fallback path.

function extractIccFromJpeg(data: Buffer): Buffer | null {
  if (data.length < 4 || data[0] !== 0xff || data[1] !== 0xd8) return null
  const chunks: Array<{ seq: number; total: number; bytes: Buffer }> = []
  let pos = 2
  while (pos + 4 <= data.length) {
    while (pos < data.length && data[pos] === 0xff) pos++
    if (pos >= data.length) break
    const marker = data[pos]
    pos++
    if (marker === 0xd8 || marker === 0xd9 || (marker >= 0xd0 && marker <= 0xd7)) continue
    if (marker === 0xda) break // SOS — entropy-coded data follows
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

function injectIccIntoJpeg(jpegBuf: Buffer, icc: Buffer): Buffer | null {
  if (jpegBuf.length < 2 || jpegBuf[0] !== 0xff || jpegBuf[1] !== 0xd8) return null
  const overhead = 2 + 12 + 2
  if (icc.length + overhead > 0xffff) return null
  const segLen = overhead + icc.length
  const seg = Buffer.alloc(2 + segLen)
  seg[0] = 0xff
  seg[1] = 0xe2
  seg.writeUInt16BE(segLen, 2)
  seg.write('ICC_PROFILE\x00', 4, 'ascii')
  seg[16] = 0x01
  seg[17] = 0x01
  icc.copy(seg, 18)
  return Buffer.concat([jpegBuf.subarray(0, 2), seg, jpegBuf.subarray(2)])
}

/**
 * Get the libmodernimage version string.
 */
export function version(): string {
  return modernimage_version()
}
