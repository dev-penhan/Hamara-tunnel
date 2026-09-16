#!/usr/bin/env bash
set -Eeuo pipefail

VERSION="${HAMARA_VERSION:-0.2.0}"
REPOSITORY="${HAMARA_REPOSITORY:-https://github.com/dev-penhan/Hamara-tunnel.git}"
INSTALL_DIR="${HAMARA_INSTALL_DIR:-/opt/hamara-tunnel}"

log() { printf '\033[1;36m[hamara-install]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[hamara-install] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[[ ${EUID:-$(id -u)} -eq 0 ]] || die "Run as root: sudo ./install.sh"
[[ -r /etc/os-release ]] || die "Cannot identify this operating system"
# shellcheck disable=SC1091
source /etc/os-release
[[ ${ID:-} == ubuntu ]] || die "Version 0.2 supports Ubuntu only (detected ${ID:-unknown})"
[[ ${VERSION_ID:-} == 22.04 || ${VERSION_ID:-} == 24.04 ]] || die "Supported Ubuntu versions: 22.04 and 24.04"

export DEBIAN_FRONTEND=noninteractive
log "Installing build prerequisites"
apt-get update
apt-get install -y --no-install-recommends ca-certificates curl git golang-go make sudo

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="$SCRIPT_DIR"
TEMP_DIR=""
if [[ ! -f "$SOURCE_DIR/go.mod" ]]; then
  TEMP_DIR="$(mktemp -d)"
  trap 'rm -rf "${TEMP_DIR:-}"' EXIT
  log "Fetching source from $REPOSITORY"
  if [[ "$VERSION" == "main" || "$VERSION" == "dev" ]]; then
    git clone --depth 1 "$REPOSITORY" "$TEMP_DIR/source"
  else
    git clone --depth 1 --branch "v$VERSION" "$REPOSITORY" "$TEMP_DIR/source" || \
      git clone --depth 1 --branch "$VERSION" "$REPOSITORY" "$TEMP_DIR/source"
  fi
  SOURCE_DIR="$TEMP_DIR/source"
fi

log "Building Hamara Tunnel $VERSION"
cd "$SOURCE_DIR"
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /usr/local/bin/hamara ./cmd/hamara
chmod 0755 /usr/local/bin/hamara

mkdir -p "$INSTALL_DIR"
install -m 0644 README.md README_FA.md LICENSE CHANGELOG.md SECURITY.md "$INSTALL_DIR/"
if [[ -d docs ]]; then
  rm -rf "$INSTALL_DIR/docs"
  cp -a docs "$INSTALL_DIR/docs"
fi

log "Installed /usr/local/bin/hamara"
printf '\nRun:\n  sudo hamara\n\n'
