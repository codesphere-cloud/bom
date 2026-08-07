#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist}"
MATRIX="${MATRIX:-linux/amd64 linux/arm64}"
GOCACHE_DIR="${GOCACHE_DIR:-$ROOT_DIR/.tmp/go-build-dist}"
GOMODCACHE_DIR="${GOMODCACHE_DIR:-$ROOT_DIR/.tmp/go-mod-dist}"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"
mkdir -p "$GOCACHE_DIR"
mkdir -p "$GOMODCACHE_DIR"

build_binary() {
	goos=$1
	goarch=$2
	binary_name=$3
	package_path=$4

	output_dir="$DIST_DIR/$goos-$goarch"
	output_path="$output_dir/$binary_name"

	mkdir -p "$output_dir"
	echo "building $output_path"
	GOCACHE=$GOCACHE_DIR GOMODCACHE=$GOMODCACHE_DIR GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 \
		go build -o "$output_path" "$package_path"
}

for target in $MATRIX; do
	goos=${target%/*}
	goarch=${target#*/}

	if [ "$goos" = "$goarch" ]; then
		echo "invalid matrix entry: $target" >&2
		exit 1
	fi

	build_binary "$goos" "$goarch" helm-bom ./cmd/helm-bom
	build_binary "$goos" "$goarch" helm-bom-action ./cmd/helm-bom-action
done
