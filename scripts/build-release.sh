#!/bin/sh

set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version-tag> <output-directory>" >&2
    exit 2
fi

version=$1
output=$2
case "$version" in
    v[0-9]*.[0-9]*.[0-9]*-alpha.[0-9]*) ;;
    *)
        echo "build-release: version must be an alpha tag such as v0.1.0-alpha.1: $version" >&2
        exit 2
        ;;
esac

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
repository=$(CDPATH= cd -- "$script_directory/.." && pwd -P)
case "$output" in
    /*) ;;
    *) output="$repository/$output" ;;
esac
if [ -e "$output" ]; then
    echo "build-release: refusing to overwrite existing output $output" >&2
    exit 1
fi

scratch_root=${TMPDIR:-/tmp}
scratch=$(mktemp -d "${scratch_root%/}/forma-release-build.XXXXXX")
output_parent=$(dirname -- "$output")
mkdir -p "$output_parent"
asset_staging=$(mktemp -d "$output_parent/.forma-release-assets.XXXXXX")
cleanup() {
    rm -rf -- "$scratch"
    if [ -n "${asset_staging:-}" ] && [ -d "$asset_staging" ]; then
        rm -rf -- "$asset_staging"
    fi
}
trap cleanup EXIT HUP INT TERM

version_without_v=${version#v}

build_archive() {
    goos=$1
    goarch=$2
    archive="forma_${version_without_v}_${goos}_${goarch}.tar.gz"
    stage="$scratch/$goos-$goarch"
    mkdir -p "$stage"
    (
        cd "$repository"
        CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOFLAGS=-mod=readonly GOTOOLCHAIN=local \
            go build -trimpath -ldflags "-s -w -X=main.versionOverride=$version" \
            -o "$stage/forma" ./cmd/forma
    )
    tar -C "$stage" -czf "$asset_staging/$archive" forma
}

build_archive darwin amd64
build_archive darwin arm64
build_archive linux amd64
build_archive linux arm64

(
    cd "$asset_staging"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum forma_*.tar.gz > SHA256SUMS
    else
        shasum -a 256 forma_*.tar.gz > SHA256SUMS
    fi
)

mv "$asset_staging" "$output"
asset_staging=""
echo "built release assets in $output"
