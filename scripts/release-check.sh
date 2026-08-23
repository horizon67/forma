#!/bin/sh

set -eu

if [ "$#" -ne 0 ]; then
    echo "usage: $0" >&2
    exit 2
fi

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
repository=$(CDPATH= cd -- "$script_directory/.." && pwd -P)
cd "$repository"

profile_version=$(sed -n 's/^[[:space:]]*AlphaLanguageProfile[[:space:]]*=[[:space:]]*"\([^"]*\)"/\1/p' docs/embed.go)
if [ -z "$profile_version" ]; then
    echo "release-check: cannot derive AlphaLanguageProfile from docs/embed.go" >&2
    exit 1
fi
if [ -n "${FORMA_RELEASE_VERSION:-}" ] && [ "$FORMA_RELEASE_VERSION" != "$profile_version" ]; then
    echo "release-check: tag $FORMA_RELEASE_VERSION differs from alpha profile $profile_version" >&2
    exit 1
fi

require_text() {
    file=$1
    text=$2
    if ! grep -F -- "$text" "$file" >/dev/null; then
        echo "release-check: $file does not contain release version contract: $text" >&2
        exit 1
    fi
}

require_text docs/alpha-language-profile.md "# Forma \`$profile_version\` Language Profile"
require_text docs/language-guide.md "\`$profile_version\` reference front-end"
require_text docs/language-reference.md "\`$profile_version\` release candidate"
require_text docs/cli.md "\`$profile_version\` release candidate"
require_text docs/quickstart.md "workflow for \`$profile_version\`"
require_text docs/quickstart.md "github.com/horizon67/forma/cmd/forma@$profile_version"
require_text docs/quickstart.md "forma $profile_version"
require_text docs/quickstart.md "horizon67/forma/$profile_version/docs/examples/alpha-quickstart.forma"
require_text docs/security.md "\`$profile_version\` thin runner"
require_text docs/install.md "\`$profile_version\` release candidate"
require_text docs/install.md "github.com/horizon67/forma/cmd/forma@$profile_version"
release_version=${profile_version#v}
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64
do
    require_text docs/install.md "forma_${release_version}_${platform}.tar.gz"
done
require_text README.md "unstable \`$profile_version\` distribution"
require_text README.md "github.com/horizon67/forma/cmd/forma@$profile_version"
require_text README.ja.md "不安定な\`$profile_version\`配布"
require_text README.ja.md "github.com/horizon67/forma/cmd/forma@$profile_version"
release_notes="docs/releases/$profile_version.md"
if [ ! -f "$release_notes" ]; then
    echo "release-check: missing release notes for $profile_version: $release_notes" >&2
    exit 1
fi
require_text "$release_notes" "# Forma $profile_version"
require_text "$release_notes" "blob/$profile_version/docs/alpha-language-profile.md"

echo "==> committed and working-tree whitespace"
diff_base=${FORMA_DIFF_BASE:-}
case "$diff_base" in
    ""|0000000000000000000000000000000000000000)
        if diff_base=$(git rev-parse --verify HEAD^ 2>/dev/null); then
            :
        else
            diff_base=4b825dc642cb6eb9a060e54bf8d69288fbee4904
        fi
        ;;
esac
if ! git cat-file -e "$diff_base^{tree}" 2>/dev/null; then
    echo "release-check: whitespace diff base is unavailable: $diff_base" >&2
    exit 1
fi
git diff --check "$diff_base" HEAD
git diff --check HEAD
git ls-files --others --exclude-standard | while IFS= read -r file
do
    if output=$(git diff --no-index --check -- /dev/null "$file" 2>&1); then
        :
    else
        status=$?
        if [ "$status" -gt 1 ]; then
            echo "$output" >&2
            exit 1
        fi
    fi
done

echo "==> source release gate"
GOFLAGS=-mod=readonly go test -count=1 ./...
GOFLAGS=-mod=readonly go vet ./...
GOFLAGS=-mod=readonly go test -race -count=1 ./...

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
    echo "release-check: gofmt required for:" >&2
    echo "$unformatted" >&2
    exit 1
fi

echo "==> repository E2E targets"
for target in \
    experiments/membership-agent-e2e/target \
    experiments/order-invariant-agent-e2e/target
do
    (
        cd "$target"
        GOFLAGS=-mod=readonly go test -count=1 ./...
        GOFLAGS=-mod=readonly go vet ./...
        GOFLAGS=-mod=readonly go test -race -count=1 ./...
    )
done

scratch_root=${TMPDIR:-/tmp}
scratch=$(mktemp -d "${scratch_root%/}/forma-release-check.XXXXXX")
cleanup() {
    chmod -R u+w "$scratch" 2>/dev/null || true
    rm -rf -- "$scratch"
}
trap cleanup EXIT HUP INT TERM

mkdir -p \
    "$scratch/home" \
    "$scratch/tmp" \
    "$scratch/gocache" \
    "$scratch/gomodcache" \
    "$scratch/gopath" \
    "$scratch/bin" \
    "$scratch/output"

clean_go() {
    env -i \
        PATH="$PATH" \
        HOME="$scratch/home" \
        TMPDIR="$scratch/tmp" \
        GOCACHE="$scratch/gocache" \
        GOMODCACHE="$scratch/gomodcache" \
        GOPATH="$scratch/gopath" \
        GOBIN="$scratch/bin" \
        GOENV=off \
        GOFLAGS=-mod=readonly \
        GOTOOLCHAIN=local \
        CGO_ENABLED=0 \
        "$@"
}

echo "==> clean module cache and source install"
clean_go go mod download
clean_go go install ./cmd/forma
forma="$scratch/bin/forma"
version_output=$($forma version)
case "$version_output" in
    "forma devel"*) ;;
    *)
        echo "release-check: clean source install reported unexpected version: $version_output" >&2
        exit 1
        ;;
esac
binary_version=${version_output#forma }

$forma authoring-context > "$scratch/output/authoring-context-1.md"
$forma authoring-context > "$scratch/output/authoring-context-2.md"
cmp "$scratch/output/authoring-context-1.md" "$scratch/output/authoring-context-2.md"
require_text "$scratch/output/authoring-context-1.md" "Forma binary: \`$binary_version\`"
require_text "$scratch/output/authoring-context-1.md" "Language profile: \`$profile_version\`"

echo "==> installed CLI example determinism"
while IFS='|' read -r name source
do
    $forma check "$source"
    $forma resolve "$source" > "$scratch/output/$name-resolve-1.json"
    $forma resolve "$source" > "$scratch/output/$name-resolve-2.json"
    cmp "$scratch/output/$name-resolve-1.json" "$scratch/output/$name-resolve-2.json"
    $forma request "$source" > "$scratch/output/$name-request-1.json"
    $forma request "$source" > "$scratch/output/$name-request-2.json"
    cmp "$scratch/output/$name-request-1.json" "$scratch/output/$name-request-2.json"
    for projection in navigation outcomes states flow
    do
        $forma project "$projection" "$source" > "$scratch/output/$name-$projection-1.txt"
        $forma project "$projection" "$source" > "$scratch/output/$name-$projection-2.txt"
        cmp "$scratch/output/$name-$projection-1.txt" "$scratch/output/$name-$projection-2.txt"
    done
done <<'EOF'
admin|examples/users.forma
membership|examples/email-verified-membership.forma
order|examples/orders.forma
quickstart|docs/examples/alpha-quickstart.forma
EOF

echo "==> release archives and checksums"
"$repository/scripts/build-release.sh" "$profile_version" "$scratch/release"
(
    cd "$scratch/release"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c SHA256SUMS
    else
        shasum -a 256 -c SHA256SUMS
    fi
)
archive_os=$(go env GOHOSTOS)
archive_arch=$(go env GOHOSTARCH)
host_archive="$scratch/release/forma_${profile_version#v}_${archive_os}_${archive_arch}.tar.gz"
if [ ! -f "$host_archive" ]; then
    echo "release-check: no host archive produced for $archive_os/$archive_arch" >&2
    exit 1
fi
mkdir -p "$scratch/release-host"
tar -C "$scratch/release-host" -xzf "$host_archive"
if [ "$("$scratch/release-host/forma" version)" != "forma $profile_version" ]; then
    echo "release-check: release archive binary version mismatch" >&2
    exit 1
fi
"$scratch/release-host/forma" authoring-context > "$scratch/output/release-authoring-context.md"
require_text "$scratch/output/release-authoring-context.md" "Forma binary: \`$profile_version\`"
require_text "$scratch/output/release-authoring-context.md" "Language profile: \`$profile_version\`"

echo "release gate passed for $profile_version"
