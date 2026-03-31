#!/bin/bash
# Release all packages based on the current git tag.
# Usage: make release (or ./scripts/release.sh)
#
# Prerequisites:
#   - Current HEAD must have a tag matching v* (e.g. v0.2.0)
#   - cargo login (for crates.io)
#   - npm login (for npmjs.com)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

# Get current tag
TAG=$(git describe --tags --exact-match HEAD 2>/dev/null || true)
if [ -z "$TAG" ]; then
  echo "Error: HEAD has no tag. Create a tag first: git tag v0.x.0" >&2
  exit 1
fi

if [[ ! "$TAG" =~ ^v[0-9] ]]; then
  echo "Error: Tag '$TAG' does not match v* pattern" >&2
  exit 1
fi

VERSION="${TAG#v}"  # strip leading v
echo "Releasing version: $TAG ($VERSION)"
echo ""

# ── 1. Go module tag ─────────────────────────────────────────────
echo "=== Go module: tagging golang/$TAG ==="
GO_TAG="golang/$TAG"

if git rev-parse "$GO_TAG" >/dev/null 2>&1; then
  echo "  Tag $GO_TAG already exists, skipping."
else
  git tag "$GO_TAG"
  echo "  Created tag: $GO_TAG"
fi

git push origin "$TAG" "$GO_TAG" 2>/dev/null || git push origin "$GO_TAG"
echo "  Pushed: $GO_TAG"
echo ""

# ── 2. Rust crate: cargo publish ─────────────────────────────────
echo "=== Rust crate: cargo publish ==="
cd "$PROJECT_DIR/rust"

# Update Cargo.toml version if needed
CURRENT_CARGO_VER=$(grep '^version' Cargo.toml | head -1 | sed 's/.*"\(.*\)"/\1/')
if [ "$CURRENT_CARGO_VER" != "$VERSION" ]; then
  sed -i.bak "s/^version = \".*\"/version = \"$VERSION\"/" Cargo.toml
  rm -f Cargo.toml.bak
  echo "  Updated Cargo.toml version: $CURRENT_CARGO_VER -> $VERSION"
fi

cargo publish --allow-dirty
echo "  Published to crates.io: modernimage $VERSION"
echo ""

# ── 3. TypeScript: npm publish ───────────────────────────────────
echo "=== TypeScript: npm publish ==="
cd "$PROJECT_DIR/typescript"

# Update package.json version if needed
CURRENT_NPM_VER=$(node -p "require('./package.json').version")
if [ "$CURRENT_NPM_VER" != "$VERSION" ]; then
  npm version "$VERSION" --no-git-tag-version
  echo "  Updated package.json version: $CURRENT_NPM_VER -> $VERSION"
fi

npm run build
npm publish
echo "  Published to npm: modernimage $VERSION"
echo ""

# ── Done ─────────────────────────────────────────────────────────
echo "=== Release $TAG complete ==="
echo "  Go:         github.com/ideamans/modernimage/golang@$GO_TAG"
echo "  Rust:       https://crates.io/crates/modernimage/$VERSION"
echo "  TypeScript: https://www.npmjs.com/package/modernimage/v/$VERSION"
