#!/usr/bin/env bash
# Download libmodernimage for ALL platforms and install into golang/shared/.
# Usage: ./scripts/setup-go.sh [version]

set -euo pipefail

VERSION="${1:-0.3.1}"
# Windows dev note: we extract BOTH libmodernimage.a (static archive)
# AND libmodernimage.dll / libmodernimage.dll.a (import lib + DLL) so
# the Go binding can choose between static and dynamic linking. See
# golang/modernimage.go for the #cgo LDFLAGS comment on why Windows
# uses the DLL path.
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

  # Windows additionally needs the DLL + import library (.dll.a).
  # CGO on Windows statically links winpthreads-based libmodernimage
  # badly with Go's exception handling; linking the DLL via its
  # import library sidesteps that by keeping libmodernimage isolated
  # in its own module.
  if [ "$GO_PLATFORM" = "windows-amd64" ]; then
    cp "${SRC}/libmodernimage.dll"   "${GO_DIR}/lib/${GO_PLATFORM}/"
    cp "${SRC}/libmodernimage.dll.a" "${GO_DIR}/lib/${GO_PLATFORM}/"
  fi

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
