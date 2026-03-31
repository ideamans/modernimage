export class ModernImageError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ModernImageError'
  }
}

export interface EncodeResult {
  data: Buffer
  mimeType: string
}
