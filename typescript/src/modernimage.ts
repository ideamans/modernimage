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

function callTool(
  toolFunc: ToolFunc,
  inputData: Buffer,
  argv: string[],
  useStdin: boolean,
): Buffer {
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

    // Find output path from argv (-o <path>)
    let outPath = ''
    for (let i = 0; i < argv.length; i++) {
      if (argv[i] === '-o' && i + 1 < argv.length) {
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

    return outData
  } finally {
    modernimage_context_free(ctx)
  }
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

    const outData = callTool(modernimage_cwebp, data, argv, true)
    return { data: outData, mimeType: 'image/webp' }
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

    const outData = callTool(modernimage_cwebp, data, argv, true)
    return { data: outData, mimeType: 'image/webp' }
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

    const outData = callTool(modernimage_gif2webp, data, argv, false)
    return { data: outData, mimeType: 'image/webp' }
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

    const outData = callTool(modernimage_avifenc, data, argv, true)
    return { data: outData, mimeType: 'image/avif' }
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
 * Get the libmodernimage version string.
 */
export function version(): string {
  return modernimage_version()
}
