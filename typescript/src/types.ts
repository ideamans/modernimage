export class ModernImageError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ModernImageError'
  }
}

export interface EncodeResult {
  data: Buffer
  mimeType: string
  /**
   * Every non-empty stderr line emitted by the underlying tool
   * (cwebp / avifenc / gif2webp / jpegtran). The encode operation
   * succeeded — these are not fatal. The array may contain a mix of:
   *
   * - Genuine warnings about data quality issues, e.g.
   *   "Premature end of JPEG file" (libjpeg quietly fabricated missing
   *   entropy data — your output likely has black/grey areas where the
   *   source was truncated)
   * - Informational diagnostics that the tool always prints (cwebp in
   *   particular prints encoding stats: dimensions, output size, PSNR,
   *   macroblock distribution, etc.)
   *
   * libmodernimage does NOT filter these; the caller is expected to
   * inspect the lines and decide which deserve attention. A safe
   * heuristic is to look for known warning substrings ("Premature end",
   * "WARNING", "ERROR" etc.) rather than treating every line as a
   * problem.
   *
   * Empty when the tool produced no stderr.
   */
  warnings: string[]
}
