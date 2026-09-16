#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
VERSION="$(tr -d '[:space:]' < VERSION)"
[[ -n "$VERSION" ]] || { echo "VERSION is empty" >&2; exit 1; }
command -v go >/dev/null || { echo "Go is required" >&2; exit 1; }

rm -rf release
mkdir -p release
work="$(mktemp -d)"
cleanup() { rm -rf "$work"; }
trap cleanup EXIT

for arch in amd64 arm64; do
  stage="$work/stage-$arch"
  mkdir -p "$stage"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" -o "$stage/hamara" ./cmd/hamara
  install -m 0644 README.md README_FA.md LICENSE CHANGELOG.md "$stage/"
  tar -C "$stage" -czf "release/hamara-tunnel_${VERSION}_linux_${arch}.tar.gz" \
    hamara README.md README_FA.md LICENSE CHANGELOG.md
done

source_stage="$work/Hamara-tunnel-$VERSION"
mkdir -p "$source_stage"
cp -a .github cmd docs examples internal scripts \
  .gitignore CHANGELOG.md LICENSE Makefile README.md README_FA.md SECURITY.md \
  VERSION go.mod install.sh "$source_stage/"
tar -C "$work" -czf "release/hamara-tunnel_${VERSION}_source.tar.gz" "Hamara-tunnel-$VERSION"

(
  cd release
  sha256sum ./*.tar.gz > SHA256SUMS
  sha256sum -c SHA256SUMS
)
printf 'Release %s written to %s/release\n' "$VERSION" "$ROOT"
