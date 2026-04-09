#!/bin/bash
# Download libmodernimage release and install into binding directories.
# Usage: ./scripts/setup.sh [version]

set -euo pipefail

VERSION="${1:-0.3.1}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

GITHUB_REPO="ideamans/libmodernimage"
BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"

# Detect platform for release archive name
detect_release_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    darwin)  os="darwin" ;;
    linux)   os="linux" ;;
    mingw*|msys*|cygwin*) os="windows" ;;
    *)       echo "Unsupported OS: $os" >&2; exit 1 ;;
  esac

  case "$arch" in
    arm64|aarch64)
      [ "$os" = "darwin" ] && arch="arm64" || arch="aarch64"
      ;;
    x86_64|amd64) arch="x86_64" ;;
    *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
  esac

  echo "${os}-${arch}"
}

# Detect platform for Go-style directory naming (GOOS-GOARCH)
detect_go_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    mingw*|msys*|cygwin*) os="windows" ;;
  esac

  case "$arch" in
    arm64|aarch64) arch="arm64" ;;
    x86_64|amd64) arch="amd64" ;;
  esac

  echo "${os}-${arch}"
}

RELEASE_PLATFORM="$(detect_release_platform)"
GO_PLATFORM="$(detect_go_platform)"
ARCHIVE_NAME="libmodernimage-${RELEASE_PLATFORM}.tar.gz"
URL="${BASE_URL}/${ARCHIVE_NAME}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading libmodernimage v${VERSION} for ${RELEASE_PLATFORM}..."
curl -fsSL "${URL}" | tar xz -C "${TMPDIR}"

SRC="${TMPDIR}/libmodernimage-${RELEASE_PLATFORM}"

# Go binding: shared/include/ and shared/lib/{go-platform}/
echo "Installing for Go binding..."
mkdir -p "${PROJECT_DIR}/golang/shared/include"
mkdir -p "${PROJECT_DIR}/golang/shared/lib/${GO_PLATFORM}"
cp "${SRC}/modernimage.h" "${PROJECT_DIR}/golang/shared/include/"
cp "${SRC}/libmodernimage.a" "${PROJECT_DIR}/golang/shared/lib/${GO_PLATFORM}/"
# Mirror the Rust binding's Windows layout — same reason: we link the
# DLL via its import library on Windows to avoid the winpthreads/
# CGO runtime conflict. See golang/modernimage.go for details.
if [ "$GO_PLATFORM" = "windows-amd64" ]; then
  cp "${SRC}/libmodernimage.dll"   "${PROJECT_DIR}/golang/shared/lib/${GO_PLATFORM}/"
  cp "${SRC}/libmodernimage.dll.a" "${PROJECT_DIR}/golang/shared/lib/${GO_PLATFORM}/"
fi

# TypeScript binding: lib/{go-platform}/ (shared library)
echo "Installing for TypeScript binding..."
case "$(uname -s)" in
  Darwin)              SHARED_LIB="libmodernimage.dylib" ;;
  MINGW*|MSYS*|CYGWIN*) SHARED_LIB="libmodernimage.dll" ;;
  *)                   SHARED_LIB="libmodernimage.so" ;;
esac
mkdir -p "${PROJECT_DIR}/typescript/lib/${GO_PLATFORM}"
cp "${SRC}/${SHARED_LIB}" "${PROJECT_DIR}/typescript/lib/${GO_PLATFORM}/"

# Rust binding: lib/{go-platform}/ (static library)
echo "Installing for Rust binding..."
mkdir -p "${PROJECT_DIR}/rust/lib/${GO_PLATFORM}"
cp "${SRC}/libmodernimage.a" "${PROJECT_DIR}/rust/lib/${GO_PLATFORM}/"
# On Windows, Rust links the DLL's import library (.dll.a) instead of
# the fat static archive to sidestep a winpthreads/runtime conflict —
# see rust/build.rs. Ship the DLL and import library alongside the .a.
if [ "$GO_PLATFORM" = "windows-amd64" ]; then
  cp "${SRC}/libmodernimage.dll"   "${PROJECT_DIR}/rust/lib/${GO_PLATFORM}/"
  cp "${SRC}/libmodernimage.dll.a" "${PROJECT_DIR}/rust/lib/${GO_PLATFORM}/"
fi
cp "${SRC}/modernimage.h" "${PROJECT_DIR}/rust/"

echo ""
echo "Done! Installed libmodernimage v${VERSION} for all bindings."
echo "  Go:         golang/shared/lib/${GO_PLATFORM}/libmodernimage.a"
echo "  TypeScript: typescript/lib/${GO_PLATFORM}/${SHARED_LIB}"
echo "  Rust:       rust/lib/${GO_PLATFORM}/libmodernimage.a"
