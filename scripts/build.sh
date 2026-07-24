#!/usr/bin/env bash
# Release build (spec §6.1): compile all four targets from one machine,
# tar.gz each, and emit SHA256SUMS for install.sh / `potato update` to verify.
set -euo pipefail
cd "$(dirname "$0")/.."

version="${1:?usage: scripts/build.sh <version>   (e.g. 1.0.0, no leading v)}"
targets=(darwin-x64 darwin-arm64 linux-x64 linux-arm64)

rm -rf dist
mkdir -p dist

for target in "${targets[@]}"; do
  echo "building potato-${target}…"
  mkdir -p "dist/${target}"
  bun build --compile --minify src/cli.tsx \
    --target="bun-${target}" \
    --define "process.env.POTATO_VERSION=\"${version}\"" \
    --outfile "dist/${target}/potato"
  tar -czf "dist/potato-${target}.tar.gz" -C "dist/${target}" potato
done

(cd dist && shasum -a 256 potato-*.tar.gz > SHA256SUMS)
echo
echo "artifacts in dist/:"
ls -lh dist/*.tar.gz dist/SHA256SUMS
echo
echo "release with: gh release create v${version} dist/potato-*.tar.gz dist/SHA256SUMS"
