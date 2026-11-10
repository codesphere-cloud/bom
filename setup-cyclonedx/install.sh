#!/usr/bin/env bash
# Installs the pinned cyclonedx-cli into the directory given as $1 and prints
# the binary's path. This file is the only place the version and checksums are
# defined: setup-cyclonedx and attest-chart both call it, and Renovate updates
# the version here (scripts/pin-cyclonedx.sh rewrites the checksums).
#
# A GitHub release asset is mutable, so the content is pinned and not just the
# version.

set -euo pipefail

# renovate: datasource=github-releases depName=CycloneDX/cyclonedx-cli
CYCLONEDX_VERSION=0.33.1
CYCLONEDX_SHA256_LINUX_X64=bfc8b2538da86fe239bc53658bbb63c1c8c510a293c1e6891aa5bea5d3c58746
CYCLONEDX_SHA256_LINUX_ARM64=b2e9fdf9665ef49868a2ec012171c6e785dcd69745bc5869e53e4f4bfb096a5f

install_dir="${1:?usage: install.sh <install-dir>}"

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64)
    asset=cyclonedx-linux-x64
    sha256="$CYCLONEDX_SHA256_LINUX_X64"
    ;;
  Linux/aarch64 | Linux/arm64)
    asset=cyclonedx-linux-arm64
    sha256="$CYCLONEDX_SHA256_LINUX_ARM64"
    ;;
  *)
    echo "unsupported platform: $(uname -s)/$(uname -m)" >&2
    exit 1
    ;;
esac

binary="$install_dir/cyclonedx"
mkdir -p "$install_dir"

# Several steps of one job may call this; a binary that already passes the
# checksum is reused instead of downloaded again.
if ! echo "$sha256  $binary" | sha256sum --check --strict --status 2>/dev/null; then
  curl -fsSL --retry 3 \
    "https://github.com/CycloneDX/cyclonedx-cli/releases/download/v${CYCLONEDX_VERSION}/${asset}" \
    -o "$binary.download"
  echo "$sha256  $binary.download" | sha256sum --check --strict >&2
  mv "$binary.download" "$binary"
  chmod 0755 "$binary"
fi

"$binary" --version >&2
echo "$binary"
