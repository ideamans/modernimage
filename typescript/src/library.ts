import * as path from 'path'
import * as fs from 'fs'

export function getPlatform(): string {
  const platform = process.platform
  const arch = process.arch

  if (platform === 'darwin') {
    return arch === 'arm64' ? 'darwin-arm64' : 'darwin-amd64'
  } else if (platform === 'linux') {
    return arch === 'arm64' ? 'linux-arm64' : 'linux-amd64'
  } else if (platform === 'win32') {
    return 'windows-amd64'
  }

  throw new Error(`Unsupported platform: ${platform}-${arch}`)
}

export function getLibraryFileName(): string {
  const platform = process.platform
  if (platform === 'darwin') return 'libmodernimage.dylib'
  if (platform === 'linux') return 'libmodernimage.so'
  if (platform === 'win32') return 'libmodernimage.dll'
  throw new Error(`Unsupported platform: ${platform}`)
}

export function findLibraryPath(): string {
  const platform = getPlatform()
  const libFileName = getLibraryFileName()

  // Development mode: ../lib/<platform>/
  const devPlatformPath = path.join(__dirname, '..', '..', 'lib', platform, libFileName)
  if (fs.existsSync(devPlatformPath)) {
    return devPlatformPath
  }

  // Installed package: ../lib/<platform>/
  const installedPath = path.join(__dirname, '..', 'lib', platform, libFileName)
  if (fs.existsSync(installedPath)) {
    return installedPath
  }

  throw new Error(
    `Cannot find libmodernimage shared library for ${platform}.\n` +
      `Searched paths:\n` +
      `  - ${devPlatformPath} (development)\n` +
      `  - ${installedPath} (installed)\n`
  )
}

let cachedLibraryPath: string | null = null

export function getLibraryPath(): string {
  if (!cachedLibraryPath) {
    cachedLibraryPath = findLibraryPath()
  }
  return cachedLibraryPath
}
