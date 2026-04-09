import {
  encodeLossy,
  encodeLossless,
  encodeGif,
  encodeBalanced,
  encodeCompact,
  encodeFast,
  jpegOrientation,
  normalizeJpegOrientation,
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

export const jpeg = {
  orientation: jpegOrientation,
  normalizeOrientation: normalizeJpegOrientation,
}
