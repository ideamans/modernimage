#!/bin/bash
# Download libmodernimage release artifacts for the current platform.
# Usage: ./scripts/download.sh [version] [target_dir]
#   version:    Release version (default: 0.1.0)
#   target_dir: Directory to extract into (default: current directory)

set -euo pipefail

VERSION="${1:-0.2.0}"
TARGET_DIR="${2:-.}"

GITHUB_REPO="ideamans/libmodernimage"
BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"

# Detect platform
detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    darwin) os="darwin" ;;
    linux)  os="linux" ;;
    *)      echo "Unsupported OS: $os" >&2; exit 1 ;;
  esac

  case "$arch" in
    arm64|aarch64)
      if [ "$os" = "darwin" ]; then
        arch="arm64"
      else
        arch="aarch64"
      fi
      ;;
    x86_64|amd64)
      arch="x86_64"
      ;;
    *)
      echo "Unsupported architecture: $arch" >&2; exit 1
      ;;
  esac

  echo "${os}-${arch}"
}

PLATFORM="$(detect_platform)"
ARCHIVE_NAME="libmodernimage-${PLATFORM}.tar.gz"
URL="${BASE_URL}/${ARCHIVE_NAME}"

echo "Downloading libmodernimage v${VERSION} for ${PLATFORM}..."
echo "URL: ${URL}"
echo "Target: ${TARGET_DIR}"

mkdir -p "${TARGET_DIR}"
curl -fsSL "${URL}" | tar xz -C "${TARGET_DIR}"

echo "Successfully downloaded to ${TARGET_DIR}/libmodernimage-${PLATFORM}/"
