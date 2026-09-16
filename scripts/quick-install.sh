#!/usr/bin/env bash
# Bootstrap installer for a published Hamara repository.
set -Eeuo pipefail

REPOSITORY="${HAMARA_REPOSITORY:-https://github.com/dev-penhan/Hamara-tunnel.git}"
VERSION="${HAMARA_VERSION:-main}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "Run as root: curl ... | sudo bash" >&2
  exit 1
fi

apt-get update
apt-get install -y --no-install-recommends ca-certificates git
if [[ "$VERSION" == "main" || "$VERSION" == "dev" ]]; then
  git clone --depth 1 "$REPOSITORY" "$work/hamara-tunnel"
else
  git clone --depth 1 --branch "v$VERSION" "$REPOSITORY" "$work/hamara-tunnel" || \
    git clone --depth 1 --branch "$VERSION" "$REPOSITORY" "$work/hamara-tunnel"
fi
INSTALL_VERSION="$VERSION"
if [[ "$VERSION" == "main" || "$VERSION" == "dev" ]]; then
  INSTALL_VERSION="$(tr -d '[:space:]' < "$work/hamara-tunnel/VERSION")"
fi
HAMARA_VERSION="$INSTALL_VERSION" "$work/hamara-tunnel/install.sh"
