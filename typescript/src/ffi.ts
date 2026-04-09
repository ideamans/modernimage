import koffi from 'koffi'
import { getLibraryPath } from './library'

const libPath = getLibraryPath()
const lib = koffi.load(libPath)

// Opaque pointer type for modernimage_context_t*
const ctx_ptr = koffi.pointer('modernimage_context_t', koffi.opaque())

export const modernimage_context_new = lib.func('modernimage_context_new', ctx_ptr, [])

export const modernimage_context_free = lib.func('modernimage_context_free', 'void', [ctx_ptr])

export const modernimage_context_reset = lib.func('modernimage_context_reset', 'void', [ctx_ptr])

export const modernimage_set_stdin = lib.func('modernimage_set_stdin', 'void', [
  ctx_ptr,
  koffi.pointer('void'),
  'size_t',
])

export const modernimage_cwebp = lib.func('modernimage_cwebp', 'int', [ctx_ptr, 'int', 'str *'])

export const modernimage_gif2webp = lib.func('modernimage_gif2webp', 'int', [ctx_ptr, 'int', 'str *'])

export const modernimage_avifenc = lib.func('modernimage_avifenc', 'int', [ctx_ptr, 'int', 'str *'])

export const modernimage_jpegtran = lib.func('modernimage_jpegtran', 'int', [ctx_ptr, 'int', 'str *'])

export const modernimage_jpeg_orientation = lib.func('modernimage_jpeg_orientation', 'int', [
  koffi.pointer('void'),
  'size_t',
])

export const modernimage_get_exit_code = lib.func('modernimage_get_exit_code', 'int', [ctx_ptr])

export const modernimage_get_stderr_size = lib.func('modernimage_get_stderr_size', 'size_t', [ctx_ptr])

export const modernimage_copy_stderr = lib.func('modernimage_copy_stderr', 'size_t', [
  ctx_ptr,
  koffi.pointer('char'),
  'size_t',
])

export const modernimage_version = lib.func('modernimage_version', 'str', [])
