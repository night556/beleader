#!/bin/bash
# Build BeLeader Desktop for all platforms.
# Prerequisites: Go 1.21+, Node.js, npm
#
# Usage:
#   ./build.sh          # Build for current platform
#   ./build.sh windows  # Cross-compile for Windows
#   ./build.sh all      # Build for all platforms

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DESKTOP_DIR="$ROOT/desktop"
WEB_DIR="$ROOT/web"
OUTPUT_DIR="$ROOT/dist"

# Build web frontend
echo "=== Building web frontend ==="
cd "$WEB_DIR"
npm install --silent
npm run build

# Sync web dist into desktop embed directory
rm -rf "$DESKTOP_DIR/webdist"
cp -r "$WEB_DIR/dist" "$DESKTOP_DIR/webdist"

# Build Go binary
echo "=== Building desktop binary ==="
cd "$DESKTOP_DIR"
go mod tidy

build_one() {
    local os="$1"
    local arch="$2"
    local ext="$3"
    local out="$OUTPUT_DIR/beleader-${os}-${arch}${ext}"

    echo "  -> $os/$arch"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags="-s -w" -o "$out" .
}

mkdir -p "$OUTPUT_DIR"

case "${1:-current}" in
    windows)
        build_one windows amd64 .exe
        ;;
    all)
        build_one windows amd64 .exe
        build_one linux amd64 ""
        build_one darwin amd64 ""
        build_one darwin arm64 ""
        ;;
    *)
        build_one "$(go env GOOS)" "$(go env GOARCH)" ""
        ;;
esac

echo "=== Done. Output in $OUTPUT_DIR ==="
ls -lh "$OUTPUT_DIR"