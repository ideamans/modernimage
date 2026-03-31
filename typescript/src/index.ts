import {
  encodeLossy,
  encodeLossless,
  encodeGif,
  encodeBalanced,
  encodeCompact,
  encodeFast,
  version,
} from './modernimage'

export { ModernImageError, EncodeResult } from './types'
export { version }

export const webp = {
  encodeLossy,
  encodeLossless,
  encodeGif,
}

export const avif = {
  encodeBalanced,
  encodeCompact,
  encodeFast,
}
