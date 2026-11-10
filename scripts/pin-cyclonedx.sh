#!/usr/bin/env bash
# Rewrites the cyclonedx-cli checksums in setup-cyclonedx/install.sh for the
# version that file pins. Renovate runs it after bumping CYCLONEDX_VERSION, so
# a version update and its checksums land in the same commit.

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname "$0")/.." && pwd)"
INSTALL_SH="$ROOT_DIR/setup-cyclonedx/install.sh"

version="$(sed -n 's/^CYCLONEDX_VERSION=//p' "$INSTALL_SH")"
if [ -z "$version" ]; then
  echo "no CYCLONEDX_VERSION in $INSTALL_SH" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

for pair in LINUX_X64:cyclonedx-linux-x64 LINUX_ARM64:cyclonedx-linux-arm64; do
  var="CYCLONEDX_SHA256_${pair%%:*}"
  asset="${pair#*:}"
  curl -fsSL --retry 3 \
    "https://github.com/CycloneDX/cyclonedx-cli/releases/download/v${version}/${asset}" \
    -o "$tmp/$asset"
  sha="$(sha256sum "$tmp/$asset" | cut -d' ' -f1)"
  sed -i.bak "s/^${var}=.*/${var}=${sha}/" "$INSTALL_SH"
  rm -f "$INSTALL_SH.bak"
  echo "$var=$sha"
done
