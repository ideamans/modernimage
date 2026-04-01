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

describe('version', () => {
  it('returns a version string', () => {
    const v = version()
    assert.ok(v.length > 0)
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
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
})

describe('webp.encodeGif', () => {
  for (const name of gifFiles) {
    it(name, () => {
      const data = loadTestData(name)
      const result = webp.encodeGif(data)
      assert.ok(isWebP(result.data))
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
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
      console.log(`  ${name}: ${data.length} -> ${result.data.length}`)
    })
  }
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