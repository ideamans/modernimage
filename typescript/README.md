# modernimage - TypeScript/Node.js Binding

TypeScript/Node.js binding for libmodernimage via [Koffi](https://koffi.dev/) FFI, providing WebP and AVIF image encoding.

## Installation

```bash
npm install modernimage
```

### Pre-built Library

The shared library (`.dylib`/`.so`) must exist under `lib/{platform}/` before use. Run the setup script from the project root:

```bash
make setup
```

Supported platforms: `darwin-arm64`, `darwin-amd64`, `linux-amd64`, `linux-arm64`

## Usage

### WebP Encoding

```typescript
import { webp } from 'modernimage'
import * as fs from 'fs'

const jpeg = fs.readFileSync('photo.jpg')

// Lossy WebP (quality 80, multi-threaded)
const result = webp.encodeLossy(jpeg, 80, true)
fs.writeFileSync('photo.webp', result.data)
// result.mimeType === 'image/webp'

// Lossless WebP
const lossless = webp.encodeLossless(jpeg)
fs.writeFileSync('photo-lossless.webp', lossless.data)

// Animated GIF to WebP
const gif = fs.readFileSync('animation.gif')
const animated = webp.encodeGif(gif, true)
fs.writeFileSync('animation.webp', animated.data)
```

### AVIF Encoding

```typescript
import { avif } from 'modernimage'

const jpeg = fs.readFileSync('photo.jpg')

// Balanced (speed=6, YUV444, BT.709)
const balanced = avif.encodeBalanced(jpeg, 80)

// Best compression (speed=0, 10-bit, YUV444, BT.709)
const compact = avif.encodeCompact(jpeg, 80)

// Fastest (speed=9, YUV420, BT.709/sRGB/BT.601)
const fast = avif.encodeFast(jpeg, 80, 4) // quality=80, jobs=4
```

## API

### WebP (`webp`)

```typescript
webp.encodeLossy(data: Buffer, quality?: number, multithread?: boolean): EncodeResult
webp.encodeLossless(data: Buffer, multithread?: boolean): EncodeResult
webp.encodeGif(data: Buffer, multithread?: boolean): EncodeResult
```

### AVIF (`avif`)

```typescript
avif.encodeBalanced(data: Buffer, quality?: number, jobs?: number): EncodeResult
avif.encodeCompact(data: Buffer, quality?: number, jobs?: number): EncodeResult
avif.encodeFast(data: Buffer, quality?: number, jobs?: number): EncodeResult
```

### Types

```typescript
interface EncodeResult {
  data: Buffer
  mimeType: string // "image/webp" or "image/avif"
}

class ModernImageError extends Error {}
```

### Parameters

| Parameter | Range | Default | Description |
|-----------|-------|---------|-------------|
| `quality` | 0-100 | 80 | Encoding quality |
| `multithread` | boolean | false | Enable WebP multi-threading |
| `jobs` | 0+ | preset default | AVIF thread count (0 = auto) |

### Input Format

Input format is auto-detected from magic bytes:
- JPEG - accepted by lossy, lossless, balanced, compact, fast
- PNG - accepted by lossy, lossless, balanced, compact, fast
- GIF - accepted by encodeGif only

Passing an unsupported format throws `ModernImageError`.

## Requirements

- Node.js >= 18.0.0
- [Koffi](https://koffi.dev/) (installed as dependency)

## Testing

```bash
cd typescript
npm install
npm test
```
