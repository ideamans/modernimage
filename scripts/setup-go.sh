#!/usr/bin/env bash
# Download libmodernimage for ALL platforms and install into golang/shared/.
# Usage: ./scripts/setup-go.sh [version]

set -euo pipefail

VERSION="${1:-0.2.1}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
GO_DIR="${PROJECT_DIR}/golang/shared"

GITHUB_REPO="ideamans/libmodernimage"
BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"

# release-platform:go-platform pairs
PLATFORMS="darwin-arm64:darwin-arm64 linux-x86_64:linux-amd64 linux-aarch64:linux-arm64 windows-x86_64:windows-amd64"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

HEADER_INSTALLED=false

for PAIR in $PLATFORMS; do
  RELEASE_PLATFORM="${PAIR%%:*}"
  GO_PLATFORM="${PAIR##*:}"
  ARCHIVE_NAME="libmodernimage-${RELEASE_PLATFORM}.tar.gz"
  URL="${BASE_URL}/${ARCHIVE_NAME}"

  echo "Downloading ${RELEASE_PLATFORM} -> ${GO_PLATFORM}..."
  curl -fsSL "${URL}" | tar xz -C "${TMPDIR}"

  SRC="${TMPDIR}/libmodernimage-${RELEASE_PLATFORM}"

  # Static library
  mkdir -p "${GO_DIR}/lib/${GO_PLATFORM}"
  cp "${SRC}/libmodernimage.a" "${GO_DIR}/lib/${GO_PLATFORM}/"

  # Header (once)
  if [ "$HEADER_INSTALLED" = false ]; then
    mkdir -p "${GO_DIR}/include"
    cp "${SRC}/modernimage.h" "${GO_DIR}/include/"
    HEADER_INSTALLED=true
  fi

  echo "  -> ${GO_DIR}/lib/${GO_PLATFORM}/libmodernimage.a"
done

echo ""
echo "Done! All platforms installed to golang/shared/"
ls -lh "${GO_DIR}/lib"/*/libmodernimage.a
