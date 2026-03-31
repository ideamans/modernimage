import { describe, it } from 'node:test'
import * as assert from 'node:assert'
import * as fs from 'fs'
import * as path from 'path'
import { webp, avif, version, ModernImageError } from '../index'

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

function isAVIF(data: Buffer): boolean {
  return data.length >= 12 && data[4] === 0x66 && data[5] === 0x74 && data[6] === 0x79 && data[7] === 0x70
}

describe('version', () => {
  it('returns a version string', () => {
    const v = version()
    assert.ok(v.length > 0, 'version should not be empty')
    console.log(`libmodernimage version: ${v}`)
  })
})

describe('webp.encodeLossy', () => {
  it('encodes JPEG to lossy WebP', () => {
    const data = loadTestData('photo.jpg')
    const result = webp.encodeLossy(data, 80)
    assert.ok(isWebP(result.data), 'output should be WebP')
    assert.strictEqual(result.mimeType, 'image/webp')
    console.log(`JPEG -> WebP lossy: ${data.length} -> ${result.data.length} bytes`)
  })

  it('encodes PNG to lossy WebP with multithread', () => {
    const data = loadTestData('logo.png')
    const result = webp.encodeLossy(data, 80, true)
    assert.ok(isWebP(result.data), 'output should be WebP')
    console.log(`PNG -> WebP lossy: ${data.length} -> ${result.data.length} bytes`)
  })
})

describe('webp.encodeLossless', () => {
  it('encodes PNG to lossless WebP', () => {
    const data = loadTestData('logo.png')
    const result = webp.encodeLossless(data)
    assert.ok(isWebP(result.data), 'output should be WebP')
    console.log(`PNG -> WebP lossless: ${data.length} -> ${result.data.length} bytes`)
  })

  it('encodes JPEG to lossless WebP', () => {
    const data = loadTestData('photo.jpg')
    const result = webp.encodeLossless(data, true)
    assert.ok(isWebP(result.data), 'output should be WebP')
    console.log(`JPEG -> WebP lossless: ${data.length} -> ${result.data.length} bytes`)
  })
})

describe('webp.encodeGif', () => {
  it('encodes GIF to animated WebP', () => {
    const data = loadTestData('animation.gif')
    const result = webp.encodeGif(data)
    assert.ok(isWebP(result.data), 'output should be WebP')
    console.log(`GIF -> WebP: ${data.length} -> ${result.data.length} bytes`)
  })
})

describe('avif.encodeBalanced', () => {
  it('encodes JPEG to AVIF balanced', () => {
    const data = loadTestData('photo.jpg')
    const result = avif.encodeBalanced(data, 80)
    assert.ok(isAVIF(result.data), 'output should be AVIF')
    assert.strictEqual(result.mimeType, 'image/avif')
    console.log(`JPEG -> AVIF balanced: ${data.length} -> ${result.data.length} bytes`)
  })

  it('encodes PNG to AVIF balanced', () => {
    const data = loadTestData('logo.png')
    const result = avif.encodeBalanced(data, 80)
    assert.ok(isAVIF(result.data), 'output should be AVIF')
    console.log(`PNG -> AVIF balanced: ${data.length} -> ${result.data.length} bytes`)
  })
})

describe('avif.encodeCompact', () => {
  it('encodes JPEG to AVIF compact', () => {
    const data = loadTestData('photo.jpg')
    const result = avif.encodeCompact(data, 80)
    assert.ok(isAVIF(result.data), 'output should be AVIF')
    console.log(`JPEG -> AVIF compact: ${data.length} -> ${result.data.length} bytes`)
  })
})

describe('avif.encodeFast', () => {
  it('encodes JPEG to AVIF fast', () => {
    const data = loadTestData('photo.jpg')
    const result = avif.encodeFast(data, 80)
    assert.ok(isAVIF(result.data), 'output should be AVIF')
    console.log(`JPEG -> AVIF fast: ${data.length} -> ${result.data.length} bytes`)
  })
})

describe('error handling', () => {
  it('throws on empty input', () => {
    assert.throws(() => webp.encodeLossy(Buffer.alloc(0), 80), ModernImageError)
  })

  it('throws on GIF input to webp.encodeLossy', () => {
    const data = loadTestData('animation.gif')
    assert.throws(() => webp.encodeLossy(data, 80), ModernImageError)
  })

  it('throws on JPEG input to webp.encodeGif', () => {
    const data = loadTestData('photo.jpg')
    assert.throws(() => webp.encodeGif(data), ModernImageError)
  })
})
