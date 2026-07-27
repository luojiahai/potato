#!/usr/bin/env bash
# Release build (spec §6.1): compile all four targets from one machine,
# tar.gz each, and emit SHA256SUMS for install.sh / `potato update` to verify.
#
# The asset names are frozen: installed binaries resolve them by these exact
# strings, so Go's amd64 is published as x64.
set -euo pipefail
cd "$(dirname "$0")/.."

version="${1:?usage: scripts/build.sh <version>   (e.g. 1.0.0, no leading v)}"
targets=(darwin-x64 darwin-arm64 linux-x64 linux-arm64)

rm -rf dist
mkdir -p dist

for target in "${targets[@]}"; do
  echo "building potato-${target}…"
  os="${target%-*}"
  arch="${target#*-}"
  [ "$arch" = x64 ] && arch=amd64
  mkdir -p "dist/${target}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
    -trimpath \
    -ldflags "-s -w -X github.com/luojiahai/potato/internal/version.Version=${version}" \
    -o "dist/${target}/potato" \
    ./cmd/potato
  tar -czf "dist/potato-${target}.tar.gz" -C "dist/${target}" potato
done

(cd dist && shasum -a 256 potato-*.tar.gz > SHA256SUMS)
echo
echo "artifacts in dist/:"
ls -lh dist/*.tar.gz dist/SHA256SUMS
echo
echo "release with: gh release create v${version} dist/potato-*.tar.gz dist/SHA256SUMS"
