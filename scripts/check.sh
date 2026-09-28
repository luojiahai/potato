#!/usr/bin/env bash
# What CI and the release job check. staticcheck and govulncheck are pinned and
# fetched by `go run`, so go.mod lists only potato's own dependencies.
set -euo pipefail
cd "$(dirname "$0")/.."

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "check: gofmt wants to rewrite:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
# -count=1: cmd/potato's tests build and run the binary in their TestMain, an
# input the test cache cannot see, so a cached result there says nothing about
# the current source.
go test -race -count=1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
